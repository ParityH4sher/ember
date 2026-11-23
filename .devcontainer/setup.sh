#!/usr/bin/env bash
set -euo pipefail

# The devcontainer feature build leaves /tmp at 0777, without the sticky bit,
# and Claude Code then refuses to create its cross-session messaging sockets
# there. Features install after the Containerfile, so restore the mode here.
sudo chmod 1777 /tmp
bash .devcontainer/setup-ansible.sh
# New dependency volumes can be root-owned. Keep npm itself unprivileged.
sudo chown "$(id -u):$(id -g)" website/node_modules apps/frontends/web/{node_modules,.svelte-kit} \
  "${CLAUDE_CONFIG_DIR:-${HOME}/.claude}"
npm --prefix website ci --no-audit --no-fund
npm --prefix apps/frontends/web ci --no-audit --no-fund
npm --prefix apps/frontends/web run check
