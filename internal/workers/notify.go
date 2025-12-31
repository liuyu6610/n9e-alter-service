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
	"n9e-alter-service/internal/state"
)

type Notifier struct {
	cfg config.Config
	st  *state.Store
	dt  *dingtalk.Client
	rr  *RobotResolver
}

func NewNotifier(cfg config.Config, st *state.Store) *Notifier {
	timeout := time.Duration(cfg.N9E.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &Notifier{cfg: cfg, st: st, dt: dingtalk.New(timeout), rr: NewRobotResolver(cfg)}
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

	for i := range n.cfg.Routes {
		rc := n.cfg.Routes[i]
		if !rc.Enabled {
			continue
		}
		if !rc.Notify.Enabled {
			continue
		}

		routeName := normalizeRouteName(rc.Name, i)
		globalCfg := mergeDingTalk(n.cfg.DingTalk, rc.Notify.DingTalk)

		maxLines := rc.DailyReport.MaxLines
		maxChars := rc.DailyReport.MaxChars

		activeItems := n.st.PickNotifyActive(routeName, now, rc.Notify.ObserveSeconds, rc.Notify.RepeatIntervalSeconds, maxLines)
		if len(activeItems) > 0 {
			n.sendByRobots(routeName, rc.Notify.RobotID, globalCfg, activeItems, true, len(activeItems), maxLines, maxChars, now)
		}

		if rc.Notify.SendRecovered {
			recItems := n.st.PickNotifyRecovered(routeName, now, maxLines)
			if len(recItems) > 0 {
				n.sendByRobots(routeName, rc.Notify.RobotID, globalCfg, recItems, false, len(recItems), maxLines, maxChars, now)
			}
		}
	}
}

func (n *Notifier) sendByRobots(routeName string, routeRobotID string, globalCfg dingtalk.Config, items []state.Record, active bool, total int, maxLines int, maxChars int, now int64) {
	if len(items) == 0 {
		return
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
		for _, rr := range robots {
			dtCfg := rr.Cfg
			if strings.TrimSpace(dtCfg.Webhook) == "" {
				continue
			}
			var title, text string
			if active {
				title, text = report.BuildActiveMarkdown("N9E 告警通知", routeName, batch, total, maxLines, maxChars)
			} else {
				title, text = report.BuildRecoveredMarkdown("N9E 告警恢复", routeName, batch, maxLines, maxChars)
			}
			sent := true
			if err := n.dt.SendMarkdown(dtCfg, title, text); err != nil {
				sent = false
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
			for _, it := range batch {
				_ = n.st.MarkNotified(it.ServiceHash, now)
			}
		}
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
