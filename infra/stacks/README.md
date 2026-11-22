# Shared service stacks

Each directory here is one independently deployed Compose project for a
**shared service**: layers 1 and 2 in
[shared services](../../docs/architecture/shared-services.md). Ember never
starts, stops, or upgrades these stacks, and neither do agents. The operator
does, outside Ember
([DEC-006](../../docs/planning/human-decisions.md#dec-006---lifecycle-authority-and-sharing)).
Deploying, reinstalling, or deleting Ember must never touch them.

| Stack | Role |
| --- | --- |
| [`authentik/`](authentik/README.md) | Identity provider for human sign-in (DEC-002) |
| [`traefik/`](traefik/README.md) | Routes public hostnames to containers on the internal `edge` network (curated routes plus opted-in labels) |
| [`cloudflared/`](cloudflared/README.md) | Cloudflare Tunnel connector: public subdomains → Traefik, outbound only |

`traefik` and `cloudflared` form the public ingress. They are deployed only when
the inventory sets `ingress_domain`, and only after the safety checks in
`playbooks/stacks.yml` pass.

## Conventions

- **Plain Compose.** `compose.yaml` must run with `docker compose up` and no
  Ansible. Set `name: stack-<directory>` so that the project, network, and
  volume names never collide with Ember or another stack.
- **Pinned images.** Pin each image by tag *and* multi-architecture index
  digest, and check that the index includes `linux/amd64` and `linux/arm64`
  (`skopeo inspect --raw docker://IMAGE:TAG`). Upgrade deliberately by changing
  both together, as `docs/engineering/testing-strategy.md` describes.
- **Secrets are files.** Declare them under the top-level `secrets:` key with
  `file: ./secrets/<name>`, and never put them in `compose.yaml`, `.env`, or the
  repository. List the secrets that may be random values in
  `x-ember-stack.generated-secrets`. Every other secret must come from the
  operator.
- **Non-secret settings** come from `.env`, interpolated by Compose. Document
  each variable in `.env.example`.
- **Private by default.** A web UI that Traefik should route joins the shared
  internal `edge` network with an alias (for example `aliases: [authentik]`);
  only that front container joins. Databases and caches stay on the stack's own
  network. A loopback port (`127.0.0.1:…`) is optional, for administration over
  SSH. Never mount the Docker socket; only Traefik's read-only socket proxy
  touches it.
- **Generated config.** A `templates/*.j2` file is rendered by Ansible with the
  inventory's variables to the same path without `.j2`. A template that renders
  empty, such as an optional feature that is off, is not installed, and an
  earlier copy is removed. Without Ansible, write the file by hand; the
  stack's README shows an example.

## Deploying with Ansible

`infra/ansible/playbooks/stacks.yml` deploys every stack listed in the
inventory under `shared_default_services` and `shared_optional_services`. When
`ingress_domain` is set, it then deploys the public ingress (Traefik and
cloudflared), but only if Ember sign-in is on and Authentik's administrator
account is claimed. It is separate from `site.yml`, which deploys Ember. See
the [Ansible guide](../ansible/README.md#shared-service-stacks).

For each stack, the `stack` role:

1. copies the stack to `/opt/stacks/<name>/`;
2. writes `.env` from the inventory;
3. installs secrets from the repository's gitignored `.env` (or a Vault file),
   or generates the allowed ones once and never overwrites them;
4. renders any templates;
5. runs `docker compose up` and waits until the containers are healthy.

It never removes volumes.

## Deploying by hand

```sh
sudo install -d -m 0750 /opt/stacks/authentik
sudo cp infra/stacks/authentik/compose.yaml /opt/stacks/authentik/
cd /opt/stacks/authentik
sudo install -d -m 0700 secrets
# Create each file listed under `secrets:` in compose.yaml (no trailing newline):
openssl rand -base64 48 | tr -d '\n' | sudo tee secrets/authentik_secret_key >/dev/null
sudo chmod 0444 secrets/*          # read by non-root container users; the directory stays 0700
sudo docker compose up -d --wait
```

## Adding a stack

1. Create `infra/stacks/<name>/` with `compose.yaml` (`name: stack-<name>`),
   `.env.example`, and a README covering data, backup, restore, and upgrades.
2. Add it to `shared_default_services` (cannot be deactivated in Ember) or
   `shared_optional_services` (an optional integration) in the inventory.
3. To publish it, put its front container on `edge` with an alias, then either
   add an `ingress_routes` entry (`url: http://<alias>:<port>`) or give the
   container Traefik labels (see the
   [Traefik stack](traefik/README.md#two-ways-to-route)). Make sure Cloudflare
   sends the subdomain to the tunnel. It is then reachable from the internet, so
   it must enforce its own authentication.

Each stack is backed up and restored on its own. A Docker volume is not a
backup; see each stack's README.
