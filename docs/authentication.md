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
   and authorized party, expiry, issue time, and nonce. It then applies the
   optional group policy.
3. Ember stores the session in memory and sets an opaque `HttpOnly`,
   `SameSite=Lax` cookie. Over HTTPS the cookie is also `Secure` and uses the
   `__Host-` prefix. Access and refresh tokens are discarded.

With sign-in configured, behavior changes as follows:

- Every API route except `/healthz` and `/v1/auth/*` needs a session.
  Unauthenticated requests receive `401 unauthenticated`.
- State-changing requests from another origin are refused with `403`, based on
  `Sec-Fetch-Site` and `Origin`.
- If the provider stops answering its discovery document, new sign-ins and
  signed-in access pause with `503` until it returns, as DEC-002 requires.
  Ember checks the provider at most every 30 seconds. `/healthz` stays up.
- Sessions last `sessionTtl` and are not renewed. Restarting the API signs
  everyone out.
- **Sign out** removes the Ember session. If the provider advertises an
  `end_session_endpoint`, the browser is then sent there to end the provider
  session as well.

The home page (`/`) and Project notes (`/apps/notes/`) show **Sign in** and
**Sign out** controls when sign-in is enabled.

## Configuration

Two files configure sign-in. Their paths are passed in environment variables:

| Variable | Contents |
| --- | --- |
| `EMBER_AUTH_CONFIG_FILE` | JSON configuration (below). Unset means sign-in is disabled. |
| `EMBER_OIDC_CLIENT_SECRET_FILE` | The client secret alone. A trailing newline is ignored. |

The client secret is kept out of the JSON file so that it can follow the
secret-file rules in DEC-002: permission-restricted, mounted read-only into
the API only, and excluded from backups.

```json
{
  "issuer": "https://auth.example.com/application/o/ember/",
  "clientId": "ember",
  "redirectUrl": "https://ember.example.com/api/v1/auth/callback",
  "postLogoutRedirectUrl": "https://ember.example.com/",
  "scopes": ["openid", "profile", "email"],
  "allowedGroups": ["ember-users"],
  "sessionTtl": "8h"
}
```

| Field | Required | Default | Notes |
| --- | --- | --- | --- |
| `issuer` | yes | | Must match the `issuer` in the provider's discovery document exactly, including any trailing slash. |
| `clientId` | yes | | The OAuth client ID registered at the provider. |
| `redirectUrl` | yes | | The browser-visible callback URL, **including the gateway's `/api` prefix**. Register exactly this URL at the provider. Its origin is also the only origin allowed to send state-changing requests. |
| `postLogoutRedirectUrl` | no | | Sent to the provider's logout endpoint. Register it at the provider. |
| `scopes` | no | `openid profile email` | Must include `openid`. |
| `tokenEndpointAuthMethod` | no | `client_secret_basic` with a secret, `none` without | Also accepts `client_secret_post`. |
| `groupsClaim` | no | `groups` | ID token claim that lists the user's groups. |
| `allowedGroups` | no | everyone | When set, only users in at least one listed group may sign in. |
| `sessionTtl` | no | `8h` | A Go duration from `5m` to `720h`. |

All URLs must use `https`. Plain `http` is accepted only for loopback hosts
during development. The API refuses to start if the configuration is invalid
and reports which field is wrong.

## Provider setup

Create a **confidential** OIDC client using the authorization code flow, with
the redirect URI set to `redirectUrl`. Use an RS256 or ES256 signing key; HMAC
(HS256) ID tokens are not accepted. Include group names in the ID token if you
use `allowedGroups`.

- **Authentik:** create an *OAuth2/OpenID Provider* and an application. The
  issuer is `https://<authentik-host>/application/o/<application-slug>/`.
  Select a signing certificate, so that tokens are not signed with HS256.
  Ember ships an Authentik stack with step-by-step instructions for connecting
  Ember:
  [`infra/stacks/authentik`](https://github.com/ParityH4sher/ember/tree/main/infra/stacks/authentik).
- **Keycloak:** the issuer is `https://<keycloak-host>/realms/<realm>`. Add a
  *Group Membership* mapper if you use `allowedGroups`.

The API container must be able to reach the issuer under the same URL the
browser uses, including its DNS name and TLS certificate.

## Running with Compose

`deploy/compose.oidc.yaml` mounts both files read-only into the API container.
`install.sh` adds it automatically when `EMBER_AUTH_CONFIG_FILE` is set. Use
absolute host paths:

```sh
export EMBER_AUTH_CONFIG_FILE=/etc/ember/auth.json
export EMBER_OIDC_CLIENT_SECRET_FILE=/etc/ember/secrets/oidc-client-secret
./install.sh
```

Without the installer, pass both Compose files:
`docker compose -f deploy/compose.yaml -f deploy/compose.oidc.yaml up -d`.

The API runs as UID 65532, so both files must be readable by that user:

- **Docker:** `sudo chown 65532:65532 <file> && sudo chmod 0400 <file>`.
- **Rootless Podman:** `podman unshare chown 65532:65532 <file>`, then `chmod 0400 <file>`.
- **SELinux hosts:** the files also need a container-readable label, for
  example `chcon -t container_file_t <file>`.

Restart the stack after changing either file, because both are read once at
startup.

## Running with Ansible and the Authentik stack

The Ansible deployment automates all of this. Set `ingress_domain` in the
inventory, then run `playbooks/stacks.yml` followed by `playbooks/site.yml`:

- Authentik registers Ember's client from a generated blueprint.
- Ember gets a matching `auth.json`, client secret, and the overlay.

Setting `ingress_domain` publishes `ember.<domain>` and `auth.<domain>` on the
**public internet** through a Cloudflare Tunnel, and `stacks.yml` refuses to do
so while Ember sign-in is off. Do not put Cloudflare Access in front of
`auth.<domain>`, because the Ember API calls its OIDC endpoints
server-to-server. See
[`infra/stacks/authentik`](https://github.com/ParityH4sher/ember/tree/main/infra/stacks/authentik#connecting-ember)
and
[`infra/stacks/cloudflared`](https://github.com/ParityH4sher/ember/tree/main/infra/stacks/cloudflared#security).

This setup was verified with a scripted browser against Authentik 2026.8.5.
The sign-in flow returned to Ember, which validated the ID token, created a
session with the user's name, email and groups, and honored `allowedGroups`.
Signing out returned Authentik's end-session URL.

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

- Every signed-in user can use every project and plugin. Project ownership and
  roles are not implemented yet.
- Sessions and plugin configuration live in API memory.
- Sign-in covers people only. Workload credentials from DEC-002 are separate
  and not implemented.
- Requests refused before routing appear in the access log with the route `/`.
