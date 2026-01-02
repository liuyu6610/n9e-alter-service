package workers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"n9e-alter-service/internal/config"
	"n9e-alter-service/internal/dingtalk"
	"n9e-alter-service/internal/redismgr"
	"n9e-alter-service/internal/report"
	"n9e-alter-service/internal/runtime"
	"n9e-alter-service/internal/state"
	"n9e-alter-service/internal/telemetry"
)

type Notifier struct {
	cfg config.Config
	rt  *runtime.Runtime
	st  *state.Store
	dt  *dingtalk.Client
	rm  *redismgr.Manager
	stt *telemetry.Stats
}

func NewNotifier(cfg config.Config, st *state.Store, rm *redismgr.Manager, stats *telemetry.Stats) *Notifier {
	timeout := time.Duration(cfg.N9E.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &Notifier{cfg: cfg, st: st, dt: dingtalk.New(timeout), rm: rm, stt: stats}
}

func NewNotifierWithRuntime(rt *runtime.Runtime, st *state.Store, rm *redismgr.Manager, stats *telemetry.Stats) *Notifier {
	snap := rt.Get()
	cfg := snap.Cfg
	timeout := time.Duration(cfg.N9E.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &Notifier{cfg: cfg, rt: rt, st: st, dt: dingtalk.New(timeout), rm: rm, stt: stats}
}

func (n *Notifier) Start(ctx context.Context) {
	check := time.Duration(n.cfg.Pull.IntervalSeconds) * time.Second
	if check <= 0 {
		check = 30 * time.Second
	}
	if check > 10*time.Second {
		check = 10 * time.Second
	}
	if check < 5*time.Second {
		check = 5 * time.Second
	}

	t := time.NewTicker(check)
	go func() {
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				n.tick()
			}
		}
	}()
}

func (n *Notifier) tick() {
	cfg := n.cfg
	if n.rt != nil {
		cfg = n.rt.Get().Cfg
	}
	resolver := NewRobotResolver(cfg)

	now := time.Now().Unix()

	type routePlan struct {
		idx         int
		routeName   string
		routeRobot  string
		globalCfg   dingtalk.Config
		maxLines    int
		maxChars    int
		observeSec  int
		repeatSec   int
		sendRec     bool
		webhook     config.WebhookConfig
		escalations []config.EscalationConfig
	}

	plans := make([]routePlan, 0, len(cfg.Routes))
	byRoute := map[string]*routePlan{}
	for i := range cfg.Routes {
		rc := cfg.Routes[i]
		if !rc.Enabled {
			continue
		}
		if !rc.Notify.Enabled {
			continue
		}
		routeName := normalizeRouteName(rc.Name, i)
		p := routePlan{
			idx:         i,
			routeName:   routeName,
			routeRobot:  rc.Notify.RobotID,
			globalCfg:   mergeDingTalk(cfg.DingTalk, rc.Notify.DingTalk),
			maxLines:    rc.DailyReport.MaxLines,
			maxChars:    rc.DailyReport.MaxChars,
			observeSec:  rc.Notify.ObserveSeconds,
			repeatSec:   rc.Notify.RepeatIntervalSeconds,
			sendRec:     rc.Notify.SendRecovered,
			webhook:     rc.Notify.Webhook,
			escalations: rc.Notify.Escalations,
		}
		plans = append(plans, p)
		byRoute[routeName] = &plans[len(plans)-1]
	}
	if len(plans) == 0 {
		return
	}

	actives := map[string][]state.Record{}
	recovereds := map[string][]state.Record{}
	escalations := map[string]map[int][]state.Record{}
	activeTotal := map[string]int{}
	recoveredTotal := map[string]int{}

	n.st.ForEachRecord(func(rec state.Record) bool {
		p := byRoute[rec.RouteName]
		if p == nil {
			return true
		}

		if rec.Status == state.StatusActive {
			activeTotal[p.routeName] = activeTotal[p.routeName] + 1
			if idx, esc, ok := pickEscalation(p.escalations, rec, now); ok {
				minRepeat := esc.RepeatIntervalSeconds
				if minRepeat <= 0 {
					minRepeat = 3600
				}
				if rec.EscalatedStep < idx || rec.LastEscalatedAt == 0 || now-rec.LastEscalatedAt >= int64(minRepeat) {
					byStep := escalations[p.routeName]
					if byStep == nil {
						byStep = map[int][]state.Record{}
						escalations[p.routeName] = byStep
					}
					limit := p.maxLines
					if limit <= 0 {
						limit = 1000
					}
					items := byStep[idx]
					if len(items) < limit {
						byStep[idx] = append(items, rec)
					}
				}
			}
			observe := p.observeSec
			if observe < 0 {
				observe = 0
			}
			repeat := p.repeatSec
			if repeat <= 0 {
				repeat = 3600
			}
			limit := p.maxLines
			if limit <= 0 {
				limit = 1000
			}
			if observe > 0 && now-rec.FirstSeenAt < int64(observe) {
				return true
			}
			if rec.LastNotified > 0 && now-rec.LastNotified < int64(repeat) {
				return true
			}
			items := actives[p.routeName]
			if len(items) < limit {
				actives[p.routeName] = append(items, rec)
			}
			return true
		}

		if rec.Status == state.StatusRecovered {
			if !p.sendRec {
				return true
			}
			recoveredTotal[p.routeName] = recoveredTotal[p.routeName] + 1
			limit := p.maxLines
			if limit <= 0 {
				limit = 1000
			}
			if rec.LastNotified != 0 {
				return true
			}
			items := recovereds[p.routeName]
			if len(items) < limit {
				recovereds[p.routeName] = append(items, rec)
			}
			return true
		}

		return true
	})

	for i := range plans {
		p := plans[i]
		if items := actives[p.routeName]; len(items) > 0 {
			n.sendByRobots(cfg, resolver, p.routeName, p.routeRobot, p.globalCfg, p.webhook, items, true, activeTotal[p.routeName], p.maxLines, p.maxChars, now)
		}
		if items := recovereds[p.routeName]; len(items) > 0 {
			n.sendByRobots(cfg, resolver, p.routeName, p.routeRobot, p.globalCfg, p.webhook, items, false, recoveredTotal[p.routeName], p.maxLines, p.maxChars, now)
		}
		if byStep := escalations[p.routeName]; len(byStep) > 0 {
			stepKeys := make([]int, 0, len(byStep))
			for k := range byStep {
				stepKeys = append(stepKeys, k)
			}
			sort.Ints(stepKeys)
			for _, stepIdx := range stepKeys {
				batch := byStep[stepIdx]
				if len(batch) == 0 {
					continue
				}
				esc, ok := escalationByIndex(p.escalations, stepIdx)
				if !ok {
					continue
				}
				n.sendEscalation(cfg, resolver, p.routeName, esc, stepIdx, batch, activeTotal[p.routeName], p.maxLines, p.maxChars, now)
			}
		}
	}
}

