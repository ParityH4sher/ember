# Browser tests against real infrastructure

These tests drive a deployed Ember with Chromium, with nothing mocked: the
gateway, the API, the web container, the Redis session store, and sign-in
through a real Authentik. The mocked UI suite in `../ui` covers page
behaviour; this suite checks that the deployment is wired together.

## What it checks

| File | Checks |
| --- | --- |
| `stack.spec.ts` | `/api/healthz` through the gateway; sign-in enforced (`401`) and the provider named; slash redirects and the in-app 404; signed-out visits land on `/login/` with one button; cross-site writes refused; over HTTPS, `__Host-`, `Secure`, `HttpOnly` cookies |
| `signin.spec.ts` | Sign-in through Authentik returns to the requested page; the dashboard, notes and plugins load from the real API; a user gets "Admins only" and a `403` from the API; sign-out ends the session; an admin sees live instance data and plugin settings; optionally, a session survives an API restart |

Tests whose settings are missing are skipped, not failed.

## Test accounts

Use dedicated accounts, never a person's. With the Ansible deployment, set
`ember_e2e_users: true` in the inventory and run `playbooks/stacks.yml`. The
blueprint `infra/stacks/authentik/templates/blueprints/ember-e2e.yaml.j2` then
creates:

- `ember-e2e-user`, in the first `ember_oidc_allowed_groups` group if any;
- `ember-e2e-admin`, in the first `ember_oidc_admin_groups` group.

Both use the generated password in the Authentik stack's
`secrets/ember_e2e_password` on the host. Set `ember_e2e_users: false` and run
`stacks.yml` again to stop managing them; delete the users in Authentik
(**Directory → Users**, path `users/ember-e2e`) to remove them.

## Settings

Put them in the repository's root `.env` (gitignored, next to Ansible's
operator secrets; `.env.example` lists them under "Browser tests"):

```sh
cp -n .env.example .env && chmod 600 .env   # then fill in the EMBER_E2E_* lines
```

Only `EMBER_E2E_*` lines are read from that file; Ansible ignores them, and
the tests never see its other secrets. A variable already set in the shell
takes precedence, so `EMBER_E2E_BASE_URL=… npm run test:real` overrides it for
one run. `EMBER_E2E_ENV_FILE` points the suite at a different file.

| Variable | Required | Meaning |
| --- | --- | --- |
| `EMBER_E2E_BASE_URL` | yes | Ember's public origin, e.g. `https://ember.example.com` |
| `EMBER_E2E_USER`, `EMBER_E2E_PASSWORD` | for sign-in tests | A `user`-role account |
| `EMBER_E2E_ADMIN_USER`, `EMBER_E2E_ADMIN_PASSWORD` | for admin tests | An `admin`-role account; the password defaults to `EMBER_E2E_PASSWORD` |
| `EMBER_E2E_RESTART_API` | no | Shell command that restarts the API container, e.g. `podman restart ember-api-1`. Enables the session-survives-restart test. |
| `EMBER_E2E_IGNORE_HTTPS_ERRORS` | no | `1` accepts self-signed certificates |

## Running

From this devcontainer (Playwright and its browsers are in the image), or any
machine with `npm ci` done in `apps/frontends/web`:

```sh
cd apps/frontends/web
npm run test:real
```

On the Ember host itself, with only Podman or Docker, use
`scripts/test-real.sh`; it passes only the `EMBER_E2E_*` lines of `.env` into
the container. It runs the suite in the
official Playwright image with host networking, so it also reaches an Ember
that listens on loopback only. The restart test needs `npm run test:real` on
the host, because the container cannot restart other containers.

The HTML report is written to `playwright-report-real/`; traces and
screenshots of failures to `test-results-real/`.
