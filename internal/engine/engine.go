package engine

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"n9e-alter-service/internal/config"
	"n9e-alter-service/internal/n9e"
	"n9e-alter-service/internal/state"
)

type Status struct {
	LastStartUnix   int64  `json:"last_start_unix"`
	LastEndUnix     int64  `json:"last_end_unix"`
	LastError       string `json:"last_error"`
	LastFetched     int    `json:"last_fetched"`
	LastTotal       int    `json:"last_total"`
	LastInputs      int    `json:"last_inputs"`
	ActiveTotal     int    `json:"active_total"`
	RecoveredTotal  int    `json:"recovered_total"`
	NewActives      int    `json:"new_actives"`
	NewRecovereds   int    `json:"new_recovereds"`
	PurgedRecovered int    `json:"purged_recovered"`
}

type Engine struct {
	cfg config.Config
	n9e *n9e.Client
	st  *state.Store

	routes []*compiledRoute

	pullMu sync.Mutex

	statusMu sync.RWMutex
	status   Status
}

type compiledRoute struct {
	cfg         config.RouteConfig
	name        string
	groupRe     *regexp.Regexp
	ruleRe      *regexp.Regexp
	severitySet map[int]struct{}
	rewrites    []compiledRewrite
}

type compiledRewrite struct {
	field   string
	re      *regexp.Regexp
	replace string
}

func New(cfg config.Config, st *state.Store) (*Engine, error) {
	c := n9e.New(cfg.N9E)

	routes := make([]*compiledRoute, 0, len(cfg.Routes))
	for i := range cfg.Routes {
		rc := cfg.Routes[i]
		if !rc.Enabled {
			continue
		}
		name := strings.TrimSpace(rc.Name)
		if name == "" {
			name = fmt.Sprintf("route-%d", i)
		}

		grpPat := strings.TrimSpace(rc.Match.GroupNameRegex)
		if grpPat == "" {
			grpPat = ".*"
		}
		groupRe, err := regexp.Compile(grpPat)
		if err != nil {
			return nil, fmt.Errorf("route %s group_name_regex: %w", name, err)
		}

		rulePat := strings.TrimSpace(rc.Match.RuleNameRegex)
		if rulePat == "" {
			rulePat = ".*"
		}
		ruleRe, err := regexp.Compile(rulePat)
		if err != nil {
			return nil, fmt.Errorf("route %s rule_name_regex: %w", name, err)
		}

		sevSet := map[int]struct{}{}
		for _, s := range rc.Match.SeverityIn {
			sevSet[s] = struct{}{}
		}

		rewrites := make([]compiledRewrite, 0, len(rc.Dedup.Rewrites))
		for _, rw := range rc.Dedup.Rewrites {
			field := strings.TrimSpace(rw.Field)
			pat := strings.TrimSpace(rw.Pattern)
			if field == "" || pat == "" {
				continue
			}
			re, err := regexp.Compile(pat)
			if err != nil {
				return nil, fmt.Errorf("route %s rewrite %s: %w", name, field, err)
			}
			rewrites = append(rewrites, compiledRewrite{field: field, re: re, replace: rw.Replace})
		}

		routes = append(routes, &compiledRoute{
			cfg:         rc,
			name:        name,
			groupRe:     groupRe,
			ruleRe:      ruleRe,
			severitySet: sevSet,
			rewrites:    rewrites,
		})
	}

	if len(routes) == 0 {
		return nil, fmt.Errorf("no enabled routes")
	}

	return &Engine{cfg: cfg, n9e: c, st: st, routes: routes}, nil
}

func (e *Engine) Status() Status {
	e.statusMu.RLock()
	st := e.status
	e.statusMu.RUnlock()
	return st
}

func (e *Engine) BuildInputEvents(evs []n9e.CurEvent) []state.InputEvent {
	return e.buildInputs(evs)
}

type PreviewItem struct {
	N9EHash string            `json:"n9e_hash"`
	N9EID   int64             `json:"n9e_id"`
	GroupID int64             `json:"group_id"`
	RuleID  int64             `json:"rule_id"`
	Severity int              `json:"severity"`
	Tags    map[string]string `json:"tags"`

	RouteName   string `json:"route_name"`
	DedupKey    string `json:"dedup_key"`
	ServiceHash string `json:"service_hash"`

	GroupNameBefore string `json:"group_name_before"`
	GroupNameAfter  string `json:"group_name_after"`
	RuleNameBefore  string `json:"rule_name_before"`
	RuleNameAfter   string `json:"rule_name_after"`
	EntityBefore    string `json:"entity_before"`
	EntityAfter     string `json:"entity_after"`
}

