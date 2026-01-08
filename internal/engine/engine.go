package engine

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"text/template"
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
	LastMatched     int    `json:"last_matched"`
	LastDropped     int    `json:"last_dropped"`
	LastUnmatched   int    `json:"last_unmatched"`
	ActiveTotal     int    `json:"active_total"`
	RecoveredTotal  int    `json:"recovered_total"`
	NewActives      int    `json:"new_actives"`
	NewRecovereds   int    `json:"new_recovereds"`
	PurgedRecovered int    `json:"purged_recovered"`
}

type PullDebug struct {
	Fetched   int            `json:"fetched"`
	Total     int            `json:"total"`
	Matched   int            `json:"matched"`
	Dropped   int            `json:"dropped"`
	Unmatched int            `json:"unmatched"`
	Routes    map[string]int `json:"routes"`
}

type Engine struct {
	cfg config.Config
	n9e *n9e.Client
	st  *state.Store

	routes []*compiledRoute

	pullMu sync.Mutex

	statusMu sync.RWMutex
	status   Status

	pullDbgMu sync.RWMutex
	pullDbg   PullDebug
}

type compiledRoute struct {
	cfg         config.RouteConfig
	name        string
	groupRe     *regexp.Regexp
	ruleRe      *regexp.Regexp
	severitySet map[int]struct{}
	processors  []compiledProcessor
	rewrites    []compiledRewrite
}

type compiledProcessor struct {
	typ string

	dropWhen *template.Template

	relabel []compiledRelabelRule
	updates []config.UpdateSet
}

