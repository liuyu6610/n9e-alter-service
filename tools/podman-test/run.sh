#!/usr/bin/env bash
set -euo pipefail

IMAGE="${IMAGE:-crpi-n5rumpjwbqinoz4c.cn-hangzhou.personal.cr.aliyuncs.com/canary-keda/n9e-alter-service:v0.0.1}"
NAME="${NAME:-n9e-alter-service-local}"
PORT="${PORT:-18081}"
PUSH_TOKEN="${PUSH_TOKEN:-push-token-123}"
WAIT_SECONDS="${WAIT_SECONDS:-20}"
VOLUME_OPTS="${VOLUME_OPTS:-}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DATA_DIR="${DATA_DIR:-${SCRIPT_DIR}/data}"
mkdir -p "$DATA_DIR"

if ! command -v podman >/dev/null 2>&1; then
  echo "podman not found in PATH" >&2
  exit 127
fi

if [[ ! "$PORT" =~ ^[0-9]+$ ]]; then
  echo "invalid PORT=$PORT" >&2
  exit 2
fi

podman rm -f "$NAME" >/dev/null 2>&1 || true

podman run -d \
  --name "$NAME" \
  -p "${PORT}:8080" \
  -v "${DATA_DIR}:/app/data${VOLUME_OPTS}" \
  -e SERVICE_ADDR=":8080" \
  -e WEB_DIR="/app/web/dist" \
  -e DATA_DIR="/app/data" \
  -e STATE_REDIS_ENABLED="false" \
  -e PUSH_ENABLED="true" \
  -e PUSH_TOKEN="$PUSH_TOKEN" \
  "$IMAGE" >/dev/null

if [[ "$WAIT_SECONDS" -gt 0 ]] && command -v curl >/dev/null 2>&1; then
  deadline=$((SECONDS + WAIT_SECONDS))
  while (( SECONDS < deadline )); do
    if curl -fsS --max-time 2 "http://127.0.0.1:${PORT}/readyz" >/dev/null 2>&1; then
      break
    fi
    sleep 0.5
  done
fi

echo "name=$NAME"
echo "image=$IMAGE"
echo "ui=http://127.0.0.1:${PORT}/"
echo "healthz=http://127.0.0.1:${PORT}/healthz"
echo "tip=podman logs -f $NAME"
