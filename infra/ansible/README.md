# Run the Ansible setup

Run these commands from the Ember devcontainer. It installs Python 3.13,
`ansible-core` 2.22.0, and the pinned collections when the container is created.
If needed, rebuild the devcontainer, then open a new terminal and check:

```sh
ansible --version
```

It should report `ansible-core 2.22.0`. The playbook requires core 2.19 or newer.

## Check and run the playbook

From the repository root:

```sh
cd /workspaces/ember/infra/ansible
ansible-galaxy collection install -r requirements.yml
```

Select the inventory for the intended host. For the supplied homelab inventory:

```sh
INVENTORY=inventories/homelab/hosts.yml
ansible-inventory -i "$INVENTORY" --graph
ansible-playbook -i "$INVENTORY" playbooks/site.yml --syntax-check
ansible -i "$INVENTORY" ember_hosts -m ansible.builtin.ping
```

Review the inventory target and connection settings before continuing. Check
mode contacts the target and is not a substitute for a disposable-host test:

```sh
ansible-playbook -i "$INVENTORY" playbooks/site.yml --check --diff
```

To apply the setup:

```sh
ansible-playbook -i "$INVENTORY" playbooks/site.yml
```

The playbook uses privilege escalation, applies a default-deny inbound UFW
policy while allowing SSH, installs Docker/Compose and Tailscale, builds or
pulls Ember's images, and starts Ember. It may make real changes to the
target. If SSH or privilege escalation credentials are prompted for, add
`--ask-pass` or `--ask-become-pass` as appropriate.

## Prebuilt images

`ember_image_source` chooses where Ember's images come from
([ADR 011](../../docs/decisions/011-prebuilt-registry-images.md)):

| Value | Behavior |
| --- | --- |
| `build` (role default) | Copies the source to the host and builds the three images there, rebuilding only when the inputs change. |
| `registry` (homelab inventory) | Installs only the Compose manifests and runs the images that CI publishes, through `deploy/compose.registry.yaml`. Every run pulls `ember_image_tag` again and replaces only the containers whose image changed. |

For every push to `main` that passes the tests, the `images` job in
`.github/workflows/build.yml` publishes `ghcr.io/<owner>/ember-api`,
`ember-web` and `ember-gateway` for amd64 and arm64. They are
tagged `sha-<full commit>`, and `latest` moves to the newest build. Only
committed and pushed source is published.

| Variable | Meaning |
| --- | --- |
| `ember_image_registry` | Image namespace, such as `ghcr.io/parityh4sher` (lowercase). |
| `ember_image_tag` | `latest` follows `main`. `sha-<full commit>` pins a build, or rolls back to one. |
| `ember_registry_username`, `ember_registry_token` | Only for private packages: a GitHub username and a classic token with `read:packages`, from a Vault file. |

New GHCR packages can start out private. After the first publish, open each
package under the repository owner's **Packages** tab on GitHub. Either make it
public (**Package settings → Change visibility**), or keep it private and
supply the token (DEC-008). The role then runs `docker login` on the host, which
stores the token unencrypted in `/root/.docker/config.json`.

Check what is running with `curl -s http://127.0.0.1:3000/api/healthz` on the
host: `version` is the deployed commit. Registry mode needs Docker Compose
2.24.4 or newer, which the role checks.

## Tailscale enrollment and Serve

The playbook installs Tailscale and manages the Serve route automatically for
an already-enrolled host. For a new host, choose either an auth key in Ansible
Vault (fully automated enrollment) or log in once manually.

To automate enrollment, create an auth key in the Tailscale admin console, then
create a Vault-encrypted vars file outside the repository:

```sh
mkdir -p ~/.config/ember
ansible-vault create ~/.config/ember/tailscale.yml
```

Put this in the editor that opens:

```yaml
tailscale_auth_key: "tskey-auth-REPLACE_WITH_YOUR_KEY"
```

Then pass the encrypted file when running the playbook:

```sh
ansible-playbook -i "$INVENTORY" playbooks/site.yml \
  -e "@$HOME/.config/ember/tailscale.yml" --ask-vault-pass
```

Ansible suppresses task output for the enrollment command so the auth key is
not printed in logs. Keep the Vault password and file private. Use a
pre-authorized auth key if the host must enroll without a separate admin
approval; a key obtained directly from the admin console is still a one-time
manual prerequisite.

If the host was enrolled already, simply rerun the playbook; it will configure
Serve without an auth key. Without a Vault key, the playbook reports the single
manual `sudo tailscale up` enrollment step and configures Serve on the next run.
The Serve route stays tailnet-only; do not enable Funnel for this use.

