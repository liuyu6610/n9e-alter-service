package workers

import (
	"regexp"
	"sort"
	"strings"

	"n9e-alter-service/internal/config"
	"n9e-alter-service/internal/dingtalk"
	"n9e-alter-service/internal/state"
)

type compiledBinding struct {
	cfg     config.BindingRule
	groupRe *regexp.Regexp
	ruleRe  *regexp.Regexp
	tagRe   map[string]*regexp.Regexp
}

type RobotResolver struct {
	robots   map[string]dingtalk.Config
	fallback map[string][]string
	bindings []compiledBinding
}

type ResolvedRobot struct {
	ID  string
	Cfg dingtalk.Config
}

func NewRobotResolver(cfg config.Config) *RobotResolver {
	r := &RobotResolver{robots: map[string]dingtalk.Config{}, fallback: map[string][]string{}}
	for _, rb := range cfg.Robots {
		id := strings.TrimSpace(rb.ID)
		if id == "" {
			continue
		}
		r.robots[id] = dingtalk.Config{Webhook: strings.TrimSpace(rb.Webhook), Secret: strings.TrimSpace(rb.Secret), Keyword: strings.TrimSpace(rb.Keyword)}
		if len(rb.FallbackRobotIDs) > 0 {
			out := make([]string, 0, len(rb.FallbackRobotIDs))
			seen := map[string]struct{}{}
			for _, v := range rb.FallbackRobotIDs {
				v = strings.TrimSpace(v)
				if v == "" {
					continue
				}
				if _, ok := seen[v]; ok {
					continue
				}
				out = append(out, v)
				seen[v] = struct{}{}
			}
			if len(out) > 0 {
				r.fallback[id] = out
			}
		}
	}

	compiled := make([]compiledBinding, 0, len(cfg.Bindings))
	for i := range cfg.Bindings {
		br := cfg.Bindings[i]
		if !br.Enabled {
			continue
		}
		cb := compiledBinding{cfg: br, tagRe: map[string]*regexp.Regexp{}}
		if strings.TrimSpace(br.GroupNameRegex) != "" {
			re, err := regexp.Compile(br.GroupNameRegex)
			if err != nil {
				continue
			}
			cb.groupRe = re
		}
		if strings.TrimSpace(br.RuleNameRegex) != "" {
			re, err := regexp.Compile(br.RuleNameRegex)
			if err != nil {
				continue
			}
			cb.ruleRe = re
		}
		for k, pat := range br.TagRegex {
			k = strings.TrimSpace(k)
			pat = strings.TrimSpace(pat)
			if k == "" || pat == "" {
				continue
			}
			re, err := regexp.Compile(pat)
			if err != nil {
				continue
			}
			cb.tagRe[k] = re
		}
		compiled = append(compiled, cb)
	}

	sort.SliceStable(compiled, func(i, j int) bool {
		pi, pj := compiled[i].cfg.Priority, compiled[j].cfg.Priority
		if pi == pj {
			return strings.TrimSpace(compiled[i].cfg.Name) < strings.TrimSpace(compiled[j].cfg.Name)
		}
		return pi > pj
	})
	r.bindings = compiled
	return r
}

func (r *RobotResolver) ResolveForRecord(rec state.Record, routeName string, routeRobotID string, global dingtalk.Config) ([]dingtalk.Config, []config.BindingRule) {
	cfgs, _, matched := r.resolve(rec, routeName, routeRobotID, global)
	return cfgs, matched
}

func (r *RobotResolver) ResolveIDsForRecord(rec state.Record, routeName string, routeRobotID string, global dingtalk.Config) ([]string, []config.BindingRule) {
	_, ids, matched := r.resolve(rec, routeName, routeRobotID, global)
	return ids, matched
}

func (r *RobotResolver) ResolveRobotsForRecord(rec state.Record, routeName string, routeRobotID string, global dingtalk.Config) ([]ResolvedRobot, []config.BindingRule) {
	cfgs, ids, matched := r.resolve(rec, routeName, routeRobotID, global)
	if len(cfgs) == 0 {
		return nil, matched
	}
	out := make([]ResolvedRobot, 0, len(cfgs))
	for i := range cfgs {
		id := ""
		if i < len(ids) {
			id = ids[i]
		}
		out = append(out, ResolvedRobot{ID: id, Cfg: cfgs[i]})
	}
	return out, matched
}