func (e *Engine) PreviewInputs(evs []n9e.CurEvent) []PreviewItem {
	if len(evs) == 0 {
		return nil
	}
	out := make([]PreviewItem, 0, len(evs))
	for _, ev := range evs {
		groupName := strings.TrimSpace(ev.GroupName)
		ruleName := strings.TrimSpace(ev.RuleName)
		sev := ev.Severity

		rt := e.matchRoute(groupName, ruleName, sev)
		if rt == nil {
			continue
		}

		tags := applyTagRewrites(rt.rewrites, tagsToMap(ev))
		entityKey, entityDisp := pickEntity(ev, tags, rt.cfg.Dedup.NormalizePodName)

		groupName2 := applyRewrites(rt.rewrites, "group_name", groupName)
		ruleName2 := applyRewrites(rt.rewrites, "rule_name", ruleName)
		entityKey2 := applyRewrites(rt.rewrites, "entity", entityKey)
		entityDisp2 := applyRewrites(rt.rewrites, "entity", entityDisp)

		dedupKey := buildDedupKey(rt.cfg.Dedup, ev, groupName2, ruleName2, sev, entityKey2)
		serviceHash := hashKey(rt.name + "|" + dedupKey)

		out = append(out, PreviewItem{
			N9EHash:          strings.TrimSpace(ev.Hash),
			N9EID:            ev.ID,
			GroupID:          ev.GroupID,
			RuleID:           ev.RuleID,
			Severity:         sev,
			Tags:             tags,
			RouteName:        rt.name,
			DedupKey:         dedupKey,
			ServiceHash:      serviceHash,
			GroupNameBefore:  groupName,
			GroupNameAfter:   groupName2,
			RuleNameBefore:   ruleName,
			RuleNameAfter:    ruleName2,
			EntityBefore:     entityDisp,
			EntityAfter:      entityDisp2,
		})
	}
	return out
}

func (e *Engine) RunOnce(ctx context.Context) (state.ApplyResult, error) {
	e.pullMu.Lock()
	defer e.pullMu.Unlock()

	start := time.Now()
	e.setStatus(func(s *Status) {
		s.LastStartUnix = start.Unix()
		s.LastError = ""
		s.LastFetched = 0
		s.LastTotal = 0
		s.LastInputs = 0
		s.NewActives = 0
		s.NewRecovereds = 0
		s.PurgedRecovered = 0
	})

	evs, total, err := e.n9e.FetchCurEvents(ctx, e.cfg.Pull)
	if err != nil {
		e.setStatus(func(s *Status) {
			s.LastEndUnix = time.Now().Unix()
			s.LastError = err.Error()
		})
		return state.ApplyResult{}, err
	}

	inputs := e.buildInputs(evs)

	res := e.st.ApplyPull(time.Now(), inputs, state.ApplyOptions{
		RecoverMissCount:       e.cfg.State.RecoverMissCount,
		RetainRecoveredSeconds: e.cfg.State.RetainRecoveredSeconds,
	})

	end := time.Now()
	active, recovered, _ := e.st.Summary()

	e.setStatus(func(s *Status) {
		s.LastEndUnix = end.Unix()
		s.LastFetched = len(evs)
		s.LastTotal = total
		s.LastInputs = len(inputs)
		s.ActiveTotal = active
		s.RecoveredTotal = recovered
		s.NewActives = len(res.NewActives)
		s.NewRecovereds = len(res.NewRecovereds)
		s.PurgedRecovered = res.PurgedRecovered
	})

	log.Printf("pull done fetched=%d total=%d inputs=%d active=%d recovered=%d new_active=%d new_recovered=%d", len(evs), total, len(inputs), active, recovered, len(res.NewActives), len(res.NewRecovereds))
	return res, nil
}

func (e *Engine) setStatus(fn func(s *Status)) {
	e.statusMu.Lock()
	fn(&e.status)
	e.statusMu.Unlock()
}