func pickEscalation(escalations []config.EscalationConfig, rec state.Record, now int64) (int, config.EscalationConfig, bool) {
	if len(escalations) == 0 {
		return 0, config.EscalationConfig{}, false
	}
	if rec.FirstSeenAt <= 0 {
		return 0, config.EscalationConfig{}, false
	}
	age := now - rec.FirstSeenAt
	if age < 0 {
		age = 0
	}
	bestIdx := -1
	bestAfter := int64(-1)
	best := config.EscalationConfig{}
	for i := range escalations {
		es := escalations[i]
		after := es.AfterSeconds
		if after < 0 {
			after = 0
		}
		if int64(after) <= age {
			if int64(after) > bestAfter {
				bestAfter = int64(after)
				bestIdx = i
				best = es
			}
		}
	}
	if bestIdx < 0 {
		return 0, config.EscalationConfig{}, false
	}
	return bestIdx, best, true
}

func escalationByIndex(escalations []config.EscalationConfig, idx int) (config.EscalationConfig, bool) {
	if idx < 0 || idx >= len(escalations) {
		return config.EscalationConfig{}, false
	}
	return escalations[idx], true
}

func (n *Notifier) sendEscalation(cfg config.Config, resolver *RobotResolver, routeName string, esc config.EscalationConfig, stepIdx int, items []state.Record, total int, maxLines int, maxChars int, now int64) {
	if n == nil || len(items) == 0 {
		return
	}
	if resolver == nil {
		resolver = NewRobotResolver(cfg)
	}
	filtered := make([]state.Record, 0, len(items))
	for i := range items {
		if n.isSilenced(cfg, routeName, items[i], now) {
			continue
		}
		filtered = append(filtered, items[i])
	}
	items = filtered
	if len(items) == 0 {
		return
	}

	robots := make([]ResolvedRobot, 0, len(esc.RobotIDs))
	for _, id := range esc.RobotIDs {
		cfg2, ok := resolver.ConfigByID(id)
		if !ok {
			continue
		}
		robots = append(robots, ResolvedRobot{ID: strings.TrimSpace(id), Cfg: cfg2})
	}
	if len(robots) == 0 && !(esc.Webhook.Enabled && strings.TrimSpace(esc.Webhook.URL) != "") {
		return
	}

	limit := maxLines
	if limit <= 0 {
		limit = 1000
	}
	batch := items
	if len(batch) > limit {
		batch = batch[:limit]
	}

	title, text := report.BuildActiveMarkdown("N9E 告警升级", routeName, batch, total, maxLines, maxChars)
	sentAny := false
	if esc.Webhook.Enabled && strings.TrimSpace(esc.Webhook.URL) != "" {
		payload := map[string]any{
			"time_unix": now,
			"route":     routeName,
			"active":    true,
			"step":      stepIdx,
			"title":     title,
			"text":      text,
			"total":     total,
			"items":     batch,
		}
		if err := n.sendWebhook(esc.Webhook, payload); err != nil {
			if n.stt != nil {
				n.stt.IncNotifySendError(1)
			}
			log.Printf("notify escalation webhook route=%s step=%d err=%v", routeName, stepIdx, err)
		} else {
			if n.stt != nil {
				n.stt.IncNotifySendOK(1)
			}
			sentAny = true
		}
	}
	for _, rb := range robots {
		dtCfg := rb.Cfg
		if strings.TrimSpace(dtCfg.Webhook) == "" {
			continue
		}
		if err := n.dt.SendMarkdown(dtCfg, title, text); err != nil {
			if n.stt != nil {
				n.stt.IncNotifySendError(1)
			}
			log.Printf("notify escalation route=%s step=%d err=%v", routeName, stepIdx, err)
			continue
		}
		if n.stt != nil {
			n.stt.IncNotifySendOK(1)
		}
		sentAny = true
	}
	if !sentAny {
		return
	}
	for _, it := range batch {
		_ = n.st.MarkEscalated(it.ServiceHash, stepIdx, now)
	}
}

