#!/usr/bin/env bash
set -euo pipefail

if [[ ! -f deploy/compose.yaml || ! -d apps/api || ! -d apps/frontend ]]; then
  echo "Run install.sh from the extracted Ember Compose bundle directory." >&2
  exit 1
fi

if docker compose version >/dev/null 2>&1; then
  compose=(docker compose)
elif command -v podman >/dev/null && podman compose version >/dev/null 2>&1; then
  compose=(podman compose)
else
  echo "Docker Compose v2 or Podman Compose is required." >&2
  exit 1
fi

export EMBER_HTTP_PORT="${EMBER_HTTP_PORT:-3000}"
"${compose[@]}" -f deploy/compose.yaml up --build -d
echo "Ember started at http://127.0.0.1:${EMBER_HTTP_PORT}"
echo "Stop it with: ${compose[*]} -f deploy/compose.yaml down"
