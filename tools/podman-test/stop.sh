#!/usr/bin/env bash
set -euo pipefail

NAME="${NAME:-n9e-alter-service-local}"

if ! command -v podman >/dev/null 2>&1; then
  echo "podman not found in PATH" >&2
  exit 127
fi

podman rm -f "$NAME" >/dev/null 2>&1 || true

echo "removed=$NAME"