func (n *Notifier) isSilenced(cfg config.Config, routeName string, rec state.Record, now int64) bool {
	if len(cfg.Silences) == 0 {
		return false
	}
	for i := range cfg.Silences {
		s := cfg.Silences[i]
		if !s.Enabled {
			continue
		}
		if s.ExpiresAtUnix > 0 && now >= s.ExpiresAtUnix {
			continue
		}
		if strings.TrimSpace(s.RouteName) != "" && !strings.EqualFold(strings.TrimSpace(s.RouteName), strings.TrimSpace(routeName)) {
			continue
		}
		ok := true
		if len(s.Tags) > 0 {
			for k, v := range s.Tags {
				k = strings.TrimSpace(k)
				if k == "" {
					continue
				}
				want := strings.TrimSpace(v)
				got := ""
				if rec.Tags != nil {
					got = strings.TrimSpace(rec.Tags[k])
				}
				if got != want {
					ok = false
					break
				}
			}
		}
		if !ok {
			continue
		}
		if len(s.TagRegex) > 0 {
			for k, pat := range s.TagRegex {
				k = strings.TrimSpace(k)
				pat = strings.TrimSpace(pat)
				if k == "" || pat == "" {
					continue
				}
				got := ""
				if rec.Tags != nil {
					got = rec.Tags[k]
				}
				matched, err := regexp.MatchString(pat, got)
				if err != nil || !matched {
					ok = false
					break
				}
			}
		}
		if ok {
			return true
		}
	}
	return false
}

func (n *Notifier) sendWebhook(cfg config.WebhookConfig, payload any) error {
	if !cfg.Enabled {
		return nil
	}
	url := strings.TrimSpace(cfg.URL)
	if url == "" {
		return nil
	}
	tmo := time.Duration(cfg.TimeoutSeconds) * time.Second
	if tmo <= 0 {
		tmo = 5 * time.Second
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	for k, v := range cfg.Headers {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		req.Header.Set(k, v)
	}
	cli := &http.Client{Timeout: tmo}
	resp, err := cli.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook http status=%d", resp.StatusCode)
	}
	return nil
}

