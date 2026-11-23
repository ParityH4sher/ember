#!/usr/bin/env bash
set -euo pipefail

venv="${HOME}/.venvs/ember-ansible"
python3 -m venv "${venv}"
"${venv}/bin/python" -m pip install --upgrade pip
# ansible-lint drives the VS Code Ansible extension's linting. The extension's
# Quick Links panel also expects ansible-creator, and reports "no Ansible
# environment" without it. Install them together so ansible-core stays pinned.
"${venv}/bin/python" -m pip install \
  "ansible-core==2.22.0" "ansible-lint==26.11.0" "ansible-creator==26.11.0"
"${venv}/bin/ansible-galaxy" collection install \
  -r infra/ansible/requirements.yml

echo "Ember Ansible environment ready:"
"${venv}/bin/ansible" --version
# ansible-lint warns about a "broken local setup" unless its venv is on PATH.
PATH="${venv}/bin:${PATH}" "${venv}/bin/ansible-lint" --version
"${venv}/bin/ansible-creator" --version
