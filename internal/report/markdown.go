package report

import (
	"fmt"
	"strings"
	"time"

	"n9e-alter-service/internal/state"
)

func BuildActiveMarkdown(titlePrefix string, routeName string, items []state.Record, total int, maxLines int, maxChars int) (string, string) {
	now := time.Now()
	title := strings.TrimSpace(titlePrefix)
	if title == "" {
		title = "N9E 告警日报"
	}
	title = fmt.Sprintf("%s %s", title, now.Format("2006-01-02"))
	if strings.TrimSpace(routeName) != "" {
		title = fmt.Sprintf("%s [%s]", title, strings.TrimSpace(routeName))
	}

	if maxLines <= 0 {
		maxLines = 50
	}
	if maxChars <= 0 {
		maxChars = 15000
	}

	sevCount := map[int]int{}
	for _, it := range items {
		sevCount[it.Severity] = sevCount[it.Severity] + 1
	}
	sevSummary := buildSeveritySummary(sevCount)

	lines := make([]string, 0, 32)
	lines = append(lines, "# "+title)
	lines = append(lines, "")
	lines = append(lines, "- 生成时间: "+now.Format("2006-01-02 15:04:05"))
	lines = append(lines, fmt.Sprintf("- 活跃告警(去重): %d", total))
	if sevSummary != "" {
		lines = append(lines, "- 严重级别分布: "+sevSummary)
	}
	lines = append(lines, "")

	show := items
	if len(show) > maxLines {
		show = show[:maxLines]
	}

	lines = append(lines, fmt.Sprintf("## 活跃告警明细(Top %d)", len(show)))
	lines = append(lines, "|严重|业务组|规则|对象|计数|首次触发|最新触发|")
	lines = append(lines, "|---|---|---|---|---:|---|---|")
	for _, it := range show {
		lines = append(lines, fmt.Sprintf("|P%d|%s|%s|%s|%d|%s|%s|",
			it.Severity,
			escapeCell(it.GroupName),
			escapeCell(it.RuleName),
			escapeCell(it.Entity),
			it.RawCount,
			fmtTs(it.FirstTriggerTime),
			fmtTs(it.LastTriggerTime),
		))
	}

	text := strings.Join(lines, "\n")
	if len(text) > maxChars {
		text = text[:maxChars-20] + "\n\n...(truncated)"
	}
	return title, text
}

func BuildRecoveredMarkdown(titlePrefix string, routeName string, items []state.Record, maxLines int, maxChars int) (string, string) {
	now := time.Now()
	title := strings.TrimSpace(titlePrefix)
	if title == "" {
		title = "N9E 告警恢复"
	}
	title = fmt.Sprintf("%s %s", title, now.Format("2006-01-02"))
	if strings.TrimSpace(routeName) != "" {
		title = fmt.Sprintf("%s [%s]", title, strings.TrimSpace(routeName))
	}

	if maxLines <= 0 {
		maxLines = 50
	}
	if maxChars <= 0 {
		maxChars = 15000
	}

	show := items
	if len(show) > maxLines {
		show = show[:maxLines]
	}

	lines := make([]string, 0, 32)
	lines = append(lines, "# "+title)
	lines = append(lines, "")
	lines = append(lines, "- 生成时间: "+now.Format("2006-01-02 15:04:05"))
	lines = append(lines, fmt.Sprintf("- 恢复条目: %d", len(items)))
	lines = append(lines, "")
	lines = append(lines, fmt.Sprintf("## 恢复明细(Top %d)", len(show)))
	lines = append(lines, "|严重|业务组|规则|对象|计数|恢复时间|")
	lines = append(lines, "|---|---|---|---|---:|---|")
	for _, it := range show {
		lines = append(lines, fmt.Sprintf("|P%d|%s|%s|%s|%d|%s|",
			it.Severity,
			escapeCell(it.GroupName),
			escapeCell(it.RuleName),
			escapeCell(it.Entity),
			it.RawCount,
			fmtTs(it.RecoveredAt),
		))
	}

	text := strings.Join(lines, "\n")
	if len(text) > maxChars {
		text = text[:maxChars-20] + "\n\n...(truncated)"
	}
	return title, text
}

func fmtTs(ts int64) string {
	if ts <= 0 {
		return ""
	}
	if ts > 1_000_000_000_000 {
		ts = ts / 1000
	}
	return time.Unix(ts, 0).Format("2006-01-02 15:04:05")
}

func escapeCell(s string) string {
	s = strings.ReplaceAll(s, "|", "\\|")
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}

func buildSeveritySummary(m map[int]int) string {
	if len(m) == 0 {
		return ""
	}
	keys := make([]int, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sortInts(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("P%d:%d", k, m[k]))
	}
	return strings.Join(parts, " ")
}

func sortInts(a []int) {
	for i := 0; i < len(a); i++ {
		for j := i + 1; j < len(a); j++ {
			if a[j] < a[i] {
				a[i], a[j] = a[j], a[i]
			}
		}
	}
}
