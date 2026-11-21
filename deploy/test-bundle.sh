#!/usr/bin/env bash
# CI smoke test for an archive produced by this checkout. Requires Docker Compose,
# Python 3, curl and GNU sha256sum. Only use trusted CI-built archives.
set -euo pipefail
archive=${1:?usage: bash deploy/test-bundle.sh dist/ember-REVISION.tar.gz}
name=$(basename "$archive" .tar.gz)
tmp=$(mktemp -d)
project="ember-smoke-$(basename "$tmp" | tr '[:upper:].' '[:lower:]-')"
stack=0
cleanup() {
  if (( stack )); then
    docker compose -f deploy/compose.yaml down --timeout 5
  fi
  rm -rf -- "$tmp"
}
trap cleanup EXIT
cp -- "$archive" "$archive.sha256" "$tmp/"
cd "$tmp"
sha256sum --check "$name.tar.gz.sha256"
tar -xzf "$name.tar.gz"
cd "$name"
docker compose -f deploy/compose.yaml config --format json | python3 -c '
import json, pathlib, sys
services = json.load(sys.stdin)["services"]
for name in ("api", "frontend"):
    build = services[name]["build"]
    context = pathlib.Path(build["context"])
    assert context.resolve() == (pathlib.Path.cwd()/"apps"/name).resolve()
    assert (context/build.get("dockerfile", "Dockerfile")).is_file()
assert not services["api"].get("ports")
assert [p["host_ip"] for p in services["frontend"]["ports"]] == ["127.0.0.1"]
'
export COMPOSE_PROJECT_NAME="$project"
EMBER_HTTP_PORT=$(python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1", 0)); print(s.getsockname()[1])')
export EMBER_HTTP_PORT
stack=1
./install.sh
health() {
  # Publishing the socket can precede Nginx accepting its first connection.
  curl --noproxy '*' --fail --silent --show-error --retry 10 --retry-all-errors \
    --retry-delay 1 --retry-max-time 25 --max-time 3 --output "$tmp/health.json" --dump-header "$tmp/headers" \
    "http://127.0.0.1:$EMBER_HTTP_PORT/api/healthz"
  python3 -c '
import json, pathlib, sys
expected = pathlib.Path("apps/api/version.txt").read_text().strip()
assert expected and json.load(sys.stdin) == {"status": "ok", "version": expected}
' < "$tmp/health.json"
  grep -Eiq '^X-Request-ID: [[:alnum:]]+' "$tmp/headers"
}
health
# A service name survives replacement; a cached container IP may not.
docker compose -f deploy/compose.yaml up -d --no-build --no-deps --force-recreate api
health
