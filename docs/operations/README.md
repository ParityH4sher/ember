---
sidebar_label: Operations
sidebar_position: 3
---

# Operations

## Local Compose deployment

Requires Docker Engine with Compose v2 or compatible Podman Compose.
Run from the repository or extracted bundle root:

```sh
docker compose -f deploy/compose.yaml up --build -d
docker compose -f deploy/compose.yaml ps
docker compose -f deploy/compose.yaml logs -f
```

Open <http://127.0.0.1:3000>. To stop the stack:

```sh
docker compose -f deploy/compose.yaml down
```

For failed builds, proxy errors or second-instance conflicts see
[diagnosis and recovery scope](troubleshooting.md).

The current scaffold has no database or persistent application data. No backup
procedure is claimed until persistent storage is introduced.

## Ansible host provisioning

The initial roles target Debian-family Linux hosts. The supplied inventories
are test fixtures. The Ember role defaults to `/opt/ember` and host port
`3000`; group or host variables can override either setting.
For a second instance on one host, override **all three** of
`ember_install_dir`, `ember_http_port` and `ember_project_name`. Reusing
the first project's name tells Compose to reconcile the first stack.
Use a private inventory outside Git, key-based access, verified SSH host keys
and explicit deployment variables for an authorized disposable target. Install
compatible collections and validate syntax locally before any target check:

```sh
cd infra/ansible
ansible-galaxy collection install -r requirements.yml
ansible-playbook -i "$PRIVATE_INVENTORY" playbooks/site.yml --syntax-check
ansible-playbook -i "$PRIVATE_INVENTORY" playbooks/site.yml --check
```

Controller combination exercised against an Ubuntu 24.04.5 AMD64 guest:
`ansible-core` 2.22.0 (a dedicated Python 3.13+ environment), `community.docker`
5.4.0 and `community.general` 14.0.1. Install the versions pinned in
`requirements.yml`. The cached Bookworm devcontainer's Ansible 2.14 does not
meet those collections' support requirements; the playbook now fails early
before connecting to a host when run with an unsupported controller. The
container-runtime role also installs Ubuntu's `docker-buildx` package, removing
the Compose Bake fallback warning and providing the BuildKit builder. The
build-input fingerprint uses
`ansible.builtin.find`'s default checksum for change detection; it is not a
cryptographic integrity or authorization boundary.

The play applies a default-deny inbound UFW policy while allowing SSH, installs
the distribution Docker engine and Compose plugin, then builds and starts Ember.
Review inventory connectivity and firewall access before applying it remotely.
Check mode can contact the host and is not proof that provisioning is
idempotent. An actual provision run applies host changes and requires the
operator's authorization.

## GitHub Actions bundle

Download the `ember-compose-bundle` artifact from a successful **Build and test
Ember** workflow run. Extract it on the target host and run `./install.sh`. The
bundle contains source build contexts and Compose configuration; the host builds
images for its own architecture. It requires Docker Compose v2 or Podman Compose
and internet access to download base images.

Artifacts expire after 30 days. Durable, signed releases and upgrade/rollback
automation are not implemented.

Ember application services run under Compose. The project does not install or
manage systemd units. Kubernetes deployment is deferred until a real multi-node
scheduling requirement exists.

## Current operational limitations

- The API has no authentication; keep it behind loopback and do not expose it
  directly to an untrusted network.
- The Compose frontend is loopback-bound by default. Put a managed TLS reverse
  proxy in front before remote access.
- No database, backups, metrics, alerting, or task recovery is implemented yet.
- Ansible hardening is a starting baseline, not a full host security audit.
