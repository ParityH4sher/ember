---
sidebar_label: Operations
sidebar_position: 3
---

# Operations

## Local Compose deployment

Requires Docker Engine with Compose v2 or compatible Podman Compose:

```sh
docker compose -f deploy/compose.yaml up --build -d
docker compose -f deploy/compose.yaml ps
docker compose -f deploy/compose.yaml logs -f
```

Open <http://127.0.0.1:3000>. To stop the stack:

```sh
docker compose -f deploy/compose.yaml down
```

The current scaffold has no database or persistent application data. No backup
procedure is claimed until persistent storage is introduced.

## Ansible host provisioning

The initial roles target Debian-family Linux hosts. Edit the POC inventory before
running; its example IP is documentation-only. Install the collections and run:

```sh
cd infra/ansible
ansible-galaxy collection install -r requirements.yml
ansible-playbook playbooks/site.yml --check --diff
ansible-playbook playbooks/site.yml
```

The play applies a default-deny inbound UFW policy while allowing SSH, installs
the distribution Docker engine and Compose plugin, then builds and starts Ember.
Review inventory connectivity and firewall access before applying it remotely.
Use a disposable host for first validation.

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
