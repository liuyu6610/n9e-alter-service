package workers

import (
	"context"
	"log"
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
}

func NewDailyReporter(cfg config.Config, st *state.Store) *DailyReporter {
	timeout := time.Duration(cfg.N9E.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &DailyReporter{cfg: cfg, st: st, dt: dingtalk.New(timeout)}
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
		dtCfg := mergeDingTalk(d.cfg.DingTalk, rc.Notify.DingTalk)
		if strings.TrimSpace(dtCfg.Webhook) == "" {
			continue
		}

		sched, err := scheduler.Parse(rc.DailyReport.Cron)
		if err != nil {
			log.Printf("daily report route=%s cron invalid: %v", routeName, err)
			continue
		}

		rc2 := rc
		go d.runRoute(ctx, routeName, dtCfg, sched, rc2)
	}
}

func (d *DailyReporter) runRoute(ctx context.Context, routeName string, dtCfg dingtalk.Config, sched scheduler.Schedule, rc config.RouteConfig) {
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

		d.runOnce(routeName, dtCfg, rc)
	}
}

func (d *DailyReporter) runOnce(routeName string, dtCfg dingtalk.Config, rc config.RouteConfig) {
	items, total := d.st.List(state.StatusActive, routeName, 0, 100000)
	title, text := report.BuildActiveMarkdown(rc.DailyReport.TitlePrefix, routeName, items, total, rc.DailyReport.MaxLines, rc.DailyReport.MaxChars)
	if err := d.dt.SendMarkdown(dtCfg, title, text); err != nil {
		log.Printf("daily report route=%s err=%v", routeName, err)
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
