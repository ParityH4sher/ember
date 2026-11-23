#!/usr/bin/env bash
# Runs the real-infrastructure browser suite (tests/real) in the official
# Playwright container, so the host needs only Podman or Docker. Settings come
# from EMBER_E2E_* in the environment or in the repository's root .env; see
# tests/real/README.md. Only the EMBER_E2E_* lines of .env reach the
# container, never its other secrets.
#
#   EMBER_E2E_BASE_URL=https://ember.example.com \
#   EMBER_E2E_USER=ember-e2e-user EMBER_E2E_ADMIN_USER=ember-e2e-admin \
#   EMBER_E2E_PASSWORD="$(sudo cat /path/to/stack-authentik/secrets/ember_e2e_password)" \
#   apps/frontends/web/scripts/test-real.sh
#
# Extra arguments go to `playwright test`, e.g. `-g "admin"`.
set -euo pipefail

# Keep equal to @playwright/test in package.json.
image=mcr.microsoft.com/playwright:v1.64.1-noble
web=$(cd "$(dirname "$0")/.." && pwd)

root_env=$web/../../../.env
e2e_env=$(mktemp)
trap 'rm -f "$e2e_env"' EXIT
grep -E '^[[:space:]]*EMBER_E2E_[A-Z0-9_]*=' "$root_env" > "$e2e_env" 2>/dev/null || true
if [ -z "${EMBER_E2E_BASE_URL:-}" ] && ! grep -qE '^[[:space:]]*EMBER_E2E_BASE_URL=[[:space:]]*[^[:space:]]' "$e2e_env"; then
  echo "Set EMBER_E2E_BASE_URL, in the environment or in the repository's .env." >&2
  exit 1
fi
if [ -n "${EMBER_E2E_RESTART_API:-}" ] || grep -qE '^[[:space:]]*EMBER_E2E_RESTART_API=[[:space:]]*[^[:space:]]' "$e2e_env"; then
  echo "EMBER_E2E_RESTART_API is ignored in the container; run 'npm run test:real' on the host for the restart test." >&2
fi

runtime=${CONTAINER_RUNTIME:-$(command -v podman || command -v docker || true)}
[ -n "$runtime" ] || { echo "Install Podman or Docker, or run 'npm run test:real' with Playwright installed." >&2; exit 1; }

# Host networking reaches a loopback-bound Ember (Tailscale Serve setups).
# node_modules is a throwaway volume, so the host's own copy is left alone.
"$runtime" run --rm --init --ipc=host --network=host \
  --security-opt label=disable \
  -v "$web:/work" -v /work/node_modules -w /work \
  -v "$e2e_env:/run/ember-e2e.env:ro" -e EMBER_E2E_ENV_FILE=/run/ember-e2e.env \
  -e CI=1 -e EMBER_E2E_BASE_URL -e EMBER_E2E_USER -e EMBER_E2E_PASSWORD \
  -e EMBER_E2E_ADMIN_USER -e EMBER_E2E_ADMIN_PASSWORD -e EMBER_E2E_IGNORE_HTTPS_ERRORS \
  "$image" bash -c 'npm ci --no-audit --no-fund >/dev/null && npx playwright test -c playwright.real.config.ts "$@"' bash "$@"
