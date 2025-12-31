package workers

import (
	"context"
	"log"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"n9e-alter-service/internal/config"
	"n9e-alter-service/internal/dingtalk"
	"n9e-alter-service/internal/report"
	"n9e-alter-service/internal/state"
	"n9e-alter-service/internal/telemetry"
)

type Notifier struct {
	cfg config.Config
	st  *state.Store
	dt  *dingtalk.Client
	rr  *RobotResolver
	rdb *redis.Client
	stt *telemetry.Stats
}

func NewNotifier(cfg config.Config, st *state.Store, rdb *redis.Client, stats *telemetry.Stats) *Notifier {
	timeout := time.Duration(cfg.N9E.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &Notifier{cfg: cfg, st: st, dt: dingtalk.New(timeout), rr: NewRobotResolver(cfg), rdb: rdb, stt: stats}
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
	now := time.Now().Unix()

	type routePlan struct {
		idx       int
		routeName  string
		routeRobot string
		globalCfg  dingtalk.Config
		maxLines   int
		maxChars   int
		observeSec int
		repeatSec  int
		sendRec    bool
	}

	plans := make([]routePlan, 0, len(n.cfg.Routes))
	byRoute := map[string]*routePlan{}
	for i := range n.cfg.Routes {
		rc := n.cfg.Routes[i]
		if !rc.Enabled {
			continue
		}
		if !rc.Notify.Enabled {
			continue
		}
		routeName := normalizeRouteName(rc.Name, i)
		p := routePlan{
			idx:       i,
			routeName:  routeName,
			routeRobot: rc.Notify.RobotID,
			globalCfg:  mergeDingTalk(n.cfg.DingTalk, rc.Notify.DingTalk),
			maxLines:   rc.DailyReport.MaxLines,
			maxChars:   rc.DailyReport.MaxChars,
			observeSec: rc.Notify.ObserveSeconds,
			repeatSec:  rc.Notify.RepeatIntervalSeconds,
			sendRec:    rc.Notify.SendRecovered,
		}
		plans = append(plans, p)
		byRoute[routeName] = &plans[len(plans)-1]
	}
	if len(plans) == 0 {
		return
	}

	actives := map[string][]state.Record{}
	recovereds := map[string][]state.Record{}
	activeTotal := map[string]int{}
	recoveredTotal := map[string]int{}

	n.st.ForEachRecord(func(rec state.Record) bool {
		p := byRoute[rec.RouteName]
		if p == nil {
			return true
		}

		if rec.Status == state.StatusActive {
			activeTotal[p.routeName] = activeTotal[p.routeName] + 1
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
			n.sendByRobots(p.routeName, p.routeRobot, p.globalCfg, items, true, activeTotal[p.routeName], p.maxLines, p.maxChars, now)
		}
		if items := recovereds[p.routeName]; len(items) > 0 {
			n.sendByRobots(p.routeName, p.routeRobot, p.globalCfg, items, false, recoveredTotal[p.routeName], p.maxLines, p.maxChars, now)
		}
	}
}

func (n *Notifier) enrichFromRedisBucket(routeName string, items []state.Record) []state.Record {
	if n == nil || n.rdb == nil || len(items) == 0 {
		return items
	}
	if !n.cfg.State.Redis.Enabled || strings.TrimSpace(n.cfg.State.Redis.Addr) == "" {
		return items
	}
	routeName = strings.TrimSpace(routeName)
	if routeName == "" {
		return items
	}

	prefix := strings.TrimSpace(n.cfg.State.Redis.KeyPrefix)
	if prefix == "" {
		prefix = "n9e_alter"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	pipe := n.rdb.Pipeline()
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

func (n *Notifier) sendByRobots(routeName string, routeRobotID string, globalCfg dingtalk.Config, items []state.Record, active bool, total int, maxLines int, maxChars int, now int64) {
	if len(items) == 0 {
		return
	}

	if active {
		items = n.enrichFromRedisBucket(routeName, items)
	}

	type bucket struct {
		robots []ResolvedRobot
		items []state.Record
	}
	by := map[string]*bucket{}
	for _, it := range items {
		robots, _ := n.rr.ResolveRobotsForRecord(it, routeName, routeRobotID, globalCfg)
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
		if len(robots) == 0 {
			continue
		}

		var title, text string
		if active {
			title, text = report.BuildActiveMarkdown("N9E 告警通知", routeName, batch, total, maxLines, maxChars)
		} else {
			title, text = report.BuildRecoveredMarkdown("N9E 告警恢复", routeName, batch, maxLines, maxChars)
		}

		sentAny := false
		for _, rr := range robots {
			dtCfg := rr.Cfg
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
				fallbackIDs := n.rr.FallbackIDs(rr.ID)
				if len(fallbackIDs) > 0 {
					for _, fid := range fallbackIDs {
						cfg2, ok := n.rr.ConfigByID(fid)
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
	if n == nil || n.rdb == nil {
		return
	}
	if !n.cfg.State.Redis.Enabled || strings.TrimSpace(n.cfg.State.Redis.Addr) == "" {
		return
	}
	routeName = strings.TrimSpace(routeName)
	if routeName == "" || len(batch) == 0 {
		return
	}

	prefix := strings.TrimSpace(n.cfg.State.Redis.KeyPrefix)
	if prefix == "" {
		prefix = "n9e_alter"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	pipe := n.rdb.Pipeline()
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
	if n == nil || n.rdb == nil {
		return
	}
	if !n.cfg.State.Redis.Enabled || strings.TrimSpace(n.cfg.State.Redis.Addr) == "" {
		return
	}
	routeName = strings.TrimSpace(routeName)
	if routeName == "" || len(batch) == 0 {
		return
	}

	prefix := strings.TrimSpace(n.cfg.State.Redis.KeyPrefix)
	if prefix == "" {
		prefix = "n9e_alter"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	pipe := n.rdb.Pipeline()
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
