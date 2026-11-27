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
    docker compose -f deploy/compose.yaml down --volumes --timeout 5
  fi
  rm -rf -- "$tmp"
}
trap cleanup EXIT
# A failing grep or test prints nothing on its own; name the command.
trap 'echo "test-bundle.sh:$LINENO: failed: $BASH_COMMAND" >&2' ERR
cp -- "$archive" "$archive.sha256" "$tmp/"
cd "$tmp"
sha256sum --check "$name.tar.gz.sha256"
tar -xzf "$name.tar.gz"
cd "$name"
docker compose -f deploy/compose.yaml config --format json | python3 -c '
import json, pathlib, sys
services = json.load(sys.stdin)["services"]
assert sorted(services) == ["api", "frontend", "web"], sorted(services)
for name, directory in (("api", "apps/api"), ("web", "apps/frontends/web"), ("frontend", "deploy/gateway")):
    build = services[name]["build"]
    context = pathlib.Path(build["context"])
    assert context.resolve() == (pathlib.Path.cwd()/directory).resolve()
    assert (context/build.get("dockerfile", "Dockerfile")).is_file()
assert all(not services[s].get("ports") for s in ("api", "web"))
assert [p["host_ip"] for p in services["frontend"]["ports"]] == ["127.0.0.1"]
'
# Sign-in overlays: Redis is reachable only from the API, on an internal network.
EMBER_AUTH_CONFIG_FILE=/etc/ember/auth.json EMBER_OIDC_CLIENT_SECRET_FILE=/etc/ember/oidc-client-secret \
  EMBER_REDIS_PASSWORD_FILE=/etc/ember/redis-password \
  docker compose -f deploy/compose.yaml -f deploy/compose.oidc.yaml -f deploy/compose.redis.yaml config --format json |
  python3 -c '
import json, sys
config = json.load(sys.stdin)
services = config["services"]
assert sorted(services) == ["api", "frontend", "redis", "web"], sorted(services)
assert sorted(services["api"]["networks"]) == ["backend", "sessions"]
assert sorted(services["redis"]["networks"]) == ["sessions"]
assert all("sessions" not in services[s]["networks"] for s in ("frontend", "web"))
assert config["networks"]["sessions"]["internal"] is True
assert not services["redis"].get("ports")
assert "@sha256:" in services["redis"]["image"]
assert services["api"]["environment"]["EMBER_REDIS_ADDR"] == "redis:6379"
'
# Agents overlay: only the API joins the router's client group and sees its socket.
EMBER_AUTH_CONFIG_FILE=/etc/ember/auth.json EMBER_OIDC_CLIENT_SECRET_FILE=/etc/ember/oidc-client-secret \
  EMBER_REDIS_PASSWORD_FILE=/etc/ember/redis-password EMBER_AGENTS_CONFIG_FILE=/etc/ember/agents.json \
  EMBER_ROUTER_SOCKET_DIR=/var/lib/ember-router-socket EMBER_ROUTER_GID=4242 \
  docker compose -f deploy/compose.yaml -f deploy/compose.oidc.yaml -f deploy/compose.redis.yaml \
    -f deploy/compose.agents.yaml config --format json |
  python3 -c '
import json, sys
services = json.load(sys.stdin)["services"]
api = services["api"]
assert api["environment"]["EMBER_AGENTS_CONFIG_FILE"] == "/etc/ember/agents.json"
assert api["group_add"] == ["4242"], api["group_add"]
mounts = {v["target"]: v for v in api["volumes"]}
assert mounts["/run/ember-router"]["source"] == "/var/lib/ember-router-socket"
assert all(mounts[t]["read_only"] for t in ("/run/ember-router", "/etc/ember/agents.json"))
for name in ("frontend", "web", "redis"):
    assert "/run/ember-router" not in [v.get("target") for v in services[name].get("volumes", [])]
    assert not services[name].get("group_add") or name == "redis"
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
base="http://127.0.0.1:$EMBER_HTTP_PORT"
# One web app serves every page, the manifest and the service workers. Pages
# behind sign-in render in the browser, so only /login/ has a prerendered title.
# The installed app starts on the desktop surface (the manifest's start_url).
curl --noproxy '*' -fsS --retry 10 --retry-all-errors --retry-delay 1 "$base/" | grep -qF '__sveltekit_'
curl --noproxy '*' -fsS "$base/apps/notes/" | grep -qF '__sveltekit_'
curl --noproxy '*' -fsS "$base/desktop/" | grep -qF '__sveltekit_'
curl --noproxy '*' -fsS "$base/login/" | grep -qF '<title>Sign in · Ember</title>'
test "$(curl --noproxy '*' -sS -o /dev/null -w '%{http_code} %{redirect_url}' "$base/apps/notes?project=p")" \
  = "308 $base/apps/notes/?project=p"
test "$(curl --noproxy '*' -sS -o /dev/null -w '%{http_code} %{redirect_url}' "$base/login?returnTo=%2F")" \
  = "308 $base/login/?returnTo=%2F"
test "$(curl --noproxy '*' -sS -o /dev/null -w '%{http_code}' "$base/no-such-page")" = 404
curl --noproxy '*' -fsS "$base/manifest.webmanifest" |
  python3 -c 'import json, sys; m = json.load(sys.stdin); assert m["display"] == "standalone" and m["start_url"] == "./desktop/"'
curl --noproxy '*' -fsS "$base/service-worker.js" > /dev/null
curl --noproxy '*' -fsS "$base/apps/notes/service-worker.js" | grep -oF 'registration.unregister()'
# A service name survives replacement; a cached container IP may not.
docker compose -f deploy/compose.yaml up -d --no-build --no-deps --force-recreate api
health