The gateway routes `/api/` to the API and every other path to the `web`
service, the single browser app. In `build` mode, Ansible stages its build
context from `apps/frontends/web/`. Each run removes containers of services that
`deploy/compose.yaml` no longer defines, such as the `home` and `notes`
frontends of earlier versions, and the staged `apps/frontends/home/`.

## Shared service stacks

Shared services such as Authentik, and the public ingress (Traefik and
cloudflared), are deployed by their own playbook, `playbooks/stacks.yml`. It
never touches Ember, and `site.yml` never touches the stacks (DEC-006). The
stack definitions and conventions are in [`infra/stacks`](../stacks/README.md).

The inventory chooses what to deploy (see
`inventories/homelab/group_vars/ember_hosts.yml`):

| Variable | Meaning |
| --- | --- |
| `shared_default_services` | Stacks that are always present, each as `{name, env}`; `env` becomes the stack's `.env`. |
| `shared_optional_services` | Optional integrations such as Immich; same format. |
| `ingress_domain` | Empty: nothing is public, and Tailscale Serve publishes Ember on the tailnet. A Cloudflare zone: Traefik and cloudflared **publish every labelled container on the internet** through a Cloudflare Tunnel. |
| `ember_public_host`, `authentik_public_host` | Public host names. They go into the Traefik labels (Ember's `EMBER_PUBLIC_HOST`, Authentik's `PUBLIC_HOST`), Ember's URLs and the OIDC issuer. Every route comes from container labels (see the [Traefik stack](../stacks/traefik/README.md#routing-with-labels)). |
| `ember_edge_enabled` | Put Ember's gateway on `edge` with Traefik labels for `ember_public_host` (`deploy/compose.edge.yaml`). `site.yml` refuses it unless Ember sign-in is on. The homelab inventory turns it on with the ingress. |

`stacks.yml` creates the internal Docker networks `tunnel` (cloudflared ↔
Traefik) and `edge` (Traefik ↔ routed services) before deploying any stack
(`roles/ingress_networks`).

Before deploying the ingress, `stacks.yml` requires Authentik's administrator
account to be claimed. On a fresh install, claim it over SSH as the failure
message explains, then rerun. Ember labels itself from `site.yml`, which
refuses to do so unless Ember sign-in is on.

Put operator secrets in the repository's **gitignored** `.env` (copy
`.env.example`, then `chmod 600 .env`). The playbook reads it on the controller,
so it also works inside the devcontainer:

```sh
CLOUDFLARE_TUNNEL_TOKEN=eyJ...
```

`stacks.yml` maps each known variable to a stack secret (`stack_secrets_from_env`
in the playbook) and installs it as a file on the host. It never puts the value
into a container's environment.

Alternatively, pass a Vault-encrypted vars file with `stack_secrets`, which
takes precedence over `.env`:

```sh
-e "@$HOME/.config/ember/stacks.yml" --ask-vault-pass
```

Secrets that a stack lists as generated, such as Authentik's database password
and secret key, are created on the host on the first run and never overwritten.
Then deploy:

```sh
ansible-playbook -i "$INVENTORY" playbooks/stacks.yml
```

Each stack lands in `/opt/stacks/<name>` as an ordinary Compose project named
`stack-<name>`. The playbook waits until its containers are healthy. It never
stops a stack or removes its volumes; do that yourself with `docker compose`
in the stack directory. After the first Authentik rollout, claim its
administrator account over SSH (see the
[Authentik stack](../stacks/authentik/README.md#first-start)). Tunnel setup is
described in the [cloudflared stack](../stacks/cloudflared/README.md).

### Ember sign-in through Authentik

With `ingress_domain` set and Authentik among the default services, the
inventory sets `ember_oidc_enabled` and derives the issuer and the redirect
URLs. `stacks.yml` registers the matching client in Authentik as a blueprint.
`site.yml` then writes Ember's `auth.json` (including the provider name
"Authentik" and the link to Authentik's user interface for the web app),
installs the shared client secret for the API, generates a Redis password, and
deploys the `deploy/compose.oidc.yaml` and `deploy/compose.redis.yaml` overlays.
Sign-in sessions are kept in that Redis container (ADR 014). It also stops
publishing Ember through Tailscale Serve, because sign-in only works on Ember's
public URL. Run `stacks.yml` first, then `site.yml`. Details are in the
[Authentik stack](../stacks/authentik/README.md#connecting-ember) and the
[sign-in guide](../../docs/authentication.md).
