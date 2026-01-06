#!/usr/bin/env bash
set -euo pipefail

BASE="${BASE:-http://127.0.0.1:18081}"
PUSH_TOKEN="${PUSH_TOKEN:-push-token-123}"
SKIP_INGEST="${SKIP_INGEST:-false}"
WAIT_SECONDS="${WAIT_SECONDS:-20}"

if ! command -v curl >/dev/null 2>&1; then
  echo "curl not found in PATH" >&2
  exit 127
fi
if ! command -v grep >/dev/null 2>&1; then
  echo "grep not found in PATH" >&2
  exit 127
fi

if [[ "$WAIT_SECONDS" -gt 0 ]]; then
  deadline=$((SECONDS + WAIT_SECONDS))
  while (( SECONDS < deadline )); do
    if curl -fsS --max-time 2 "${BASE}/readyz" >/dev/null 2>&1; then
      break
    fi
    sleep 0.5
  done
fi

health="$(curl -fsS "${BASE}/healthz")"
if [[ -z "$health" ]]; then
  echo "healthz empty" >&2
  exit 1
fi

echo "healthz=$health"

index="$(curl -fsS "${BASE}/")"
if [[ -z "$index" ]]; then
  echo "index empty" >&2
  exit 1
fi

js_path="$(printf '%s' "$index" | grep -Eo '/assets/[^" ]+\.js' | head -n1 || true)"
css_path="$(printf '%s' "$index" | grep -Eo '/assets/[^" ]+\.css' | head -n1 || true)"

if [[ -z "$js_path" || -z "$css_path" ]]; then
  echo "assets not found in index.html" >&2
  exit 1
fi

echo "js=$js_path"
echo "css=$css_path"

curl -fsSI --max-time 5 "${BASE}${js_path}" >/dev/null
curl -fsSI --max-time 5 "${BASE}${css_path}" >/dev/null

if [[ "$SKIP_INGEST" != "true" ]]; then
  now="$(date -u +%s)"
  body=$(cat <<EOF
{
  "id": 10001,
  "hash": "hash-demo-001",
  "rule_id": 20001,
  "rule_name": "demo_rule_cpu_high",
  "severity": 3,
  "group_id": 30001,
  "group_name": "demo_group",
  "target_ident": "host-01",
  "target_note": "",
  "first_trigger_time": ${now},
  "trigger_time": ${now},
  "cluster": "local",
  "tags_map": {
    "cluster": "local",
    "app": "demo",
    "instance": "host-01",
    "env": "dev"
  }
}
EOF
)

  resp="$(curl -fsS --max-time 10 -X POST "${BASE}/api/v1/events/ingest" \
    -H "Content-Type: application/json" \
    -H "X-Token: ${PUSH_TOKEN}" \
    --data "$body")"
  echo "ingest=$resp"
fi

echo "result=OK"
