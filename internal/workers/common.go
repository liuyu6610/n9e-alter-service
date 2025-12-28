package workers

import (
	"fmt"
	"strings"

	"n9e-alter-service/internal/config"
	"n9e-alter-service/internal/dingtalk"
)

func normalizeRouteName(name string, idx int) string {
	n := strings.TrimSpace(name)
	if n == "" {
		return fmt.Sprintf("route-%d", idx)
	}
	return n
}

func mergeDingTalk(global config.DingTalkConfig, override config.DingTalkConfig) dingtalk.Config {
	w := strings.TrimSpace(override.Webhook)
	if w == "" {
		w = strings.TrimSpace(global.Webhook)
	}
	s := strings.TrimSpace(override.Secret)
	if s == "" {
		s = strings.TrimSpace(global.Secret)
	}
	k := strings.TrimSpace(override.Keyword)
	if k == "" {
		k = strings.TrimSpace(global.Keyword)
	}
	return dingtalk.Config{Webhook: w, Secret: s, Keyword: k}
}
