# Authentik stack

[Authentik](https://goauthentik.io/) is the default identity provider for human
sign-in ([DEC-002](../../../docs/planning/human-decisions.md#dec-002---identity-and-secret-custody)).
It is a shared default service: the operator runs it, and Ember only uses it
as an OpenID Connect issuer ([sign-in guide](../../../docs/authentication.md)).

| Item | Value |
| --- | --- |
| Version | 2026.8.5 with PostgreSQL 16.16. Images are pinned by digest for amd64 and arm64. |
| Web UI | The `server` container is on the internal `edge` network, and its Traefik labels route `PUBLIC_HOST` (`auth.<ingress_domain>`) to its port 9000 while `PUBLIC_ROUTE` is `true`. `http://127.0.0.1:${HTTP_PORT}` (default 9000) on the host is for administration over SSH. |
| Data | Volumes `stack-authentik_database` (PostgreSQL) and `stack-authentik_data` (uploaded media) |
| Secrets | `secrets/authentik_secret_key`, `secrets/authentik_postgres_password` and `secrets/ember_oidc_client_secret`, generated once by Ansible |
| Blueprints | `blueprints/ember.yaml` (Ember's OIDC client), mounted into the worker at `/blueprints/custom` |

Differences from the upstream Compose file: secrets are read from files using
Authentik's `file://` configuration values, and the Docker socket is not
mounted. That means Authentik cannot create outposts automatically. The
embedded outpost still serves proxy providers. Containers run without Linux
capabilities and with `no-new-privileges`.

## First start

The first start runs database migrations and applies the default blueprints.
That takes a few minutes, longer on a Pi. Until `akadmin` has a password,
**anyone who can reach the UI can claim the administrator account**. For that
reason, `stacks.yml` refuses to publish anything through the tunnel while
`akadmin` has no usable password. It checks this inside Authentik, using the
same condition as Authentik's setup policy.

Authentik is reachable only on the host's loopback interface. Claim it over an
SSH tunnel:

```sh
ssh -L 9000:127.0.0.1:9000 <host>
# then open http://localhost:9000/ — it starts the initial setup
```

Then rerun `stacks.yml`. Keep the `akadmin` account (deactivating it is fine):
the publish check requires it to exist with a password. Enable MFA for
administrators before publishing.

## Connecting Ember

Ansible connects Ember automatically when `ember_oidc_enabled` is true. The
homelab inventory turns it on when `ingress_domain` is set and Authentik is a
default service. Both sides are generated from the same inventory values
(`ember_oidc_*`):

- **Authentik side (`stacks.yml`):** renders `templates/blueprints/ember.yaml.j2`
  into a blueprint. It creates a confidential OAuth2 provider named *Ember*
  (client ID `ember`, a strict redirect URI, RS256 self-signed signing key,
  claims included in the ID token) and an application with the slug
  `ember`. The client secret is the generated `secrets/ember_oidc_client_secret`,
  which the blueprint reads with `!File`. The issuer is therefore
  `https://auth.<ingress_domain>/application/o/ember/`.
- **Ember side (`site.yml`):** writes `/opt/ember/config/auth.json`, copies the
  same secret into `/opt/ember/config/oidc-client-secret` (owned by the API's
  UID 65532), and deploys the API with `deploy/compose.oidc.yaml`. It restarts
  the API when either file changes.
- **Network path:** the API reaches `auth.<ingress_domain>` like any other
  client, through Cloudflare and the tunnel. No firewall change is needed. Do
  not put Cloudflare Access in front of `auth.<ingress_domain>` (see the
  [cloudflared stack](../cloudflared/README.md#security)).

Run `stacks.yml` before `site.yml`, because Ember reads the secret that
Authentik generates. If Authentik runs on another host, set
`ember_oidc_client_secret` in your Vault file instead, and pass the same value
as `stack_secrets.authentik.ember_oidc_client_secret` there.

Turning sign-in off removes the blueprint file, but Authentik keeps the provider
and application it created. Delete them in the admin UI if you no longer want
them. Everyone in Authentik may sign in to Ember unless you set
`ember_oidc_allowed_groups`.

**Sign out** sends the browser to Authentik's end-session endpoint. With
`ember_oidc_post_logout_redirect_url` empty (the homelab default), Authentik
shows "You've logged out of Ember" and offers to sign out of Authentik too.
Setting it registers that URL as a logout URI and makes Authentik skip the
page and redirect straight back to Ember, while the Authentik session stays.

Ember gives the members of `ember_oidc_admin_groups` the `admin` role, which
may configure and activate plugins. Everyone else who signs in gets the `user`
role, which may only call plugin operations. The blueprint does not create
these groups: create them under **Directory > Groups** in the Authentik admin
UI and add members there. Ember reads the group names from the `groups` claim
that the `profile` scope adds. See
[Roles](../../../docs/authentication.md#roles).

## Backup and restore

A Docker volume is not a backup. Take a logical database dump:

```sh
cd /opt/stacks/authentik
sudo docker compose exec -T postgresql pg_dump -U authentik -d authentik | gzip > authentik-$(date +%F).sql.gz
```

Also back up the `stack-authentik_data` volume if you upload media. DEC-002
excludes the `secrets/` files from backups.

To restore after a rebuild:

1. Deploy the stack. New secrets are generated.
2. Stop `server` and `worker`.
3. Load the dump into the new database with `psql`.
4. Start the stack again.

Changing the secret key signs everyone out. Check Authentik's documentation
before restoring onto a different key.

## Upgrades

Upgrades are deliberate operator actions (DEC-006):

1. Read the Authentik release notes for every version you are moving past.
2. Take a database dump.
3. Change the tag and digest together in `compose.yaml`, both server and
   worker. Use `skopeo inspect --raw docker://ghcr.io/goauthentik/server:<tag>`
   and confirm that the index lists `linux/amd64` and `linux/arm64`.
4. Rerun `playbooks/stacks.yml`.

A PostgreSQL major-version upgrade needs a dump and restore. Do not edit the
generated secret files: the database password is set only when the database is
first initialised.
