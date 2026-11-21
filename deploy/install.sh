#!/usr/bin/env bash
set -euo pipefail

if [[ ! -f deploy/compose.yaml || ! -f apps/api/Dockerfile || ! -f apps/frontend/Dockerfile ]]; then
  echo "Run install.sh from the extracted Ember Compose bundle directory." >&2
  exit 1
fi

export EMBER_HTTP_PORT="${EMBER_HTTP_PORT:-3000}"
if [[ ! "$EMBER_HTTP_PORT" =~ ^[1-9][0-9]{0,4}$ ]] || (( EMBER_HTTP_PORT > 65535 )); then
  echo "EMBER_HTTP_PORT must be a decimal port from 1 through 65535 (no leading zeros)." >&2
  exit 2
fi

case "${EMBER_COMPOSE_RUNTIME:-auto}" in
  auto)
    if docker compose version >/dev/null 2>&1; then
      compose=(docker compose)
    else
      compose=(podman compose)
    fi ;;
  docker|podman) compose=("$EMBER_COMPOSE_RUNTIME" compose) ;;
  *) echo "EMBER_COMPOSE_RUNTIME must be auto, docker or podman." >&2; exit 2 ;;
esac
if ! "${compose[@]}" version >/dev/null 2>&1; then
  echo "Docker Compose v2 or Podman with a Compose provider is required." >&2
  exit 1
fi
compose+=(-f deploy/compose.yaml)
if [[ $# -eq 1 && $1 == --check ]]; then
  "${compose[@]}" config --quiet
  echo "Ember Compose configuration is valid."
  exit 0
elif (( $# )); then
  echo "Usage: ./install.sh [--check]" >&2
  exit 2
fi

"${compose[@]}" up --build -d
# Check the real Nginx-to-API path; no shell or HTTP client is required in the
# distroless API image. A failing stack remains available for scoped diagnostics.
ready=false
for (( attempt=0; attempt<15; attempt++ )); do
  if "${compose[@]}" exec -T frontend wget -q -T 2 -O /dev/null http://127.0.0.1:8080/api/healthz; then
    ready=true
    break
  fi
  sleep 1
done
if [[ "$ready" != true ]]; then
  echo "Frontend did not reach the API; inspect: ${compose[*]} logs api frontend" >&2
  exit 1
fi
echo "Ember started at http://127.0.0.1:${EMBER_HTTP_PORT}"
echo "Stop it with: ${compose[*]} down"