type compiledRelabelRule struct {
	target  string
	re      *regexp.Regexp
	replace string
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

		procs, err := compileProcessors(name, rc.Processors)
		if err != nil {
			return nil, err
		}

		routes = append(routes, &compiledRoute{
			cfg:         rc,
			name:        name,
			groupRe:     groupRe,
			ruleRe:      ruleRe,
			severitySet: sevSet,
			processors:  procs,
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

func (e *Engine) PullDebug() PullDebug {
	e.pullDbgMu.RLock()
	d := e.pullDbg
	e.pullDbgMu.RUnlock()
	if d.Routes == nil {
		return d
	}
	out := make(map[string]int, len(d.Routes))
	for k, v := range d.Routes {
		out[k] = v
	}
	d.Routes = out
	return d
}

func (e *Engine) BuildInputEvents(evs []n9e.CurEvent) []state.InputEvent {
	return e.buildInputs(evs)
}

func (e *Engine) buildInputs(evs []n9e.CurEvent) []state.InputEvent {
	items, _ := e.buildInputsWithStats(evs)
	return items
}

type PreviewItem struct {
	N9EHash  string            `json:"n9e_hash"`
	N9EID    int64             `json:"n9e_id"`
	GroupID  int64             `json:"group_id"`
	RuleID   int64             `json:"rule_id"`
	Severity int               `json:"severity"`
	Tags     map[string]string `json:"tags"`

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

		tags := tagsToMap(ev)
		groupName, ruleName, tags, dropped := applyProcessors(rt.processors, ev, groupName, ruleName, tags)
		if dropped {
			continue
		}

		tags = applyTagRewrites(rt.rewrites, tags)
		entityKey, entityDisp := pickEntity(ev, tags, rt.cfg.Dedup.NormalizePodName)
		entityKey, entityDisp = applyProcessorsEntity(rt.processors, entityKey, entityDisp)

		groupName2 := applyRewrites(rt.rewrites, "group_name", groupName)
		ruleName2 := applyRewrites(rt.rewrites, "rule_name", ruleName)
		entityKey2 := applyRewrites(rt.rewrites, "entity", entityKey)
		entityDisp2 := applyRewrites(rt.rewrites, "entity", entityDisp)

		dedupKey := buildDedupKey(rt.cfg.Dedup, ev, groupName2, ruleName2, sev, entityKey2)
		serviceHash := hashKey(rt.name + "|" + dedupKey)

		out = append(out, PreviewItem{
			N9EHash:         strings.TrimSpace(ev.Hash),
			N9EID:           ev.ID,
			GroupID:         ev.GroupID,
			RuleID:          ev.RuleID,
			Severity:        sev,
			Tags:            tags,
			RouteName:       rt.name,
			DedupKey:        dedupKey,
			ServiceHash:     serviceHash,
			GroupNameBefore: groupName,
			GroupNameAfter:  groupName2,
			RuleNameBefore:  ruleName,
			RuleNameAfter:   ruleName2,
			EntityBefore:    entityDisp,
			EntityAfter:     entityDisp2,
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
		s.LastMatched = 0
		s.LastDropped = 0
		s.LastUnmatched = 0
		s.NewActives = 0
		s.NewRecovereds = 0
		s.PurgedRecovered = 0
	})
	e.setPullDebug(PullDebug{})

	evs, total, err := e.n9e.FetchCurEvents(ctx, e.cfg.Pull)
	if err != nil {
		e.setStatus(func(s *Status) {
			s.LastEndUnix = time.Now().Unix()
			s.LastError = err.Error()
		})
		return state.ApplyResult{}, err
	}

	inputs, st2 := e.buildInputsWithStats(evs)

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
		s.LastMatched = st2.Matched
		s.LastDropped = st2.Dropped
		s.LastUnmatched = st2.Unmatched
		s.ActiveTotal = active
		s.RecoveredTotal = recovered
		s.NewActives = len(res.NewActives)
		s.NewRecovereds = len(res.NewRecovereds)
		s.PurgedRecovered = res.PurgedRecovered
	})
	e.setPullDebug(PullDebug{
		Fetched:   len(evs),
		Total:     total,
		Matched:   st2.Matched,
		Dropped:   st2.Dropped,
		Unmatched: st2.Unmatched,
		Routes:    st2.Routes,
	})

	log.Printf("pull done fetched=%d total=%d inputs=%d active=%d recovered=%d new_active=%d new_recovered=%d", len(evs), total, len(inputs), active, recovered, len(res.NewActives), len(res.NewRecovereds))
	return res, nil
}

func (e *Engine) setStatus(fn func(s *Status)) {
	e.statusMu.Lock()
	fn(&e.status)
	e.statusMu.Unlock()
}

func (e *Engine) setPullDebug(d PullDebug) {
	e.pullDbgMu.Lock()
	if d.Routes == nil {
		e.pullDbg = d
		e.pullDbgMu.Unlock()
		return
	}
	out := make(map[string]int, len(d.Routes))
	for k, v := range d.Routes {
		out[k] = v
	}
	d.Routes = out
	e.pullDbg = d
	e.pullDbgMu.Unlock()
}

type buildStats struct {
	Matched   int
	Dropped   int
	Unmatched int
	Routes    map[string]int
}

func (e *Engine) buildInputsWithStats(evs []n9e.CurEvent) ([]state.InputEvent, buildStats) {
	aggr := make(map[string]*state.InputEvent, len(evs))
	st := buildStats{Routes: map[string]int{}}

	for _, ev := range evs {
		groupName := strings.TrimSpace(ev.GroupName)
		ruleName := strings.TrimSpace(ev.RuleName)
		sev := ev.Severity

		rt := e.matchRoute(groupName, ruleName, sev)
		if rt == nil {
			st.Unmatched++
			continue
		}
		st.Matched++

		tags := tagsToMap(ev)
		groupName, ruleName, tags, dropped := applyProcessors(rt.processors, ev, groupName, ruleName, tags)
		if dropped {
			st.Dropped++
			continue
		}
		st.Routes[rt.name]++

		tags = applyTagRewrites(rt.rewrites, tags)
		entityKey, entityDisp := pickEntity(ev, tags, rt.cfg.Dedup.NormalizePodName)
		entityKey, entityDisp = applyProcessorsEntity(rt.processors, entityKey, entityDisp)

		groupName2 := applyRewrites(rt.rewrites, "group_name", groupName)
		ruleName2 := applyRewrites(rt.rewrites, "rule_name", ruleName)
		entityKey2 := applyRewrites(rt.rewrites, "entity", entityKey)
		entityDisp2 := applyRewrites(rt.rewrites, "entity", entityDisp)

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
				FirstTriggerTime: normalizeTs(ev.FirstTriggerTime),
				LastTriggerTime:  normalizeTs(ev.TriggerTime),
				RawCount:         1,
			}
			continue
		}

		it.RawCount++
		if it.FirstTriggerTime == 0 || (normalizeTs(ev.FirstTriggerTime) > 0 && normalizeTs(ev.FirstTriggerTime) < it.FirstTriggerTime) {
			it.FirstTriggerTime = normalizeTs(ev.FirstTriggerTime)
		}
		if normalizeTs(ev.TriggerTime) > it.LastTriggerTime {
			it.LastTriggerTime = normalizeTs(ev.TriggerTime)
		}
	}

	out := make([]state.InputEvent, 0, len(aggr))
	for _, v := range aggr {
		if v == nil {
			continue
		}
		out = append(out, *v)
	}
	return out, st
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

type dropEvalData struct {
	Event n9e.CurEvent
	Tags  map[string]string
	Group string
	Rule  string
	Sev   int
}

