---
sidebar_label: Sign-in (OIDC)
sidebar_position: 7
---

# Sign-in with OpenID Connect

Ember can require people to sign in through an OpenID Connect (OIDC) identity
provider. This implements the human sign-in part of
[DEC-002](planning/human-decisions.md#dec-002---identity-and-secret-custody):
the operator chooses the issuer (Authentik by default) and supplies its
settings in a configuration file.

Sign-in is **off by default**. Without a configuration file the API behaves as
before and has no authentication, so keep it on loopback.

## How it works

The Ember API is the OAuth 2.0 client. The browser never handles tokens.

1. The browser opens `/api/v1/auth/login`. Ember redirects to the provider using
   the authorization code flow with PKCE (S256), a `state` value bound to a
   short-lived cookie, and a `nonce`.
2. The provider redirects back to `/api/v1/auth/callback`. Ember redeems the code
   from the server and validates the ID token: its RS256/384/512 or
   ES256/384/512 signature against the provider's JWKS, the issuer, the audience
   and authorized party, expiry, issue time, and nonce. It then uses the
   user's groups to decide whether they may sign in and with which
   [role](#roles).
3. Ember stores the session on the server (in memory, or in
   [Redis](#session-store) when configured) and sets an opaque `HttpOnly`,
   `SameSite=Lax` cookie. Over HTTPS the cookie is also `Secure` and uses the
   `__Host-` prefix. Access and refresh tokens are discarded.

With sign-in configured, behavior changes as follows:

- Every API route except `/healthz` and `/v1/auth/*` needs a session.
  Unauthenticated requests receive `401 unauthenticated`.
- Changing plugin settings or activation needs the `admin` role. Other users
  receive `403 insufficient_role`.
- State-changing requests from another origin are refused with `403`, based on
  `Sec-Fetch-Site` and `Origin`.
- If the provider stops answering its discovery document, new sign-ins and
  signed-in access pause with `503` until it returns, as DEC-002 requires.
  Ember checks the provider at most every 30 seconds. `/healthz` stays up.
- Sessions last `sessionTtl` and are not renewed. Without Redis, restarting
  the API signs everyone out. With [Redis](#session-store), sessions survive
  API restarts.
- If the session store (Redis) does not answer, sign-in and signed-in access
  pause with `503 session_store_unavailable` until it returns. Ember neither
  signs anyone in nor clears their cookie meanwhile. `/healthz` stays up.
- **Sign out** removes the Ember session. If the provider advertises an
  `end_session_endpoint`, the browser is then sent there, and the provider
  decides what happens next (see `postLogoutRedirectUrl`). Authentik without
  `postLogoutRedirectUrl` shows its "You've logged out of Ember" page, which
  offers to return to Ember or to sign out of Authentik too.

In the web app, signed-out visitors are sent to `/login/`, which has a single
**Sign in with <providerName>** button and returns them to the page they asked
for. If the session ends while a page is open, the page stays and offers
**Sign in again**; while the session store or provider answers `503`, the app
shows a retry page instead of signing anyone out.
The user menu in the top bar offers **Sign out**, and **Profile & account**
(`/account/`) shows the name, email, groups, role and session expiry that
Ember received. The web app has no other sign-in method.

## Roles

Each signed-in person has one of two roles. Ember decides the role at sign-in
from the groups in the ID token:

| Role | Who | Allowed actions |
| --- | --- | --- |
| `admin` | Members of a group listed in `adminGroups` | Everything a user can do, plus configure, activate and deactivate plugins |
| `user` | Everyone else who may sign in (see `allowedGroups`) | View plugins and their non-secret settings, call operations of active plugins, use projects, notes and chats |

When `allowedGroups` is set, people in none of the listed groups and in no
admin group cannot sign in.

| Route | Least role |
| --- | --- |
| `PUT /v1/plugins/{pluginId}/configuration` | `admin` |
| `PUT /v1/plugins/{pluginId}/activation` | `admin` |
| Every other `/v1/` route, including `POST /v1/plugins/{pluginId}/api/{apiVersion}/{operationId}` | `user` |
| `/healthz`, `/v1/auth/*` | none |

- Admin groups admit their members by themselves. An admin does not also have
  to be in `allowedGroups`.
- Without `adminGroups`, nobody is an admin: no one can configure or activate
  plugins, and the API logs a warning at startup.
- The role is fixed for the whole session. A group change at the provider takes
  effect at the person's next sign-in. To withdraw admin rights at once, sign
  everyone out: restart the API when sessions are in memory, or
  [clear the session store](#session-store) when they are in Redis.
- **With sign-in off there are no identities, so every caller is an admin.**
  This is one more reason to keep an API without sign-in on loopback.
- The API enforces roles. The web app only hides the plugin settings
  and activation controls and the Admin page from users. `GET /api/v1/auth/session` reports the signed-in
  person's `role`.

## Configuration

Two files configure sign-in. Their paths are passed in environment variables:

| Variable | Contents |
| --- | --- |
| `EMBER_AUTH_CONFIG_FILE` | JSON configuration (below). Unset means sign-in is disabled. |
| `EMBER_OIDC_CLIENT_SECRET_FILE` | The client secret alone. A trailing newline is ignored. |
| `EMBER_REDIS_ADDR` | Optional `host:port` of the [session store](#session-store). Unset keeps sessions in memory. |
| `EMBER_REDIS_PASSWORD_FILE` | Optional Redis password alone, read like the client secret. Needs `EMBER_REDIS_ADDR`. |

The client secret is kept out of the JSON file so that it can follow the
secret-file rules in DEC-002: permission-restricted, mounted read-only into
the API only, and excluded from backups.

```json
{
  "issuer": "https://auth.example.com/application/o/ember/",
  "clientId": "ember",
  "redirectUrl": "https://ember.example.com/api/v1/auth/callback",
  "scopes": ["openid", "profile", "email"],
  "allowedGroups": ["ember-users"],
  "adminGroups": ["ember-admins"],
  "sessionTtl": "8h",
  "providerName": "Authentik",
  "accountUrl": "https://auth.example.com/if/user/"
}
```

| Field | Required | Default | Notes |
| --- | --- | --- | --- |
| `issuer` | yes | | Must match the `issuer` in the provider's discovery document exactly, including any trailing slash. |
| `clientId` | yes | | The OAuth client ID registered at the provider. |
| `redirectUrl` | yes | | The browser-visible callback URL, **including the gateway's `/api` prefix**. Register exactly this URL at the provider. Its origin is also the only origin allowed to send state-changing requests. |
| `postLogoutRedirectUrl` | no | | Sent to the provider's logout endpoint as `post_logout_redirect_uri`. Register it at the provider. Authentik then signs the person out of Ember only and redirects straight back here, skipping its page that offers to sign out of Authentik as well. Leave it unset to show that page. |
| `scopes` | no | `openid profile email` | Must include `openid`. |
| `tokenEndpointAuthMethod` | no | `client_secret_basic` with a secret, `none` without | Also accepts `client_secret_post`. |
| `groupsClaim` | no | `groups` | ID token claim that lists the user's groups. |
| `allowedGroups` | no | everyone | When set, only members of at least one listed group (or of an `adminGroups` group) may sign in. They get the `user` role. |
| `adminGroups` | no | nobody | Members of at least one listed group get the `admin` role and may change plugins. See [Roles](#roles). |
| `sessionTtl` | no | `8h` | A Go duration from `5m` to `720h`. |
| `providerName` | no | | The provider's display name, such as `Authentik`, at most 64 printable characters. The web app shows it on the sign-in button. |
| `accountUrl` | no | | Where people manage their own account at the provider, such as Authentik's `https://auth.example.com/if/user/`. The web app links to it. |

`GET /api/v1/auth/session` reports `providerName` and `accountUrl` as
`provider`, for signed-out callers too, so the sign-in page can use them. Each
member is left out when it is not configured, and `provider` is left out when
neither is:

```json
{"enabled": true, "authenticated": false,
 "provider": {"name": "Authentik", "accountUrl": "https://auth.example.com/if/user/"}}
```

All URLs must use `https`. Plain `http` is accepted only for loopback hosts
during development. The API refuses to start if the configuration is invalid
and reports which field is wrong.

## Session store

By default the API keeps sessions and pending sign-in attempts in memory.
Set `EMBER_REDIS_ADDR` to keep them in Redis instead
([ADR 014](decisions/014-redis-session-store.md)). They then survive API
restarts, and several API replicas could share them. Ember needs Redis 6.2 or
newer, or Valkey; the Compose overlay runs Redis 8.

- Keys are `ember:session:<SHA-256 of the cookie>` and
  `ember:pending:<SHA-256 of the state>`, so the stored data cannot be replayed
  as a cookie. Values are JSON (user, role, ID token, expiry). Redis expires
  each key when the session or attempt ends.
- The values include ID tokens. Protect the Redis volume like the client
  secret, and do not back it up: sessions are disposable, and restoring an
  empty store only means everyone signs in again.
- If Redis does not answer, Ember answers `503 session_store_unavailable`
  instead of signing people in or out, and the API keeps starting.
- `EMBER_REDIS_ADDR` without sign-in is a startup error, because Redis would
  be unused. So is `EMBER_REDIS_PASSWORD_FILE` without `EMBER_REDIS_ADDR`.
- The connection is plain RESP over TCP, without TLS. Keep Redis on a private
  network, as the Compose overlay does.

To sign everyone out, delete the session keys. With Compose, for example:

```sh
docker compose -f deploy/compose.yaml -f deploy/compose.oidc.yaml -f deploy/compose.redis.yaml \
  exec redis sh -c 'IFS= read -r REDISCLI_AUTH < /run/secrets/ember-redis-password; export REDISCLI_AUTH; redis-cli FLUSHALL'
```

## Provider setup

Create a **confidential** OIDC client using the authorization code flow, with
the redirect URI set to `redirectUrl`. Use an RS256 or ES256 signing key; HMAC
(HS256) ID tokens are not accepted. Include group names in the ID token if you
use `allowedGroups` or `adminGroups`.

- **Authentik:** create an *OAuth2/OpenID Provider* and an application. The
  issuer is `https://<authentik-host>/application/o/<application-slug>/`.
  Select a signing certificate, so that tokens are not signed with HS256.
  Ember ships an Authentik stack with step-by-step instructions for connecting
  Ember:
  [`infra/stacks/authentik`](https://github.com/ParityH4sher/ember/tree/main/infra/stacks/authentik).
- **Keycloak:** the issuer is `https://<keycloak-host>/realms/<realm>`. Add a
  *Group Membership* mapper if you use `allowedGroups` or `adminGroups`.

The API container must be able to reach the issuer under the same URL the
browser uses, including its DNS name and TLS certificate.

## Running with Compose

`deploy/compose.oidc.yaml` mounts both files read-only into the API container.
`deploy/compose.redis.yaml` adds the Redis session store: a `redis` service on
an internal network that only the API joins, with its data in the `redis-data`
volume. `install.sh` adds both overlays automatically when
`EMBER_AUTH_CONFIG_FILE` is set. Use absolute host paths:

```sh
export EMBER_AUTH_CONFIG_FILE=/etc/ember/auth.json
export EMBER_OIDC_CLIENT_SECRET_FILE=/etc/ember/secrets/oidc-client-secret
# Optional; defaults to redis-password next to the client secret.
export EMBER_REDIS_PASSWORD_FILE=/etc/ember/secrets/redis-password
./install.sh
```

When the Redis password file does not exist, `install.sh` generates it (64 hex
digits) and gives it to UID 65532 with mode `0440`. As root, or with rootless
Podman (`podman unshare`), this needs no help; otherwise the script prints the
`sudo chown` command to run before running it again. To write the file
yourself, use one line of letters, digits, `-` and `_`.

Without the installer, pass all three Compose files:
`docker compose -f deploy/compose.yaml -f deploy/compose.oidc.yaml -f deploy/compose.redis.yaml up -d`.
With only the first two, sessions stay in API memory.

The API runs as UID 65532, so the files must be readable by that user. Redis
runs as UID 999 with the supplementary group 65532, so the Redis password file
must also be group-readable (`0440`):

- **Docker:** `sudo chown 65532:65532 <file> && sudo chmod 0400 <file>`.
- **Rootless Podman:** `podman unshare chown 65532:65532 <file>`, then `chmod 0400 <file>`.
- **SELinux hosts:** the files also need a container-readable label, for
  example `chcon -t container_file_t <file>`.

Restart the stack after changing any of these files, because they are read
once at startup. After changing the Redis password, restart both `redis` and
`api`.

## Running with Ansible and the Authentik stack

The Ansible deployment automates all of this. Set `ingress_domain` in the
inventory, then run `playbooks/stacks.yml` followed by `playbooks/site.yml`:

- Authentik registers Ember's client from a generated blueprint.
- Ember gets a matching `auth.json` (with `providerName` `Authentik` and
  `accountUrl` `https://auth.<ingress_domain>/if/user/`), the client secret, a
  generated Redis password, and both overlays.

Setting `ingress_domain` publishes `ember.<domain>` and `auth.<domain>` on the
**public internet** through a Cloudflare Tunnel, and `stacks.yml` refuses to do
so while Ember sign-in is off. Do not put Cloudflare Access in front of
`auth.<domain>`, because the Ember API calls its OIDC endpoints
server-to-server. See
[`infra/stacks/authentik`](https://github.com/ParityH4sher/ember/tree/main/infra/stacks/authentik#connecting-ember)
and
[`infra/stacks/cloudflared`](https://github.com/ParityH4sher/ember/tree/main/infra/stacks/cloudflared#security).

`ember_oidc_admin_groups` and `ember_oidc_allowed_groups` become `adminGroups`
and `allowedGroups`. The groups must exist in Authentik: create them under
**Directory > Groups** and add people there. Authentik's `profile` scope puts
the user's group names in the `groups` claim, which is what Ember reads.

This setup was verified with a scripted browser against Authentik 2026.8.5.
The sign-in flow returned to Ember, which validated the ID token, created a
session with the user's name, email and groups, and honored `allowedGroups`.
Signing out returned Authentik's end-session URL. Roles (`adminGroups`) were
added later and have been tested only against the fake provider in
`apps/api/internal/auth/auth_test.go`, not yet against Authentik.

## Local development

Run the API on loopback with a loopback redirect URL, for example
`http://localhost:5173/api/v1/auth/callback` behind the Notes Vite dev server.
Then point the variables at local files:

```sh
cd apps/api
EMBER_AUTH_CONFIG_FILE=$HOME/.config/ember/auth.json \
EMBER_OIDC_CLIENT_SECRET_FILE=$HOME/.config/ember/oidc-client-secret \
go run .
```

## Limitations

- Roles apply to the whole instance. Every signed-in user can use every
  project and call every active plugin. Project ownership is not implemented
  yet.
- There are two fixed roles. Finer permissions, such as per-plugin or
  per-operation grants, are not implemented.
- Sessions live in API memory unless Redis is configured; the Compose and
  Ansible deployments configure it whenever sign-in is on. Plugin
  configuration always lives in API memory.
- A session cannot be revoked by itself: signing out removes it, and clearing
  the store removes all of them.
- Redis is reached without TLS, and the client does not support Redis Cluster
  or Sentinel. Under Redis `maxmemory` pressure, `volatile-ttl` evicts the
  entries closest to expiry first, which can sign out people whose sessions
  were about to end anyway.
- Sign-in covers people only. Workload credentials from DEC-002 are separate
  and not implemented.
- Requests refused before routing appear in the access log with the route `/`.
