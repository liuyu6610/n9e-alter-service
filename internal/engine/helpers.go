package engine

import (
	"fmt"
	"regexp"
	"strings"

	"n9e-alter-service/internal/n9e"
)

var (
	podHashRe1 = regexp.MustCompile(`^(.+)-[0-9a-f]{9,10}-[0-9a-z]{5}$`)
	podHashRe2 = regexp.MustCompile(`^(.+)-[0-9a-f]{8,10}$`)
)

func tagsToMap(ev n9e.CurEvent) map[string]string {
	m := map[string]string{}
	if len(ev.TagsMap) > 0 {
		for k, v := range ev.TagsMap {
			ks := strings.TrimSpace(fmt.Sprint(k))
			if ks == "" {
				continue
			}
			m[ks] = strings.TrimSpace(fmt.Sprint(v))
		}
		return m
	}

	switch t := ev.Tags.(type) {
	case map[string]any:
		for k, v := range t {
			ks := strings.TrimSpace(k)
			if ks == "" {
				continue
			}
			m[ks] = strings.TrimSpace(fmt.Sprint(v))
		}
	case []any:
		parseTagsList(m, t)
	case []string:
		items := make([]any, 0, len(t))
		for _, x := range t {
			items = append(items, x)
		}
		parseTagsList(m, items)
	case string:
		parseTagsString(m, t)
	default:
		// ignore
	}

	return m
}

func parseTagsList(dst map[string]string, items []any) {
	for _, it := range items {
		s := strings.TrimSpace(fmt.Sprint(it))
		if s == "" {
			continue
		}
		if !strings.Contains(s, "=") {
			continue
		}
		kv := strings.SplitN(s, "=", 2)
		k := strings.TrimSpace(kv[0])
		v := ""
		if len(kv) > 1 {
			v = strings.TrimSpace(kv[1])
		}
		if k == "" {
			continue
		}
		dst[k] = v
	}
}

func parseTagsString(dst map[string]string, s string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return
	}
	var parts []string
	if strings.Contains(s, ",") {
		parts = strings.Split(s, ",")
	} else {
		parts = strings.Fields(s)
	}
	items := make([]any, 0, len(parts))
	for _, p := range parts {
		items = append(items, p)
	}
	parseTagsList(dst, items)
}

func pickEntity(ev n9e.CurEvent, tags map[string]string, normalizePod bool) (key string, display string) {
	podKeys := []string{"pod", "pod_name", "podname", "kubernetes_pod_name"}
	pod := ""
	for _, k := range podKeys {
		if v := strings.TrimSpace(tags[k]); v != "" {
			pod = v
			break
		}
	}

	ns := coalesce(tags["namespace"], tags["kubernetes_namespace"], tags["ns"])
	cluster := coalesce(strings.TrimSpace(ev.Cluster), tags["cluster"], tags["kubernetes_cluster"])

	if pod != "" {
		podKey := pod
		if normalizePod {
			podKey = normalizePodName(podKey)
		}
		display = podKey
		if ns != "" {
			display = ns + "/" + podKey
		}
		key = display
		if cluster != "" {
			key = cluster + "|" + display
			display = cluster + "/" + display
		}
		return
	}

	instance := coalesce(tags["instance"], tags["ip"], tags["host"])
	if instance != "" {
		display = instance
		key = display
		if cluster != "" {
			key = cluster + "|" + display
			display = cluster + "/" + display
		}
		return
	}

	target := coalesce(strings.TrimSpace(ev.TargetIdent), strings.TrimSpace(ev.TargetNote))
	if target == "" {
		target = fmt.Sprintf("%d", ev.ID)
	}
	display = target
	key = display
	if cluster != "" {
		key = cluster + "|" + display
		display = cluster + "/" + display
	}
	return
}

func normalizePodName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return name
	}
	if m := podHashRe1.FindStringSubmatch(name); len(m) > 1 {
		return m[1]
	}
	if m := podHashRe2.FindStringSubmatch(name); len(m) > 1 {
		return m[1]
	}
	return name
}

func coalesce(vals ...string) string {
	for _, v := range vals {
		vv := strings.TrimSpace(v)
		if vv != "" {
			return vv
		}
	}
	return ""
}
