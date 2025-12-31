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
	"n9e-alter-service/internal/scheduler"
	"n9e-alter-service/internal/state"
)

type DailyReporter struct {
	cfg config.Config
	st  *state.Store
	dt  *dingtalk.Client
	rr  *RobotResolver
}

func NewDailyReporter(cfg config.Config, st *state.Store) *DailyReporter {
	timeout := time.Duration(cfg.N9E.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &DailyReporter{cfg: cfg, st: st, dt: dingtalk.New(timeout), rr: NewRobotResolver(cfg)}
}

func (d *DailyReporter) Start(ctx context.Context) {
	for i := range d.cfg.Routes {
		rc := d.cfg.Routes[i]
		if !rc.Enabled {
			continue
		}
		if !rc.DailyReport.Enabled {
			continue
		}

		routeName := normalizeRouteName(rc.Name, i)
		globalCfg := mergeDingTalk(d.cfg.DingTalk, rc.Notify.DingTalk)

		sched, err := scheduler.Parse(rc.DailyReport.Cron)
		if err != nil {
			log.Printf("daily report route=%s cron invalid: %v", routeName, err)
			continue
		}

		rc2 := rc
		go d.runRoute(ctx, routeName, globalCfg, sched, rc2)
	}
}

func (d *DailyReporter) runRoute(ctx context.Context, routeName string, globalCfg dingtalk.Config, sched scheduler.Schedule, rc config.RouteConfig) {
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

		d.runOnce(routeName, globalCfg, rc)
	}
}

func (d *DailyReporter) runOnce(routeName string, globalCfg dingtalk.Config, rc config.RouteConfig) {
	items, total := d.st.List(state.StatusActive, routeName, 0, 100000)
	if len(items) == 0 {
		return
	}

	// 日报按记录选择机器人：同一日报路由内可能分流到不同群/机器人，避免“发错群”。
	by := map[string]*struct {
		robots []ResolvedRobot
		items []state.Record
	}{}
	for _, it := range items {
		robots, _ := d.rr.ResolveRobotsForRecord(it, routeName, rc.Notify.RobotID, globalCfg)
		cfgs := make([]dingtalk.Config, 0, len(robots))
		for i := range robots {
			cfgs = append(cfgs, robots[i].Cfg)
		}
		key := robotsKey(cfgs)
		b := by[key]
		if b == nil {
			b = &struct {
				robots []ResolvedRobot
				items []state.Record
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
		for _, rr := range b.robots {
			dtCfg := rr.Cfg
			if strings.TrimSpace(dtCfg.Webhook) == "" {
				continue
			}
			title, text := report.BuildActiveMarkdown(rc.DailyReport.TitlePrefix, routeName, b.items, total, rc.DailyReport.MaxLines, rc.DailyReport.MaxChars)
			sentOne := true
			if err := d.dt.SendMarkdown(dtCfg, title, text); err != nil {
				sentOne = false
				log.Printf("daily report route=%s err=%v", routeName, err)
				fallbackIDs := d.rr.FallbackIDs(rr.ID)
				if len(fallbackIDs) > 0 {
					for _, fid := range fallbackIDs {
						cfg2, ok := d.rr.ConfigByID(fid)
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