func (e *Engine) buildInputs(evs []n9e.CurEvent) []state.InputEvent {
	aggr := make(map[string]*state.InputEvent, len(evs))

	for _, ev := range evs {
		groupName := strings.TrimSpace(ev.GroupName)
		ruleName := strings.TrimSpace(ev.RuleName)
		sev := ev.Severity

		rt := e.matchRoute(groupName, ruleName, sev)
		if rt == nil {
			continue
		}

		tags := applyTagRewrites(rt.rewrites, tagsToMap(ev))
		entityKey, entityDisp := pickEntity(ev, tags, rt.cfg.Dedup.NormalizePodName)

		groupName2 := applyRewrites(rt.rewrites, "group_name", groupName)
		ruleName2 := applyRewrites(rt.rewrites, "rule_name", ruleName)
		entityKey2 := applyRewrites(rt.rewrites, "entity", entityKey)
		entityDisp2 := applyRewrites(rt.rewrites, "entity", entityDisp)

		firstTs := normalizeTs(ev.FirstTriggerTime)
		lastTs := normalizeTs(ev.TriggerTime)
		if firstTs == 0 {
			firstTs = lastTs
		}

		dedupKey := buildDedupKey(rt.cfg.Dedup, ev, groupName2, ruleName2, sev, entityKey2)
		serviceHash := hashKey(rt.name + "|" + dedupKey)

		it, ok := aggr[serviceHash]
		if !ok {
			aggr[serviceHash] = &state.InputEvent{
				ServiceHash:      serviceHash,
				RouteName:        rt.name,
				DedupKey:         dedupKey,
				N9EHash:          strings.TrimSpace(ev.Hash),
				N9EID:            ev.ID,
				GroupID:          ev.GroupID,
				GroupName:        groupName2,
				RuleID:           ev.RuleID,
				RuleName:         ruleName2,
				Severity:         sev,
				Entity:           entityDisp2,
				Tags:             tags,
				FirstTriggerTime: firstTs,
				LastTriggerTime:  lastTs,
				RawCount:         1,
			}
			continue
		}

		it.RawCount++
		if it.FirstTriggerTime == 0 || (firstTs > 0 && firstTs < it.FirstTriggerTime) {
			it.FirstTriggerTime = firstTs
		}
		if lastTs > it.LastTriggerTime {
			it.LastTriggerTime = lastTs
		}
	}

	out := make([]state.InputEvent, 0, len(aggr))
	for _, v := range aggr {
		if v == nil {
			continue
		}
		out = append(out, *v)
	}
	return out
}

func (e *Engine) matchRoute(groupName, ruleName string, severity int) *compiledRoute {
	for _, rt := range e.routes {
		if rt == nil {
			continue
		}
		if !rt.groupRe.MatchString(groupName) {
			continue
		}
		if !rt.ruleRe.MatchString(ruleName) {
			continue
		}
		if len(rt.severitySet) > 0 {
			if _, ok := rt.severitySet[severity]; !ok {
				continue
			}
		}
		return rt
	}
	return nil
}

func applyRewrites(rws []compiledRewrite, field string, s string) string {
	if len(rws) == 0 {
		return s
	}
	field = strings.TrimSpace(field)
	for _, rw := range rws {
		if strings.EqualFold(rw.field, field) {
			s = rw.re.ReplaceAllString(s, rw.replace)
		}
	}
	return s
}

// tag 重写：Dedup.Rewrites 的 field 支持 "tag:<key>"，用于对指定 tag 值做正则替换/提取（会影响后续 entity/dedup/绑定匹配）。
func applyTagRewrites(rws []compiledRewrite, tags map[string]string) map[string]string {
	if len(rws) == 0 || len(tags) == 0 {
		return tags
	}
	for _, rw := range rws {
		f := strings.TrimSpace(rw.field)
		if f == "" {
			continue
		}
		if !strings.HasPrefix(strings.ToLower(f), "tag:") {
			continue
		}
		key := strings.TrimSpace(f[len("tag:"):])
		if key == "" {
			continue
		}
		v, ok := tags[key]
		if !ok {
			continue
		}
		tags[key] = rw.re.ReplaceAllString(v, rw.replace)
	}
	return tags
}

func buildDedupKey(dc config.DedupConfig, ev n9e.CurEvent, groupName string, ruleName string, severity int, entityKey string) string {
	mode := strings.TrimSpace(dc.Mode)
	evHash := strings.TrimSpace(ev.Hash)
	if mode == "n9e_hash" && evHash != "" {
		parts := make([]string, 0, 4)
		if dc.IncludeGroupID {
			parts = append(parts, strconv.FormatInt(ev.GroupID, 10))
		}
		if dc.IncludeGroupName {
			parts = append(parts, groupName)
		}
		parts = append(parts, evHash)
		return strings.Join(parts, "|")
	}

	parts := make([]string, 0, 8)
	if dc.IncludeGroupID {
		parts = append(parts, strconv.FormatInt(ev.GroupID, 10))
	}
	if dc.IncludeGroupName {
		parts = append(parts, groupName)
	}
	if dc.IncludeRuleID {
		parts = append(parts, strconv.FormatInt(ev.RuleID, 10))
	}
	if dc.IncludeRuleName {
		parts = append(parts, ruleName)
	}
	if dc.IncludeSeverity {
		parts = append(parts, strconv.Itoa(severity))
	}
	if dc.IncludeEntity {
		parts = append(parts, entityKey)
	}

	key := strings.Join(parts, "|")
	if strings.TrimSpace(key) == "" {
		if evHash != "" {
			return evHash
		}
		if ruleName != "" {
			return ruleName
		}
		return strconv.FormatInt(ev.ID, 10)
	}
	return key
}

func hashKey(s string) string {
	sum := sha1.Sum([]byte(s))
	return hex.EncodeToString(sum[:])
}

func normalizeTs(ts int64) int64 {
	if ts <= 0 {
		return 0
	}
	if ts > 1_000_000_000_000 {
		return ts / 1000
	}
	return ts
}