func compileProcessors(routeName string, pcs []config.ProcessorConfig) ([]compiledProcessor, error) {
	if len(pcs) == 0 {
		return nil, nil
	}
	out := make([]compiledProcessor, 0, len(pcs))
	for i := range pcs {
		pc := pcs[i]
		if !pc.Enabled {
			continue
		}
		typ := strings.ToLower(strings.TrimSpace(pc.Type))
		if typ == "" {
			continue
		}
		switch typ {
		case "drop":
			when := ""
			if pc.Drop != nil {
				when = strings.TrimSpace(pc.Drop.When)
			}
			if when == "" {
				continue
			}
			tmpl, err := template.New("drop_when").Option("missingkey=zero").Parse(when)
			if err != nil {
				return nil, fmt.Errorf("route %s processor[%d] drop.when: %w", routeName, i, err)
			}
			out = append(out, compiledProcessor{typ: typ, dropWhen: tmpl})
		case "relabel":
			if pc.Relabel == nil || len(pc.Relabel.Rules) == 0 {
				continue
			}
			rules := make([]compiledRelabelRule, 0, len(pc.Relabel.Rules))
			for j := range pc.Relabel.Rules {
				r := pc.Relabel.Rules[j]
				target := strings.TrimSpace(r.Target)
				pat := strings.TrimSpace(r.Pattern)
				if target == "" || pat == "" {
					continue
				}
				re, err := regexp.Compile(pat)
				if err != nil {
					return nil, fmt.Errorf("route %s processor[%d] relabel.rules[%d] pattern: %w", routeName, i, j, err)
				}
				rules = append(rules, compiledRelabelRule{target: target, re: re, replace: r.Replace})
			}
			if len(rules) == 0 {
				continue
			}
			out = append(out, compiledProcessor{typ: typ, relabel: rules})
		case "update":
			if pc.Update == nil || len(pc.Update.Sets) == 0 {
				continue
			}
			out = append(out, compiledProcessor{typ: typ, updates: pc.Update.Sets})
		default:
			continue
		}
	}
	return out, nil
}

func applyProcessors(pcs []compiledProcessor, ev n9e.CurEvent, groupName string, ruleName string, tags map[string]string) (string, string, map[string]string, bool) {
	if len(pcs) == 0 {
		return groupName, ruleName, tags, false
	}
	if tags == nil {
		tags = map[string]string{}
	}
	sev := ev.Severity
	for i := range pcs {
		p := pcs[i]
		switch p.typ {
		case "drop":
			if p.dropWhen == nil {
				continue
			}
			var b bytes.Buffer
			if err := p.dropWhen.Execute(&b, dropEvalData{Event: ev, Tags: tags, Group: groupName, Rule: ruleName, Sev: sev}); err != nil {
				continue
			}
			s := strings.TrimSpace(b.String())
			if s == "" {
				continue
			}
			if s == "1" || strings.EqualFold(s, "true") || strings.EqualFold(s, "yes") {
				return groupName, ruleName, tags, true
			}
		case "relabel":
			for _, rr := range p.relabel {
				applyRelabel(&groupName, &ruleName, tags, rr)
			}
		case "update":
			for _, u := range p.updates {
				applyUpdate(&groupName, &ruleName, tags, u)
			}
		}
	}
	return groupName, ruleName, tags, false
}

func applyProcessorsEntity(pcs []compiledProcessor, entityKey string, entityDisp string) (string, string) {
	if len(pcs) == 0 {
		return entityKey, entityDisp
	}
	for i := range pcs {
		p := pcs[i]
		switch p.typ {
		case "relabel":
			for _, rr := range p.relabel {
				if !strings.EqualFold(strings.TrimSpace(rr.target), "entity") {
					continue
				}
				entityKey = rr.re.ReplaceAllString(entityKey, rr.replace)
				entityDisp = rr.re.ReplaceAllString(entityDisp, rr.replace)
			}
		case "update":
			for _, u := range p.updates {
				if !strings.EqualFold(strings.TrimSpace(u.Field), "entity") {
					continue
				}
				entityKey = u.Value
				entityDisp = u.Value
			}
		}
	}
	return entityKey, entityDisp
}

func applyRelabel(groupName *string, ruleName *string, tags map[string]string, rr compiledRelabelRule) {
	target := strings.TrimSpace(rr.target)
	if target == "" || rr.re == nil {
		return
	}
	if strings.EqualFold(target, "group_name") {
		*groupName = rr.re.ReplaceAllString(*groupName, rr.replace)
		return
	}
	if strings.EqualFold(target, "rule_name") {
		*ruleName = rr.re.ReplaceAllString(*ruleName, rr.replace)
		return
	}
	if strings.HasPrefix(strings.ToLower(target), "tag:") {
		k := strings.TrimSpace(target[len("tag:"):])
		if k == "" {
			return
		}
		if tags == nil {
			return
		}
		v, ok := tags[k]
		if !ok {
			return
		}
		tags[k] = rr.re.ReplaceAllString(v, rr.replace)
		return
	}
}

func applyUpdate(groupName *string, ruleName *string, tags map[string]string, u config.UpdateSet) {
	field := strings.TrimSpace(u.Field)
	if field == "" {
		return
	}
	if strings.EqualFold(field, "group_name") {
		*groupName = u.Value
		return
	}
	if strings.EqualFold(field, "rule_name") {
		*ruleName = u.Value
		return
	}
	if strings.HasPrefix(strings.ToLower(field), "tag:") {
		k := strings.TrimSpace(field[len("tag:"):])
		if k == "" {
			return
		}
		if tags == nil {
			return
		}
		tags[k] = u.Value
		return
	}
}
