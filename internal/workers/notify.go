package workers

import (
	"context"
	"log"
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
}

func NewNotifier(cfg config.Config, st *state.Store) *Notifier {
	timeout := time.Duration(cfg.N9E.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &Notifier{cfg: cfg, st: st, dt: dingtalk.New(timeout)}
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
		dtCfg := mergeDingTalk(n.cfg.DingTalk, rc.Notify.DingTalk)
		if strings.TrimSpace(dtCfg.Webhook) == "" {
			continue
		}

		maxLines := rc.DailyReport.MaxLines
		maxChars := rc.DailyReport.MaxChars

		activeItems := n.st.PickNotifyActive(routeName, now, rc.Notify.ObserveSeconds, rc.Notify.RepeatIntervalSeconds, maxLines)
		if len(activeItems) > 0 {
			title, text := report.BuildActiveMarkdown("N9E 告警通知", routeName, activeItems, len(activeItems), maxLines, maxChars)
			err := n.dt.SendMarkdown(dtCfg, title, text)
			if err != nil {
				log.Printf("notify active route=%s err=%v", routeName, err)
			} else {
				for _, it := range activeItems {
					_ = n.st.MarkNotified(it.ServiceHash, now)
				}
			}
		}

		if rc.Notify.SendRecovered {
			recItems := n.st.PickNotifyRecovered(routeName, now, maxLines)
			if len(recItems) > 0 {
				title, text := report.BuildRecoveredMarkdown("N9E 告警恢复", routeName, recItems, maxLines, maxChars)
				err := n.dt.SendMarkdown(dtCfg, title, text)
				if err != nil {
					log.Printf("notify recovered route=%s err=%v", routeName, err)
				} else {
					for _, it := range recItems {
						_ = n.st.MarkNotified(it.ServiceHash, now)
					}
				}
			}
		}
	}
}
