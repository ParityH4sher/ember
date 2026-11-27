#!/usr/bin/env bash
set -euo pipefail

if [[ ! -f deploy/compose.yaml || ! -f deploy/gateway/Dockerfile || ! -f apps/api/Dockerfile || ! -f apps/frontends/web/Dockerfile ]]; then
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
runtime=${compose[0]}
compose+=(-f deploy/compose.yaml)
# Optional OpenID Connect sign-in; see docs/authentication.md. Its sessions
# live in Redis (deploy/compose.redis.yaml), whose password file defaults to
# redis-password next to the client secret (or the configuration file).
if [[ -n "${EMBER_AUTH_CONFIG_FILE:-}" ]]; then
  compose+=(-f deploy/compose.oidc.yaml -f deploy/compose.redis.yaml)
  if [[ -z "${EMBER_REDIS_PASSWORD_FILE:-}" ]]; then
    EMBER_REDIS_PASSWORD_FILE="$(dirname -- "${EMBER_OIDC_CLIENT_SECRET_FILE:-$EMBER_AUTH_CONFIG_FILE}")/redis-password"
  fi
  if [[ "$EMBER_REDIS_PASSWORD_FILE" != /* ]]; then
    echo "EMBER_REDIS_PASSWORD_FILE must be an absolute host path." >&2
    exit 2
  fi
  export EMBER_REDIS_PASSWORD_FILE
fi
# Optional project chat; see docs/project-chat.md. The sandbox router runs on
# the host with -client-group, and only signed-in users may start its agents.
if [[ -n "${EMBER_AGENTS_CONFIG_FILE:-}" ]]; then
  if [[ -z "${EMBER_AUTH_CONFIG_FILE:-}" ]]; then
    echo "EMBER_AGENTS_CONFIG_FILE needs sign-in (EMBER_AUTH_CONFIG_FILE): chat runs start containers on this host." >&2
    exit 2
  fi
  if [[ "$EMBER_AGENTS_CONFIG_FILE" != /* || "${EMBER_ROUTER_SOCKET_DIR:-}" != /* ]]; then
    echo "Set EMBER_AGENTS_CONFIG_FILE and EMBER_ROUTER_SOCKET_DIR to absolute host paths." >&2
    exit 2
  fi
  if [[ ! -f "$EMBER_AGENTS_CONFIG_FILE" || ! -d "$EMBER_ROUTER_SOCKET_DIR" ]]; then
    echo "Create the agents file and the router socket directory first (docs/project-chat.md)." >&2
    exit 1
  fi
  # The API joins the group that owns the socket directory.
  EMBER_ROUTER_GID="${EMBER_ROUTER_GID:-$(stat -c %g -- "$EMBER_ROUTER_SOCKET_DIR")}"
  if [[ ! "$EMBER_ROUTER_GID" =~ ^[1-9][0-9]*$ ]]; then
    echo "EMBER_ROUTER_GID must be the router client group's numeric GID, not root's." >&2
    exit 2
  fi
  export EMBER_ROUTER_GID
  compose+=(-f deploy/compose.agents.yaml)
fi
if [[ $# -eq 1 && $1 == --check ]]; then
  "${compose[@]}" config --quiet
  echo "Ember Compose configuration is valid."
  exit 0
elif (( $# )); then
  echo "Usage: ./install.sh [--check]" >&2
  exit 2
fi

# Generate the Redis password once. The API (UID 65532) and Redis (UID 999 with
# supplementary group 65532) read it, so it is owned by 65532:65532, mode 0440.
# An existing file is left alone, like the OIDC client secret.
if [[ -n "${EMBER_REDIS_PASSWORD_FILE:-}" && ! -e "$EMBER_REDIS_PASSWORD_FILE" ]]; then
  dir=$(dirname -- "$EMBER_REDIS_PASSWORD_FILE")
  if [[ ! -d "$dir" ]]; then
    echo "Create $dir first, or set EMBER_REDIS_PASSWORD_FILE." >&2
    exit 1
  fi
  tmp=$(mktemp "$dir/.redis-password.XXXXXXXX")
  trap 'rm -f -- "$tmp"' EXIT
  # 32 random bytes as 64 hex digits; Redis accepts letters, digits, - and _.
  od -An -N32 -tx1 /dev/urandom | tr -d ' \n' > "$tmp"
  echo >> "$tmp"
  chmod 0440 "$tmp"
  if (( EUID == 0 )); then
    chown 65532:65532 "$tmp"
  elif [[ "$runtime" == podman ]]; then
    podman unshare chown 65532:65532 "$tmp"
  else
    echo "Generated a Redis password, but only root can give it to UID 65532. Run:" >&2
    echo "  sudo chown 65532:65532 '$tmp' && mv -- '$tmp' '$EMBER_REDIS_PASSWORD_FILE'" >&2
    echo "and then run install.sh again." >&2
    trap - EXIT
    exit 1
  fi
  mv -- "$tmp" "$EMBER_REDIS_PASSWORD_FILE"
  trap - EXIT
  echo "Generated the Redis password in $EMBER_REDIS_PASSWORD_FILE"
fi

# --remove-orphans stops services that this version no longer defines, such as
# the separate home and notes frontends of earlier bundles.
"${compose[@]}" up --build -d --remove-orphans
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