func (n *Notifier) enrichFromRedisBucket(routeName string, items []state.Record) []state.Record {
	if n == nil || len(items) == 0 {
		return items
	}
	rdb := (*redis.Client)(nil)
	if n.rm != nil {
		rdb = n.rm.Get()
	}
	if rdb == nil {
		return items
	}
	cfg := n.cfg
	if n.rt != nil {
		cfg = n.rt.Get().Cfg
	}
	if !cfg.State.Redis.Enabled || strings.TrimSpace(cfg.State.Redis.Addr) == "" {
		return items
	}
	routeName = strings.TrimSpace(routeName)
	if routeName == "" {
		return items
	}

	prefix := strings.TrimSpace(cfg.State.Redis.KeyPrefix)
	if prefix == "" {
		prefix = "n9e_alter"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	pipe := rdb.Pipeline()
	cmds := make([]*redis.SliceCmd, 0, len(items))
	idx := make([]int, 0, len(items))
	for i := range items {
		it := items[i]
		if strings.TrimSpace(it.DedupKey) == "" {
			continue
		}
		k := prefix + ":bucket:" + routeName + ":" + it.DedupKey
		cmds = append(cmds, pipe.HMGet(ctx, k, "raw_count", "first_trigger_time", "last_trigger_time"))
		idx = append(idx, i)
	}
	if len(cmds) == 0 {
		return items
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return items
	}

	for j := range cmds {
		vals, err := cmds[j].Result()
		if err != nil || len(vals) < 3 {
			continue
		}
		i := idx[j]
		if i < 0 || i >= len(items) {
			continue
		}
		// raw_count
		if vals[0] != nil {
			s := strings.TrimSpace(toString(vals[0]))
			if s != "" {
				if n64, err := strconv.ParseInt(s, 10, 64); err == nil {
					if n64 > 0 {
						items[i].RawCount = int(n64)
					}
				}
			}
		}
		// first_trigger_time
		if vals[1] != nil {
			s := strings.TrimSpace(toString(vals[1]))
			if s != "" {
				if n64, err := strconv.ParseInt(s, 10, 64); err == nil {
					if n64 > 0 {
						items[i].FirstTriggerTime = n64
					}
				}
			}
		}
		// last_trigger_time
		if vals[2] != nil {
			s := strings.TrimSpace(toString(vals[2]))
			if s != "" {
				if n64, err := strconv.ParseInt(s, 10, 64); err == nil {
					if n64 > 0 {
						items[i].LastTriggerTime = n64
					}
				}
			}
		}
	}
	return items
}

func toString(v any) string {
	s, ok := v.(string)
	if ok {
		return s
	}
	bs, ok := v.([]byte)
	if ok {
		return string(bs)
	}
	return ""
}

func (n *Notifier) sendByRobots(cfg config.Config, resolver *RobotResolver, routeName string, routeRobotID string, globalCfg dingtalk.Config, webhookCfg config.WebhookConfig, items []state.Record, active bool, total int, maxLines int, maxChars int, now int64) {
	if len(items) == 0 {
		return
	}
	if resolver == nil {
		resolver = NewRobotResolver(cfg)
	}

	filtered := make([]state.Record, 0, len(items))
	for i := range items {
		if n.isSilenced(cfg, routeName, items[i], now) {
			continue
		}
		filtered = append(filtered, items[i])
	}
	items = filtered
	if len(items) == 0 {
		return
	}

	if active {
		items = n.enrichFromRedisBucket(routeName, items)
	}

	type bucket struct {
		robots []ResolvedRobot
		items  []state.Record
	}
	by := map[string]*bucket{}
	for _, it := range items {
		robots, _ := resolver.ResolveRobotsForRecord(it, routeName, routeRobotID, globalCfg)
		cfgs := make([]dingtalk.Config, 0, len(robots))
		for i := range robots {
			cfgs = append(cfgs, robots[i].Cfg)
		}
		key := robotsKey(cfgs)
		b := by[key]
		if b == nil {
			b = &bucket{robots: robots, items: make([]state.Record, 0, 8)}
			by[key] = b
		}
		b.items = append(b.items, it)
	}

	keys := make([]string, 0, len(by))
	for k := range by {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		b := by[k]
		if b == nil {
			continue
		}
		batch := b.items
		robots := b.robots

		var title, text string
		if active {
			title, text = report.BuildActiveMarkdown("N9E 告警通知", routeName, batch, total, maxLines, maxChars)
		} else {
			title, text = report.BuildRecoveredMarkdown("N9E 告警恢复", routeName, batch, maxLines, maxChars)
		}

		sentAny := false
		if webhookCfg.Enabled && strings.TrimSpace(webhookCfg.URL) != "" {
			payload := map[string]any{
				"time_unix": now,
				"route":     routeName,
				"active":    active,
				"title":     title,
				"text":      text,
				"total":     total,
				"items":     batch,
			}
			if err := n.sendWebhook(webhookCfg, payload); err != nil {
				if n.stt != nil {
					n.stt.IncNotifySendError(1)
				}
				log.Printf("notify webhook route=%s active=%v err=%v", routeName, active, err)
			} else {
				if n.stt != nil {
					n.stt.IncNotifySendOK(1)
				}
				sentAny = true
			}
		}
		for _, rb := range robots {
			dtCfg := rb.Cfg
			if strings.TrimSpace(dtCfg.Webhook) == "" {
				continue
			}
			sent := true
			if err := n.dt.SendMarkdown(dtCfg, title, text); err != nil {
				sent = false
				if n.stt != nil {
					n.stt.IncNotifySendError(1)
				}
				log.Printf("notify route=%s active=%v err=%v", routeName, active, err)
				fallbackIDs := resolver.FallbackIDs(rb.ID)
				if len(fallbackIDs) > 0 {
					for _, fid := range fallbackIDs {
						cfg2, ok := resolver.ConfigByID(fid)
						if !ok {
							continue
						}
						if err2 := n.dt.SendMarkdown(cfg2, title, text); err2 != nil {
							log.Printf("notify fallback route=%s active=%v fid=%s err=%v", routeName, active, fid, err2)
							continue
						}
						sent = true
						break
					}
				}
			}
			if !sent {
				continue
			}
			if n.stt != nil {
				n.stt.IncNotifySendOK(1)
			}
			sentAny = true
		}
		if !sentAny {
			continue
		}
		for _, it := range batch {
			_ = n.st.MarkNotified(it.ServiceHash, now)
		}
		if active {
			n.resetBucketsAfterNotify(routeName, batch)
		} else {
			n.deleteBucketsAfterRecovered(routeName, batch)
		}
	}
}

func (n *Notifier) resetBucketsAfterNotify(routeName string, batch []state.Record) {
	if n == nil {
		return
	}
	rdb := (*redis.Client)(nil)
	if n.rm != nil {
		rdb = n.rm.Get()
	}
	if rdb == nil {
		return
	}
	cfg := n.cfg
	if n.rt != nil {
		cfg = n.rt.Get().Cfg
	}
	if !cfg.State.Redis.Enabled || strings.TrimSpace(cfg.State.Redis.Addr) == "" {
		return
	}
	routeName = strings.TrimSpace(routeName)
	if routeName == "" || len(batch) == 0 {
		return
	}

	prefix := strings.TrimSpace(cfg.State.Redis.KeyPrefix)
	if prefix == "" {
		prefix = "n9e_alter"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	pipe := rdb.Pipeline()
	reset := 0
	for i := range batch {
		dk := strings.TrimSpace(batch[i].DedupKey)
		if dk == "" {
			continue
		}
		k := prefix + ":bucket:" + routeName + ":" + dk
		pipe.HSet(ctx, k, "raw_count", 0)
		pipe.HDel(ctx, k, "first_trigger_time", "last_trigger_time")
		reset++
	}
	_, _ = pipe.Exec(ctx)
	if n.stt != nil && reset > 0 {
		n.stt.IncBucketReset(uint64(reset))
	}
}

func (n *Notifier) deleteBucketsAfterRecovered(routeName string, batch []state.Record) {
	if n == nil {
		return
	}
	rdb := (*redis.Client)(nil)
	if n.rm != nil {
		rdb = n.rm.Get()
	}
	if rdb == nil {
		return
	}
	cfg := n.cfg
	if n.rt != nil {
		cfg = n.rt.Get().Cfg
	}
	if !cfg.State.Redis.Enabled || strings.TrimSpace(cfg.State.Redis.Addr) == "" {
		return
	}
	routeName = strings.TrimSpace(routeName)
	if routeName == "" || len(batch) == 0 {
		return
	}

	prefix := strings.TrimSpace(cfg.State.Redis.KeyPrefix)
	if prefix == "" {
		prefix = "n9e_alter"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	pipe := rdb.Pipeline()
	del := 0
	for i := range batch {
		dk := strings.TrimSpace(batch[i].DedupKey)
		if dk == "" {
			continue
		}
		k := prefix + ":bucket:" + routeName + ":" + dk
		pipe.Del(ctx, k)
		del++
	}
	_, _ = pipe.Exec(ctx)
	if n.stt != nil && del > 0 {
		n.stt.IncBucketDel(uint64(del))
	}
}

func robotsKey(cfgs []dingtalk.Config) string {
	if len(cfgs) == 0 {
		return ""
	}
	parts := make([]string, 0, len(cfgs))
	for _, c := range cfgs {
		parts = append(parts, strings.TrimSpace(c.Webhook)+"|"+strings.TrimSpace(c.Secret)+"|"+strings.TrimSpace(c.Keyword))
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}
