#!/usr/bin/env bash
set -euo pipefail

bash .devcontainer/setup-ansible.sh
# New dependency volumes can be root-owned. Keep npm itself unprivileged.
sudo chown "$(id -u):$(id -g)" website/node_modules apps/frontends/web/{node_modules,.svelte-kit}
npm --prefix website ci --no-audit --no-fund
npm --prefix apps/frontends/web ci --no-audit --no-fund
npm --prefix apps/frontends/web run check
