#!/usr/bin/env bash
set -euo pipefail

venv="${HOME}/.venvs/ember-ansible"
python3 -m venv "${venv}"
"${venv}/bin/python" -m pip install --upgrade pip
"${venv}/bin/python" -m pip install "ansible-core==2.22.0"
"${venv}/bin/ansible-galaxy" collection install \
  -r infra/ansible/requirements.yml

echo "Ember Ansible environment ready:"
"${venv}/bin/ansible" --version
