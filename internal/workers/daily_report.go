package workers

import (
	"context"
	"log"
	"sort"
	"strings"
	"time"

	"n9e-alter-service/internal/config"
	"n9e-alter-service/internal/dingtalk"
	"n9e-alter-service/internal/report"
	"n9e-alter-service/internal/runtime"
	"n9e-alter-service/internal/scheduler"
	"n9e-alter-service/internal/state"
)

func findRouteConfig(cfg config.Config, routeName string) (config.RouteConfig, bool) {
	routeName = strings.TrimSpace(routeName)
	if routeName == "" {
		return config.RouteConfig{}, false
	}
	for i := range cfg.Routes {
		rc := cfg.Routes[i]
		name := normalizeRouteName(rc.Name, i)
		if strings.EqualFold(name, routeName) {
			return rc, true
		}
	}
	return config.RouteConfig{}, false
}

type DailyReporter struct {
	cfg config.Config
	rt  *runtime.Runtime
	st  *state.Store
	dt  *dingtalk.Client
}

func NewDailyReporter(cfg config.Config, st *state.Store) *DailyReporter {
	timeout := time.Duration(cfg.N9E.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &DailyReporter{cfg: cfg, st: st, dt: dingtalk.New(timeout)}
}

func NewDailyReporterWithRuntime(rt *runtime.Runtime, st *state.Store) *DailyReporter {
	snap := rt.Get()
	cfg := snap.Cfg
	timeout := time.Duration(cfg.N9E.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &DailyReporter{cfg: cfg, rt: rt, st: st, dt: dingtalk.New(timeout)}
}

func (d *DailyReporter) Start(ctx context.Context) {
	go d.reconcileLoop(ctx)
}

type dailyJob struct {
	cancel context.CancelFunc
	cron   string
}

func (d *DailyReporter) reconcileLoop(ctx context.Context) {
	jobs := map[string]dailyJob{}

	reconcile := func() {
		cfg := d.cfg
		if d.rt != nil {
			cfg = d.rt.Get().Cfg
		}

		desired := map[string]string{}
		for i := range cfg.Routes {
			rc := cfg.Routes[i]
			if !rc.Enabled {
				continue
			}
			if !rc.DailyReport.Enabled {
				continue
			}
			routeName := normalizeRouteName(rc.Name, i)
			cron := strings.TrimSpace(rc.DailyReport.Cron)
			if cron == "" {
				cron = "0 18 * * *"
			}
			desired[routeName] = cron
		}

		// stop removed
		for rn := range jobs {
			if _, ok := desired[rn]; !ok {
				j := jobs[rn]
				j.cancel()
				delete(jobs, rn)
			}
		}

		// start/update
		for rn, cron := range desired {
			j, ok := jobs[rn]
			if ok && strings.TrimSpace(j.cron) == strings.TrimSpace(cron) {
				continue
			}
			if ok {
				j.cancel()
				delete(jobs, rn)
			}

			sched, err := scheduler.Parse(cron)
			if err != nil {
				log.Printf("daily report route=%s cron invalid: %v", rn, err)
				continue
			}
			jobCtx, cancel := context.WithCancel(ctx)
			jobs[rn] = dailyJob{cancel: cancel, cron: cron}
			go d.runRoute(jobCtx, rn, sched)
		}
	}

	// initial
	reconcile()

	t := time.NewTicker(1 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			for _, j := range jobs {
				j.cancel()
			}
			return
		case <-t.C:
			reconcile()
		}
	}
}

func (d *DailyReporter) runRoute(ctx context.Context, routeName string, sched scheduler.Schedule) {
	for {
		next := sched.Next(time.Now())
		if next.IsZero() {
			return
		}

		delay := time.Until(next)
		timer := time.NewTimer(delay)

		select {
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return
		case <-timer.C:
		}

		d.runOnce(routeName, dingtalk.Config{}, config.RouteConfig{})
	}
}

func (d *DailyReporter) runOnce(routeName string, globalCfg dingtalk.Config, rc config.RouteConfig) {
	cfg := d.cfg
	if d.rt != nil {
		cfg = d.rt.Get().Cfg
	}
	rc2, ok := findRouteConfig(cfg, routeName)
	if !ok {
		return
	}
	if !rc2.Enabled || !rc2.DailyReport.Enabled {
		return
	}
	rc = rc2
	globalCfg = mergeDingTalk(cfg.DingTalk, rc.Notify.DingTalk)
	resolver := NewRobotResolver(cfg)

	items, total := d.st.List(state.StatusActive, routeName, 0, 100000)
	if len(items) == 0 {
		return
	}

	// 日报按记录选择机器人：同一日报路由内可能分流到不同群/机器人，避免“发错群”。
	by := map[string]*struct {
		robots []ResolvedRobot
		items  []state.Record
	}{}
	for _, it := range items {
		robots, _ := resolver.ResolveRobotsForRecord(it, routeName, rc.Notify.RobotID, globalCfg)
		cfgs := make([]dingtalk.Config, 0, len(robots))
		for i := range robots {
			cfgs = append(cfgs, robots[i].Cfg)
		}
		key := robotsKey(cfgs)
		b := by[key]
		if b == nil {
			b = &struct {
				robots []ResolvedRobot
				items  []state.Record
			}{robots: robots, items: make([]state.Record, 0, 8)}
			by[key] = b
		}
		b.items = append(b.items, it)
	}

	keys := make([]string, 0, len(by))
	for k := range by {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	sent := false
	for _, k := range keys {
		b := by[k]
		if b == nil || len(b.robots) == 0 {
			continue
		}
		for _, rb := range b.robots {
			dtCfg := rb.Cfg
			if strings.TrimSpace(dtCfg.Webhook) == "" {
				continue
			}
			title, text := report.BuildActiveMarkdown(rc.DailyReport.TitlePrefix, routeName, b.items, total, rc.DailyReport.MaxLines, rc.DailyReport.MaxChars)
			sentOne := true
			if err := d.dt.SendMarkdown(dtCfg, title, text); err != nil {
				sentOne = false
				log.Printf("daily report route=%s err=%v", routeName, err)
				fallbackIDs := resolver.FallbackIDs(rb.ID)
				if len(fallbackIDs) > 0 {
					for _, fid := range fallbackIDs {
						cfg2, ok := resolver.ConfigByID(fid)
						if !ok {
							continue
						}
						if err2 := d.dt.SendMarkdown(cfg2, title, text); err2 != nil {
							log.Printf("daily report fallback route=%s fid=%s err=%v", routeName, fid, err2)
							continue
						}
						sentOne = true
						break
					}
				}
			}
			if sentOne {
				sent = true
			}
		}
	}
	if !sent {
		return
	}

	mode := strings.TrimSpace(rc.DailyReport.ClearMode)
	if mode == "" {
		mode = "reset_notified"
	}

	switch mode {
	case "reset_notified":
		_ = d.st.ResetRouteDaily(routeName)
	case "clear_route":
		_ = d.st.ClearRoute(routeName)
	case "clear_all":
		_ = d.st.ClearAll()
	default:
		log.Printf("daily report route=%s unknown clear_mode=%s", routeName, mode)
		_ = d.st.ResetRouteDaily(routeName)
	}

	if err := d.st.SaveToFile(d.cfg.State.SnapshotFile); err != nil {
		log.Printf("daily report save snapshot route=%s err=%v", routeName, err)
	}
}
