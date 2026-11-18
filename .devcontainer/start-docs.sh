#!/usr/bin/env bash
set -euo pipefail

cd /workspaces/ember
npm --prefix website ci --no-audit --no-fund
exec npm --prefix website start
