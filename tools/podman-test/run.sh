#!/usr/bin/env bash
set -euo pipefail

IMAGE="${IMAGE:-crpi-n5rumpjwbqinoz4c.cn-hangzhou.personal.cr.aliyuncs.com/canary-keda/n9e-alter-service:v0.0.1}"
NAME="${NAME:-n9e-alter-service-local}"
PORT="${PORT:-18081}"
PUSH_TOKEN="${PUSH_TOKEN:-push-token-123}"
WAIT_SECONDS="${WAIT_SECONDS:-20}"
VOLUME_OPTS="${VOLUME_OPTS:-}"

N9E_BASE_URL="${N9E_BASE_URL:-}"
N9E_API_PATH="${N9E_API_PATH:-}"
N9E_USER_TOKEN="${N9E_USER_TOKEN:-}"
N9E_AUTHORIZATION="${N9E_AUTHORIZATION:-}"
N9E_TIMEOUT_SECONDS="${N9E_TIMEOUT_SECONDS:-}"

NO_PROXY_EXTRA="${NO_PROXY_EXTRA:-}"

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

if [[ -n "$N9E_TIMEOUT_SECONDS" ]] && [[ ! "$N9E_TIMEOUT_SECONDS" =~ ^[0-9]+$ ]]; then
  echo "invalid N9E_TIMEOUT_SECONDS=$N9E_TIMEOUT_SECONDS" >&2
  exit 2
fi

if [[ -n "$N9E_BASE_URL" ]] && [[ "$N9E_BASE_URL" != http://* ]] && [[ "$N9E_BASE_URL" != https://* ]]; then
  echo "invalid N9E_BASE_URL=$N9E_BASE_URL (must start with http:// or https://)" >&2
  exit 2
fi

podman rm -f "$NAME" >/dev/null 2>&1 || true

podman run -d \
  --name "$NAME" \
  -p "${PORT}:8080" \
  -v "${DATA_DIR}:/app/data${VOLUME_OPTS}" \
  -e HTTP_PROXY="" \
  -e HTTPS_PROXY="" \
  -e NO_PROXY="${NO_PROXY}${NO_PROXY:+,}${NO_PROXY_EXTRA}" \
  -e SERVICE_ADDR=":8080" \
  -e WEB_DIR="/app/web/dist" \
  -e DATA_DIR="/app/data" \
  -e STATE_REDIS_ENABLED="false" \
  -e N9E_BASE_URL="$N9E_BASE_URL" \
  -e N9E_API_PATH="$N9E_API_PATH" \
  -e N9E_USER_TOKEN="$N9E_USER_TOKEN" \
  -e N9E_AUTHORIZATION="$N9E_AUTHORIZATION" \
  -e N9E_TIMEOUT_SECONDS="$N9E_TIMEOUT_SECONDS" \
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
echo "status=http://127.0.0.1:${PORT}/api/v1/status"
echo "n9e_base_url=$N9E_BASE_URL"
echo "n9e_api_path=$N9E_API_PATH"
echo "n9e_timeout_seconds=$N9E_TIMEOUT_SECONDS"
echo "no_proxy=${NO_PROXY}${NO_PROXY:+,}${NO_PROXY_EXTRA}"
echo "tip=podman logs -f $NAME"