func (r *RobotResolver) FallbackIDs(robotID string) []string {
	robotID = strings.TrimSpace(robotID)
	if robotID == "" {
		return nil
	}
	ids := r.fallback[robotID]
	if len(ids) == 0 {
		return nil
	}
	out := make([]string, 0, len(ids))
	for _, v := range ids {
		v = strings.TrimSpace(v)
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}

func (r *RobotResolver) ConfigByID(robotID string) (dingtalk.Config, bool) {
	robotID = strings.TrimSpace(robotID)
	if robotID == "" {
		return dingtalk.Config{}, false
	}
	cfg, ok := r.robots[robotID]
	if !ok {
		return dingtalk.Config{}, false
	}
	if strings.TrimSpace(cfg.Webhook) == "" {
		return dingtalk.Config{}, false
	}
	return cfg, true
}

func (r *RobotResolver) resolve(rec state.Record, routeName string, routeRobotID string, global dingtalk.Config) ([]dingtalk.Config, []string, []config.BindingRule) {
	matched := make([]config.BindingRule, 0, 2)
	final := make([]dingtalk.Config, 0, 2)
	finalIDs := make([]string, 0, 2)
	seen := map[string]struct{}{}

	for _, cb := range r.bindings {
		if !bindingMatch(cb, rec, routeName) {
			continue
		}
		matched = append(matched, cb.cfg)
		for _, rid := range expandRobotIDs(cb.cfg) {
			if rid == "" {
				continue
			}
			if _, ok := seen[rid]; ok {
				continue
			}
			cfg, ok := r.robots[rid]
			if !ok {
				continue
			}
			if strings.TrimSpace(cfg.Webhook) == "" {
				continue
			}
			final = append(final, cfg)
			finalIDs = append(finalIDs, rid)
			seen[rid] = struct{}{}
		}
	}

	if len(final) > 0 {
		return final, finalIDs, matched
	}

	if rid := strings.TrimSpace(routeRobotID); rid != "" {
		if cfg, ok := r.robots[rid]; ok {
			if strings.TrimSpace(cfg.Webhook) != "" {
				return []dingtalk.Config{cfg}, []string{rid}, nil
			}
		}
	}

	if strings.TrimSpace(global.Webhook) != "" {
		return []dingtalk.Config{global}, nil, nil
	}
	return nil, nil, nil
}

func expandRobotIDs(br config.BindingRule) []string {
	if len(br.RobotIDs) > 0 {
		out := make([]string, 0, len(br.RobotIDs))
		for _, v := range br.RobotIDs {
			v = strings.TrimSpace(v)
			if v != "" {
				out = append(out, v)
			}
		}
		return out
	}
	if v := strings.TrimSpace(br.RobotID); v != "" {
		return []string{v}
	}
	return nil
}

func bindingMatch(cb compiledBinding, rec state.Record, routeName string) bool {
	br := cb.cfg
	if strings.TrimSpace(br.RouteName) != "" {
		if !strings.EqualFold(strings.TrimSpace(br.RouteName), strings.TrimSpace(routeName)) {
			return false
		}
	}
	if br.GroupID > 0 && br.GroupID != rec.GroupID {
		return false
	}
	if cb.groupRe != nil {
		if !cb.groupRe.MatchString(strings.TrimSpace(rec.GroupName)) {
			return false
		}
	}
	if br.RuleID > 0 && br.RuleID != rec.RuleID {
		return false
	}
	if cb.ruleRe != nil {
		if !cb.ruleRe.MatchString(strings.TrimSpace(rec.RuleName)) {
			return false
		}
	}

	for k, v := range br.Tags {
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if k == "" {
			continue
		}
		if rec.Tags == nil {
			return false
		}
		vv, ok := rec.Tags[k]
		if !ok {
			return false
		}
		if strings.TrimSpace(vv) != v {
			return false
		}
	}

	for k, re := range cb.tagRe {
		if k == "" || re == nil {
			continue
		}
		if rec.Tags == nil {
			return false
		}
		vv, ok := rec.Tags[k]
		if !ok {
			return false
		}
		if !re.MatchString(strings.TrimSpace(vv)) {
			return false
		}
	}

	ids := expandRobotIDs(br)
	return len(ids) > 0
}
