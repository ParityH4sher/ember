# Engineering log

## 2026-11-21 — architecture baseline (no implementation)

**Question:** what is broken before adding any backend feature? Inspected `git ls-files` (77 baseline files), `git log`, `apps`, `services`, `contracts`, `deploy`, `infra/ansible`, workflows, development tooling and product/architecture docs.

| Experiment | Actual observation |
| --- | --- |
| Cached devcontainer, `go version`, `go test -v ./...` in `apps/api` | Go 1.24.13. Test compilation failed on unused `net/http`; no baseline unit test ran. The host shell has no `go`. |
| `go build`, isolated listener/curl and `CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath` | AMD64 binary served health JSON, POST=405, unknown route=404; ARM64 ELF built, not executed on Pi. |
| `docker compose -f deploy/compose.yaml config --format json` | Both effective contexts lack Dockerfiles. Repeating with `--project-directory .` found both. `bash -n deploy/install.sh` passed. |
| Build and run a temporary private project using explicit project directory and unused loopback port | First Podman Compose attempt lacked its default socket. Docker Compose 5.6.1 against an ephemeral local rootless Podman socket built both images and proxied health correctly. Disposable stack removed. API image user 65532; frontend image has no default nonroot user. |
| `skopeo inspect --raw` for the three Dockerfile base image tags | Each registry manifest advertised linux/amd64 and linux/arm64. This does not measure Pi storage, memory or thermal headroom. |
| Offline inventory key inspection and `ansible-playbook ... --syntax-check` | POC lacks two required vars. Bundled collection lacks Compose v2 module. After installing declared collections in temporary storage, both inventories parsed with unsupported-core warnings. No host connection, provisioning, credential use or secret rotation. |
| Draft 2020-12 schema/fixture validation and path examples | Project schema and README example validated. Noncanonical paths can pass; no importer exists. |
| `npm --prefix website ci`, `DOCS_BASE_URL=/ember/ npm --prefix website run build` | Baseline and initial architecture revision built in isolated workspace. Published OpenAPI byte-equal to canonical source; decision heading anchors present. |
| `govulncheck -mode=binary` v1.1.4 against the same API source built using Go 1.24.13 and 1.27.4 | Baseline: exit 3, 21 standard-library findings. Patched build: exit 0, “No vulnerabilities found.” Not a penetration test or scan of the base OS. Temporary tooling only; no module changes. |
| `npm audit --package-lock-only --json` on isolated website manifests | Exit 1: 26 packages (3 low, 6 moderate, 1 high, 16 critical). Includes propagation to direct Docusaurus dependencies; not 26 confirmed reachable flaws in the static page. |

**Review:** runtime and tests differ; `go build` and `compose config` are necessary but insufficient. A Compose fix must account for Ansible's base path. Prioritize these delivery defects, then bounded HTTP lifecycle. New decisions and service authority remain pending.

### Bundle relocation check

Ran the unmodified `Assemble portable Compose bundle` workflow step in a temporary copy with the baseline commit SHA. The tar contained 11 regular files and an executable `install.sh` (0755). `sha256sum -c` succeeded under the CI working directory but failed from a fresh directory holding the same `.tar.gz` and `.sha256` side by side: the recorded filename includes `dist/`. This is a path error, not a digest mismatch. Add checksum verification from the consumer's working directory to the packaging milestone.

### ARM64 execution check

Built the unmodified `apps/api` using `CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath`. Launched the resulting static ELF with `qemu-aarch64` 10.2.2 inside a rootless, network-isolated devcontainer. An initial fixture mount did not expose `go.mod`; mounting the source and emulator separately corrected the setup. The first connection preceded listener startup; a bounded retry obtained `{"status":"ok","version":"development"}` and passed a decoded-body assertion. This exercises the ARM64 executable on an emulated CPU with the host kernel. It is not a Pi kernel, memory, latency, image or power-loss test.

### Two-instance restart and recreation

From 00:43:37 to 02:31:00 UTC, two independent Compose projects served the same baseline images on distinct loopback ports. Checked the proxied JSON on each at roughly one-minute intervals with a three-second request deadline. At 01:03:37 restarted only A's API; at 01:44:35 recreated it without recreating Nginx. B passed 104/104 samples. A passed the initial 20/20 then timed out on all 84 remaining samples. These observations cover sampled availability on one host, not identity isolation or continuous uptime. Both projects were torn down.

A separate short reproduction showed **why**: after restart the API had a new container IP, `getent hosts api` inside the frontend returned that new IP, and a fresh direct `wget` from the frontend to `http://api:8080/healthz` succeeded. The Nginx path timed out; reloading only Nginx restored it. The baseline static `proxy_pass` in `apps/frontend/nginx.conf` resolves the name when loading configuration. The API did not need a restart to recover. This adds proxy name refresh to the delivery milestone.

## Phase 2 — executable health contract

After the 02:34:06 UTC gate, fixed the test compilation issue and asserted the decoded health response matches the two public fields. Added negative routing coverage for unsupported method, unknown route and accidental direct `/api/` prefix. Only `apps/api/main_test.go` changed. Go 1.24.13 `go test ./...` passed; on Go 1.27.4, `gofmt -l` reported no changes, `go test -race ./...` and `go vet ./...` passed. No route or response changes. Rollback is test-only.

## Bundle relocation and installer smoke

Implemented ADR 008 with manifest-relative `../apps/*`, Ansible `project_src` pointing at `deploy/`, a basename checksum and a fresh-directory CI smoke (`deploy/test-bundle.sh`). The API remains unpublished and the frontend loopback-only. Rendered both CLI and Ansible-equivalent models successfully. Ran the workflow assembly step and smoke script against a temporary rootless engine: checksum, both Dockerfiles, real installer, proxied status and teardown passed. Commands and checksum verification in the installation docs match the new layout. Rollback the manifest and Ansible path together.

## Proxy recovery and API lifecycle

The proxy now refreshes the `api` upstream using Nginx's shared upstream zone, runtime-supplied DNS settings and a five-second validity period. Limited template substitution to the local resolvers variable. Preserved URL rewrite and loopback bind. The extracted-bundle smoke passed before and after replacing only the API; two transient 504s occurred before recovery. Unlike the baseline this required no manual frontend reload. Roll back proxy config and its image template setup together.

Added `run`/`serve` to the existing `apps/api` package: bind before logging readiness, explicit HTTP limits, SIGTERM/interrupt handling, five-second drain and forced close on timeout. Lifecycle messages use `slog` JSON. `server_test.go` synchronizes active requests with shutdown, verifies successful drain and timeout/cancellation, and exercises an occupied bind address. `go test -race`, `go vet`, formatting and Linux ARM64 cross-build passed with Go 1.27.4. No public route or new module dependency.

## Request telemetry, release inputs and provisioning defaults

Added API-generated `X-Request-ID` to the existing health response contract. `apps/api/accesslog.go` correlates it with JSON status, duration and route pattern. No caller-supplied ID, query, raw URI or authorization header goes into this access record. Tests check two different requests, redaction, correlation and status. Go race tests, vet and formatting passed. Contract-only merges now trigger the docs workflow.

Selected the Go 1.27.4 release builder by verified multiarchitecture index digest, kept the module language level at 1.24, and set CI to vet and race-test with the release compiler. Added `roles/ember/defaults/main.yml` for the two deployment variables; the homelab group can still override them. Fresh-collection syntax checks pass for both inventories but warn about the cached controller's unsupported version; no host provisioning was run. Next verification: source bundle health should report its own revision, not `development`.

## Reproducible packaging, source attribution and operational checks

`deploy/build-bundle.sh` packages only files from a selected Git commit, embeds its full hash, and normalizes archive order, modes, mtime, owner and gzip metadata. The Actions workflow calls the same script. In a scratch Git fixture two builds under different umasks produced byte-identical tarballs; an untracked fixture was excluded, and a conflicting checksum was refused without modifying it. The latest stamped bundle passed the native Docker install/recreate/health smoke. An initial smoke encountered an early connection reset; bounded idempotent probe retries now avoid that race. API unit tests also pass on the module's Go 1.24 minimum.

`deploy/install.sh --check` validates the selected provider, decimal port and Compose configuration before startup. Normal installation checks the actual proxy path. Invalid ports, extreme valid ports, Docker and Podman preflight were exercised. A frontend health check exposes ongoing proxy availability. The current Nginx stable release and distroless base now have multiarchitecture pins; the Nginx JSON access format omits raw request data. The docs workflow builds relevant pull requests without deploying, and verifies the hosted contract bytes beside the site.

## Provisioning qualification and convergence

An isolated Ubuntu 24.04.5 AMD64 VM, `ansible-core` 2.22.0, `community.docker` 5.4.0 and `community.general` 14.0.1 deployed both services via the real playbook. SSH remained available after UFW was enabled; `/api/healthz` succeeded on both runs. First run: 9 changed, 0 failed. Second: 1 changed, the unconditional Compose build. This is evidence of functionality, not idempotency. Updated the role to record build inputs only after healthy convergence, rebuild on input change and otherwise reconcile using the manifest policy. A new VM run checks zero-change repeat, failed-build retry and a separately named sibling. Collections are pinned to the qualified versions.

Using the local PyYAML and Draft 2020-12 validation tools, parsed the actual `contracts/openapi.yaml`, resolved its Health schema, and validated both its example and a live Go response. The HTTP request ID matched the API's JSON access event. SIGTERM to the binary resulted in exit 0 with an `api stopped` log. This is a conformance check of the current operation, not generic OpenAPI conformance certification.

## User-reported Ansible compatibility correction

User's controller rejected `ansible.builtin.find.checksum_algorithm`. Removed that option; `get_checksum: true` uses the built-in default on older Ansible too. This checksum detects deployment changes only and must not be treated as cryptographic provenance or an authorization check. Validate the same fingerprint/template output on the user's Ansible version before retrying all provisioning tasks.

Verified the adjusted `ansible.builtin.find` task on Ansible 2.14.18 in a network-isolated devcontainer using a read-only fixture: one file matched and a non-empty checksum was returned. This verifies the reported unsupported parameter is gone, not compatibility of the pinned `community.docker` 5.4 Compose module with Ansible 2.14. That collection requires a newer core; retry the full play with a supported controller (2.22.0 was qualified on Ubuntu 24.04) if another compatibility error occurs.

### Follow-up to user-reported Compose and collection warnings

Added Ubuntu's `docker-buildx` alongside the distro Docker/Compose packages. Compose had succeeded by falling back to the classic builder, but BuildKit features were unavailable. Added a localhost-only playbook preflight requiring ansible-core 2.19+, matching the pinned `community.general` 14.x and `community.docker` 5.x support ranges. An older controller now fails before connecting to or changing `pi`; the qualified controller remains 2.22.0. Target package installation and the preflight should be verified by the next isolated Ubuntu provisioning run.

## Prebuilt registry images for the Ansible deployment (2026-11-22)

Implemented [ADR 011](../decisions/011-prebuilt-registry-images.md):

- the `images` and `images-latest` jobs in `.github/workflows/build.yml`;
- `deploy/compose.registry.yaml`;
- `ember_image_source: build | registry` in the `ember` role;
- the homelab inventory switched to `registry` with `latest`;
- `FROM --platform=$BUILDPLATFORM` build stages in the API and notes
  Dockerfiles.

Verified on Fedora (Linux 7.2, x86_64), Docker Engine 29.10.0 with the
containerd image store, buildx 0.39.1, Compose 5.8.0:

- The workflow's build and retag commands, run by hand against a throwaway
  `registry:2` on loopback, built all four images for amd64 and arm64.
  - Only the build platform ran `npm ci` and the Go build.
  - The arm64 API binary is `ELF ... ARM aarch64`.
  - Each index carries the source annotation, and
    `docker buildx imagetools create` moved `latest`.
- `docker compose -f compose.yaml -f compose.registry.yaml config` from a
  directory holding only the manifests showed no `build` section. It fails
  with the overlay's message when `EMBER_IMAGE_REGISTRY` is unset.
- `up -d --pull always --no-build --wait` from that directory brought all four
  services to healthy.
  - `/api/healthz` reported the stamped version, and `/` and `/apps/notes/`
    returned 200.
  - After a new API build was retagged `latest`, a second `up` recreated only
    the API.
- The real `ember` role ran in a throwaway root container: ansible-core
  2.22.0, community.docker 5.4.0, Compose 5.7.1, the host's Docker socket,
  `-i localhost,`.
  - `site.yml --syntax-check` with the homelab inventory passed.
  - An invalid `ember_image_source` and an empty registry each failed at
    their assertion.
  - Registry mode deployed from manifests only. A rerun reported `ok`, a newer
    `latest` was rolled out, and `ember_image_tag: sha-test` rolled back.
  - Build mode, switching over from registry mode, rebuilt and served
    `development`. Its rerun reported `changed=0`.
  - The Compose version guard accepted `2.24.6+ds1-0ubuntu1~24.04.1`,
    `v2.27.0` and `5.7.1`, and rejected `2.20.2+ds1…` and unparsable output.
- `actionlint` 1.7.7, with shellcheck, reported nothing for `build.yml`.
- Every test project, volume, registry and image was removed afterwards.

Not run:

- the workflow on GitHub (the GHCR login, the `docker-container` builder with
  the pinned BuildKit v0.35.0, package linking and visibility);
- `deploy/test-bundle.sh` with the changed Dockerfiles;
- the private-package `docker login` path;
- any run against the Pi, so neither its Compose version nor an arm64 pull from
  GHCR is verified.

The classic Docker builder, used when `docker-buildx` is missing, does not
define `BUILDPLATFORM`; the `container-runtime` role installs `docker-buildx`.

To roll back, set `ember_image_source: build` in the inventory. The Dockerfile
change is independent.

### First publish from GitHub (2026-11-22)

Commit 755b647 on `main` published all four images. Read anonymously with
`docker buildx imagetools inspect ghcr.io/parityh4sher/ember-<service>:latest`,
each index:

- covers `linux/amd64` and `linux/arm64`, plus provenance attestations;
- carries `org.opencontainers.image.source=https://github.com/ParityH4sher/ember`
  and the commit as `org.opencontainers.image.revision`.

The packages are public (DEC-008).

With an empty Docker configuration, so the pull was anonymous, the following
ran from a directory holding only `compose.yaml` and `compose.registry.yaml`
on the same Fedora x86_64 workstation:

```sh
docker compose -f compose.yaml -f compose.registry.yaml up -d --pull always --no-build --wait
```

All four services became healthy. `/api/healthz` reported
`755b647bd4541e785ab262101c3972c924553ae5`, and `/` and `/apps/notes/`
returned 200. The test project, its volume and the pulled images were then
removed. Still not run: a `site.yml` deploy on the Pi (arm64 pull, the Compose
version guard against Ubuntu's package).

## `stacks_root` undefined after the stack loop (2026-11-22)

On the Pi, `stacks.yml` failed at "Check whether Authentik's administrator
account is claimed" with `'stacks_root' is undefined`. `stacks_root` is a
default of the `stack` role, which the playbook loads with `include_role`. An
included role's defaults stay private to it, so the playbook task after the
loop could not see them. Earlier local runs supplied `stacks_root` themselves,
which hid this. Fix: `public: true` on the stack loop's `include_role`.

Verified with ansible-core 2.22.0 and community.docker 5.4.0 in a root container
on Fedora x86_64 (Docker Engine 29.10.0), against the host's Docker daemon:

- A minimal role reproduces the exact error without `public` and resolves the
  default with `public: true`.
- The real `stacks.yml` ran with a localhost copy of the homelab inventory:
  `example.test`, Authentik on loopback port 9095, a temporary `stacks_root`,
  no `.env`, and `--skip-tags container-runtime`.
  - Run 1 deployed Authentik healthy. The check reported `unclaimed`, and the
    playbook stopped at the claim assertion as intended.
  - After setting a password for `akadmin` with `ak shell`, run 2 passed the
    check and deployed Traefik healthy. It then stopped at cloudflared's
    missing `cloudflare_tunnel_token`, expected because the test had no `.env`.
- The test projects, their volumes, the `tunnel` and `edge` networks, the
  scratch files and the pulled images were removed afterwards.

Not run: cloudflared with a real token, and anything on the Pi.

## Public ingress reachable on the homelab Pi (2026-11-22)

After the operator ran `stacks.yml` (twice, claiming `akadmin` in between) and
`site.yml`, and created the Cloudflare tunnel routes (`ember-poc` and `auth` →
HTTP `traefik:8081`), these requests from an outside workstation through
Cloudflare succeeded:

- `https://ember-poc.parityh4sher.org/` returned 200 with the Ember page.
- `/api/healthz` reported version `755b647…`, the GHCR image deployed by
  registry mode.
- `/api/v1/auth/session` returned `{"authenticated":false,"enabled":true}`.
- `https://auth.parityh4sher.org/` returned 302 to Authentik's authentication
  flow.
- `/application/o/ember/.well-known/openid-configuration` returned 200 with the
  expected issuer.

`auth` was tested with `curl --resolve`, because the LAN router still served a
cached NXDOMAIN from before the record existed. The zone's negative TTL is
1800 s. A browser sign-in round trip has not been tested.

The same stale NXDOMAIN also reached the Pi. Until it expired, Ember's
`/api/v1/auth/login` returned 503 `identity_provider_unavailable`, because the
API could not resolve its issuer. Restarting the router did not clear it, which
points to the router's upstream resolver. Once the router resolved `auth`
again, three successive login requests returned 302 to
`https://auth.parityh4sher.org/application/o/authorize/` with PKCE and the
expected callback URL. No code change was needed.

## Read-only Immich adapter (2026-11-23)

The Immich plugin was configuration only. It now has an adapter
(`apps/api/internal/plugins/immich`), version 1.1.0, API version 1, with two
operations:

- `server` reads `/api/server/version` and `/api/users/me`, which proves the
  key, and returns the version, the user's name and `isAdmin`;
- `albums` reads `/api/albums` and returns at most 200 albums, with id, name,
  asset count, shared flag, owner name and update time.

The key is sent only as `x-api-key`. Redirects are not followed. Responses are
capped at 1 MiB. Only selected fields are returned, never an email address or
a raw upstream body. The adapter re-checks the stored URL and key before any
request. `NewDeploymentHandler` registers it in place of the
configuration-only definition. The configuration schema is unchanged, so saved
settings and existing tests stay valid.

Verified on Fedora x86_64 in the pinned `golang:1.27.4` image, with the source
mounted read-only:

- `go vet ./...` passed;
- `go test -race -count=1 ./...` passed in every package;
- the new tests, all passing:
  - `TestServerInfoAndAlbums` (a subpath install, and no email address or key
    in results);
  - `TestAlbumsAreTruncated`;
  - `TestRejectsInvalidUpstreamResponses` (redirect, 401, oversized, not
    Immich, not JSON);
  - `TestRejectsUnsafeConfigurationWithoutCalling`;
  - `TestImmichPluginUsesConfiguredKey` (the key saved through the API reaches
    the adapter and never appears in a response; a rejected key gives 502);
- `gofmt -l .` listed only `internal/plugins/weather/weather.go`, which was
  already unformatted and is unchanged here.

Earlier, on the Pi, the operator's key for the dedicated non-admin account
`ember-poc` returned its user from `/api/users/me` and `[]` from
`/api/albums`. Immich is 3.4.2 at `https://immich.parityh4sher.org`.

Not run: the adapter against the real server, because a new image must be
published and deployed first, and the configuration re-entered after the
restart. The album field names (`albumName`, `assetCount`, `owner.name`) come
from Immich's API and have only been exercised against fixtures.

## Admin and user roles (2026-11-23)

**Change:** sign-in now gives each person a role from their ID token groups.
`adminGroups` gives `admin`; `allowedGroups` (or everyone, when it is unset)
gives `user`. Only admins may `PUT` plugin configuration or activation. Users
may read plugins and call operations of active plugins. Every route in
`internal/httpapi/server.go` names its least role. A request without a role
(one that did not pass through `auth.Wrap`) is refused with 403. With sign-in
off, `auth.Wrap` gives every caller `admin`, which keeps the previous
behavior. The home page hides settings and activation from users. The
contract, `docs/authentication.md`, `docs/plugins.md`, the `ember` Ansible role
(`ember_oidc_admin_groups`) and the homelab inventory (`[ember-admins]`) were
updated.

`auth.Wrap` now passes a copy of the request (`WithContext`), and the inner
mux sets `Pattern` on that copy. Without copying it back, every access-log
line would have named the route `/`. `serveAs` copies the pattern back, and
both `TestWrapKeepsTheAPIRoutePattern` and a `/v1/plugins` case in the
access-log test fail when the copy is removed.

Verification on Fedora (x86_64), host without Go, rootless Podman:

- `podman run golang:1.27.4@sha256:205c…26cf` in `apps/api`:
  `go vet ./... && go test -race -count=1 ./...` passed.
  `gofmt -l .` listed only `internal/plugins/weather/weather.go`, which was
  already unformatted and is unchanged here.
- Mutation checks on a copy: removing the pattern copy-back failed the two
  tests above, and registering the configuration `PUT` as `user` failed
  `TestPluginRoles`.
- New tests:
  - `TestGroupsDecideSignInAndRole` covers seven group/role combinations
    against the fake provider, including an admin outside `allowedGroups`;
  - `TestPluginRoles` checks that a user gets 403 on both `PUT`s without
    changing state, a user can list and invoke, and a request without a role
    gets 403 everywhere but `/healthz`;
  - `TestDisabledAuthPassesThrough` now also asserts the admin role.
- ansible-core 2.22.0 in a `python:3.13-slim` container rendered the role's
  `auth.json` expression, taken from `roles/ember/tasks/main.yml`, with
  `[ember-admins]` and with `[]`. `adminGroups` appeared only in the first.
  `TestParseConfig` accepts that shape.
- The home page's `account.js` and `integrations.js` were run in Node 24 with
  jsdom 26 against a mocked API. Admin and sign-in-off showed Settings and
  Activate/Deactivate. User showed only Operations, plus the "An Ember admin
  must configure and activate…" note on the inactive plugin.
- OpenAPI: YAML parses and every `$ref` resolves.
- `npm --prefix website ci && npm --prefix website run build` in a Node 24
  container, on a copy without `node_modules`, succeeded. It reported no
  broken links or anchors, including the new `authentication.md#roles` links.

Not run: sign-in against the real Authentik, so it is unconfirmed that a
member of `ember-admins` arrives with that group in the `groups` claim. The
earlier Authentik run did show groups in the session. Also not run: a real
browser, the bundle smoke test, `--syntax-check` of the playbooks, and a
deployment.

## Label-only Traefik routing (2026-11-23)

Implemented [ADR 012](../decisions/012-label-only-routing.md). The operator
wants every public route defined by labels on the containers themselves. Asked
to choose, the operator kept the read-only socket proxy rather than mounting
the Docker socket into Traefik.

- The curated `ingress_routes` are removed. Traefik's file provider now holds
  only the `public-https` middleware (`templates/dynamic/middlewares.yml.j2`).
- Ember's gateway labels itself in `deploy/compose.edge.yaml` with
  `EMBER_PUBLIC_HOST`. Its `ember-gateway` alias is gone.
- Authentik's server labels itself while `PUBLIC_ROUTE` is true, with
  `PUBLIC_HOST`. Its `authentik` alias is gone.
- The "no Ember without sign-in" check moved from `stacks.yml` to the `ember`
  role, where Ember gets labelled.
- `stacks.yml` checks each routed stack's `PUBLIC_HOST` and deletes the old
  `dynamic/routes.yml`.

Verified on Fedora x86_64 with Docker Engine 29.10.0, in a root container
running ansible-core 2.22.0, community.docker 5.4.0 and Compose 5.7.1, against
the host daemon. The inventory was a localhost copy of homelab: `example.test`,
loopback ports 9095 and 3096, a temporary `stacks_root`, no `.env`.

- `--syntax-check` passed for `site.yml` and `stacks.yml` with the homelab
  inventory.
- `stacks.yml` run 1: the `PUBLIC_HOST` check passed, Authentik became
  healthy, and the play stopped at the unclaimed-`akadmin` assertion.
- After `akadmin` got a password, run 2 did the following:
  - deleted a planted `dynamic/routes.yml`;
  - rendered `middlewares.yml`;
  - deployed Traefik healthy;
  - stopped at cloudflared's missing token, as expected.
- The `ember` role (registry images, edge, sign-in on) deployed Ember. With
  `-e ember_oidc_enabled=false`, it failed at the new assertion and deployed
  nothing.
- These requests went from a container on `tunnel` to `traefik:8081`, using
  only label routes:
  - Authentik's discovery returned 200 with issuer
    `https://auth.example.test/application/o/ember/`, so the middleware still
    applies;
  - Ember's `/`, `/apps/notes/`, `/api/healthz` (GHCR `8464612…`) and
    `/api/v1/auth/session` (`"enabled":true`) returned 200;
  - an unlabelled host returned 404.
- Traefik logged no errors.
- The test projects, volumes, networks, files and images were removed
  afterwards.

Not run: anything on the Pi, a real tunnel, and a second container claiming an
existing host (it is no longer prevented; see ADR 012).

## Sign out shows Authentik's logout page (2026-11-23)

**Problem (user report):** Sign out returned straight to Ember. Authentik's
"You've logged out of Ember" page, which offers to sign out of Authentik too,
never appeared, so the Authentik session stayed.

**Cause:** in authentik 2026.8.5 (`providers/oauth2/views/end_session.py` and
`SessionEndStage` in `flows/stage.py`, read at tag `version/2026.8.5`), a
registered `post_logout_redirect_uri` plus `id_token_hint` makes the session
end stage redirect to that URI instead of showing its page. Without the
parameter, the provider's invalidation flow shows the page. The homelab sent
`{{ ember_public_url }}/`. Ember's code was correct.

**Change:** the homelab inventory sets `ember_oidc_post_logout_redirect_url`
to `""`. Ember then omits the parameter, and the blueprint no longer registers
a logout URI. The docs explain the trade-off. `TestSignOutWithoutPostLogoutRedirect`
asserts that the logout URL has `id_token_hint` but no `post_logout_redirect_uri`.

Verification (Podman, Go 1.27.4 container): `go vet` and
`go test -race -count=1 ./...` passed. ansible-core 2.22.0 rendered the real
blueprint template and the role's `auth.json` expression with the empty value.
The blueprint lists only the callback URI, and `auth.json` has no
`postLogoutRedirectUrl`.

Not run: a sign-out against the homelab Authentik. That needs `stacks.yml`
(blueprint) and `site.yml` (`auth.json` and an API restart). No new image is
needed. The docs site build was not rerun; the docs change adds no links.

## Duplicate-key warnings from the Authentik stack file (2026-11-23)

**Problem (user report):** each `stacks.yml` run printed "Found duplicate
mapping key 'secrets'" and "'volumes'" at the stack role's "Read the stack's
declared secrets and settings" task.

**Cause:** in `infra/stacks/authentik/compose.yaml`, `worker` merged
`<<: *authentik` and then overrode `secrets` and `volumes` from it. That is
valid YAML, and Compose applies it as intended. Ansible's `from_yaml` expands
the merge first and then reports the overriding keys as duplicates, keeping
the last value, which is the intended one. The role reads only the top-level
`secrets` and `x-ember-stack`, so nothing went wrong, but the warnings recurred
on every run, and setting `DUPLICATE_YAML_DICT_KEY=error` would stop the play.

**Change:** `secrets` and `volumes` left the `x-authentik` anchor. `server` and
`worker` each list their own.

Verification:

- Docker Compose 5.8.0 ran `config --format json` on the committed and the
  edited file, with `PUBLIC_HOST` and `PUBLIC_ROUTE` set. `services`,
  `volumes`, `secrets` and `networks` were identical; only the ignored
  `x-authentik` extension differed.
- ansible-core 2.22.0 (`python:3.13-slim` container) evaluated the role's
  `lookup('ansible.builtin.file', …) | from_yaml` on both files. The committed
  file produced both warnings; the edited file produced none, with the same
  top-level secrets.

Not run: `stacks.yml` against the Pi. The services are unchanged, so Compose
should not recreate the containers.

## ansible-lint and ansible-creator in the devcontainer (2026-11-23)

**Problem (user report):** in the devcontainer, the VS Code Ansible extension
(`redhat.ansible` 26.11.0) showed "Looks like you don't have an Ansible
environment set up yet". Its Quick Links panel (`getSystemDetails` in
`dist/extension/extension.js`) shows this when any of `ansible --version`,
`python3 --version` or `ansible-creator --version` fails. The extension host
had the venv first on `PATH`, so Ansible and Python were found. Only
`ansible-creator` was missing. `ansible-lint`, which the extension uses for
linting, was also absent. The extension's built-in file patterns already cover
`infra/ansible` playbooks, roles, `group_vars` and `requirements.yml`.

**Change:** `.devcontainer/setup-ansible.sh` installs
`ansible-lint==26.11.0` and `ansible-creator==26.11.0` in the same `pip`
call as `ansible-core==2.22.0`, then prints their versions.

Verification: the edited script ran in a clean `python:3.13-slim` container
with the repository mounted read-only, and exited 0. `ansible-core` stayed
2.22.0; `ansible-lint` 26.11.0 (ansible-compat 26.11.0) and
`ansible-creator` 26.11.0 ran. Calling `ansible-lint` outside its venv first
printed a "PATH altered" warning, so the script now runs it with the venv on
`PATH`.

Not run: a devcontainer rebuild, and ansible-lint over `infra/ansible`, so how
many findings the extension will now show is unknown.

## Project chat with a sandboxed agent, P0–P7 (2026-11-23)

Plan: [project chat](../planning/project-chat.md). Decision request:
[DEC-009](../planning/human-decisions.md#dec-009---agent-chat-from-the-web-api).
Developed on branch `worktree-project-chat` from `8464612`, then rebased onto
`7e4282b`, which includes the admin and user roles (`021b62f`). `GET /v1/agents` and the four chat
routes are registered with `auth.RoleUser`.

**What changed.** Contract first (`contracts/openapi.yaml`: `GET /v1/agents`,
`/v1/projects/{projectId}/chats`, `.../chats/{chatId}`, `.../messages`). New
`internal/agents` (agents file, `ember.chat-turn/v1` envelope fitted to the
router's 32 KiB body, Unix-socket router client with fixed user messages),
`internal/projects/chats.go` (transcripts as `chats/<id>.json` via
`projectfiles`), `internal/httpapi/chats.go` (asynchronous runs: `202`, one run
slot, one run per chat, interruption on shutdown and after restart),
`main.go` (`EMBER_AGENTS_CONFIG_FILE`; refuses to start without sign-in unless
`allowWithoutSignIn`), a chat panel in the Notes app, and an offline fixture
agent (`services/sandbox-router/chat-fixture`). CI also runs the fixture's
Python tests.

**Platform.** Fedora 45, Linux 7.2.11 x86_64, rootless Podman 6.2.0 with
cgroup v2. No Go on the host: Go ran in
`golang:1.27.4@sha256:205c7636…126cf` with Podman. Node 26.13.1 on the host.
Firefox 160.0.1 (Flatpak), headless, driven over Marionette.

| Check | Result |
| --- | --- |
| `gofmt -l .`, `go vet ./...`, `go test -race -count=1 ./...` in `apps/api` (container) | Pass. `gofmt` still lists `internal/plugins/weather/weather.go`, which was unformatted before this change and is untouched. |
| `go vet ./... && go test -race ./...` in `services/sandbox-router` (container) | Pass. Router code unchanged. |
| After the rebase: `go test -race -count=20 -run TestChat ./internal/httpapi/`, then the full suite | Pass. The first run after the rebase failed once in `TestChatRunFailuresAreSanitized`: a run recorded its answer before giving back the run slot and chat claim, so a send the instant the answer appeared could get `429` or `409`. Runs now give both back while holding the vault lock for that write, and readers check for a run before reading. `TestChatSendRightAfterAnswer` covers it. |
| Requests without a role (`TestChatRoutesNeedUserRole`) | `403 insufficient_role` on all five routes; no run started, nothing saved. |
| `python3 -B -m unittest discover -s chat-fixture` and `-s pilot` in `services/sandbox-router` | Pass (4 and existing tests). |
| `npm ci && npm run check && npm run build` in `apps/frontends/web` | 0 errors, 0 warnings; static build written. |
| `npm --prefix website ci && npm --prefix website run build` | Pass; `cmp contracts/openapi.yaml website/build/openapi.yaml` identical; DEC-009 anchor present. |
| Local end to end: fixture image built with Podman, router (`trusted`) on a `0700` socket directory, API binary (`CGO_ENABLED=0`) on `127.0.0.1:8080` with an agents file and `allowWithoutSignIn`, curl | Chat created; send returned `202` with a pending message; router logged `profile=trusted success=true`; answer recorded about 0.26 s later; second turn saw 2 earlier messages; a send during a run got `409`; transcript on disk as indented JSON; no containers left (`podman ps -a --filter label=dev.ember.sandbox-router=v1`). |
| Same, router stopped | Message recorded as failed with "The sandbox router is not reachable…"; API log line has outcome `router_unavailable` and no message text. |
| API with an agents file and no `allowWithoutSignIn`, sign-in off | Exits 1 with the sign-in error before listening. A router policy passed as the agents file is rejected (`unknown field`). |
| `SIGTERM` to the API | `api stopped` logged. |
| Notes app built, `vite preview` proxying `/api`, headless Firefox via Marionette | Chats listed; **New chat** → message → **Send** showed "Agent is working…" with Send disabled, then the fixture's answer as text; URL became `?project=desk&chat=<id>`. Screenshots checked by eye. |

**Not run.** Sign-in on (OIDC) with chat; the `isolated` (Kata) profile for a
chat agent; the Compose deployment and installer (they do not mount the router
socket yet, plan P8); ARM64; an API restart while a real run is in progress
(covered only by handler tests); any real model, since agents have no network
(P9). The headless-browser run is a smoke check, not an accessibility or
cross-browser review.

## Claude Code in the devcontainer (2026-11-23)

**Request (user):** include Claude Code in the devcontainer by default.

**Change:**

- `.devcontainer/Containerfile` installs `@anthropic-ai/claude-code`, pinned
  by the `CLAUDE_CODE_VERSION` build arg (2.1.323, npm's `stable` tag that
  day; `latest` was 2.1.330). It installs as `node` into the image's
  node-owned npm prefix, so Claude Code's self-update needs no root. An empty
  build arg skips the install.
- The package's `postinstall` hard-links the native binary from the
  platform's optional dependency (`claude-code-linux-x64` or `-linux-arm64`)
  over `bin/claude.exe`. npm 11.19.1 still runs it, but warns that it is "not
  yet covered by allowScripts", so the install passes
  `--allow-scripts=@anthropic-ai/claude-code`.
- `devcontainer.json`:
  - mounts the named volume `ember-claude-config-${devcontainerId}` at
    `/home/node/.claude`;
  - sets `CLAUDE_CONFIG_DIR` to it, so `.claude.json` lands there too and
    sign-in survives rebuilds;
  - adds the `anthropic.claude-code` extension.
- `setup.sh` chowns that volume with the other dependency volumes.

Anthropic's devcontainer feature
`ghcr.io/anthropics/devcontainer-features/claude-code` 1.0.5 was not used. Its
`install.sh` installs the latest release, unpinned. It also installs
iptables, ipset and other firewall packages, plus an inactive firewall script.
This is a development aid only: Ember itself does not depend on any agent.

**Platform:** Fedora 45, Linux 7.2.11 x86_64, rootless Podman 6.2.0, base
image `mcr.microsoft.com/devcontainers/javascript-node:24-bookworm` (npm
11.19.1).

| Check | Result |
| --- | --- |
| `bash -n .devcontainer/setup.sh`; `devcontainer.json` parsed as JSON | Pass. |
| `podman build -f .devcontainer/Containerfile .devcontainer`, then `claude --version` as `node` | `2.1.323 (Claude Code)`; `claude` resolves to `/usr/local/share/npm-global/bin/claude`, an x86-64 ELF identical (SHA-256) to the platform package's binary; package files owned by `node`. No install-scripts warning with `--allow-scripts`. |
| Container run with `--userns=keep-id:uid=1000,gid=1000`, `CLAUDE_CONFIG_DIR`, and a new named volume at `/home/node/.claude` | Volume owned by `node`; the `setup.sh` chown succeeded; a file written there was present in a second container using the same volume. |
| Build with `--build-arg CLAUDE_CODE_VERSION=` | Succeeded; no `claude` on `PATH`; `~/.claude` still exists. |

**Not run:** `devcontainer up` with the Dev Container CLI, and therefore the
full `postCreateCommand` and the VS Code extension install. The user's own
devcontainer held ports 3000 and 5173. Also not run: an arm64 build, an
interactive `claude` sign-in, and a self-update inside the container.

## One web frontend (2026-11-23)

**Change (operator request):** Ember supports one frontend for now, the
SvelteKit app. `apps/frontends/home` is removed and its logic moved into
`apps/frontends/web`, which now owns the site root: the home page at `/`,
Project notes at `/apps/notes/` ([ADR 013](../decisions/013-single-web-frontend.md)).
One Compose service and image, `web`/`ember-web`, replaces `home` and `notes`.
The gateway now sets `absolute_redirect off`: before, the `/apps/notes`
redirect was absolute and named the container's port 8080. `install.sh` and
the `ember` role remove orphaned containers.

The change was rebased onto project chat (`3539847`). Its chat UI moved with
Notes to `routes/apps/notes/+page.svelte`, and the chat rules from `notes.css`
into that page's scoped styles. After the rebase, `npm run check` (0 errors,
0 warnings), `npm run build`, the bundle smoke test and the docs build passed
again; the other checks below ran before the rebase.

Verification (Fedora host, Node 26.13.1, Docker Compose 5.8.0):

- `npm ci && npm run check && npm run build` in `apps/frontends/web`:
  svelte-check 0 errors and 0 warnings; the build prerenders `/` and
  `/apps/notes/`.
- `bash deploy/build-bundle.sh DIR` then `bash deploy/test-bundle.sh` on the
  archive of the committed change: exit 0. Through the gateway: `/` and
  `/apps/notes/` titles, `308` from `/apps/notes?project=p` to
  `/apps/notes/?project=p` on the published port, the manifest, both service
  workers, and health before and after recreating the API.
- A plain `nginx:1.30.7-alpine` with the old `return 308` location sent
  `Location: http://127.0.0.1:8080/apps/notes/?project=p` from a different
  published port; with `absolute_redirect off`, `/apps/notes/?project=p`.
- Upgrade: the `7e4282b` stack (`api`, `home`, `notes`, `frontend`) ran with a
  project created through the API; `bash deploy/install.sh` from the new
  source, with the same project name, removed `home` and `notes`, started
  `web`, and the project was still listed.
- `PluginCard.svelte` and `SchemaFields.svelte` were server-rendered (Vite
  `ssrLoadModule`, `svelte/server`) with the Immich and Weather plugins and
  catalogs returned by the running API. Controls, `required`, the write-only
  placeholder, number limits and the user-role notice matched the removed
  `integrations.js`. `readFields` ran on stand-in controls; its Immich payloads,
  with and without the key, returned 200 from `PUT .../configuration`, the
  key was kept, and an operation invocation with `{}` reached the adapter.
- `npm --prefix website run build` succeeded (links checked).
- ansible-core 2.22.0 (throwaway venv, collections from `requirements.yml`):
  `site.yml --syntax-check` passed; `ansible-doc` lists `remove_orphans` for
  community.docker 5.4.0.

Not run: no browser was available, so clicking through the pages, the
draft guard on client-side navigation, and the retirement of an installed
Notes service worker were not exercised. Neither were the GitHub `images`
job, a registry-mode deploy (the new `ember-web` package must be made public
first, DEC-008), or `site.yml` against a host.

## Sticky `/tmp` in the devcontainer (2026-11-23)

**Problem (user report):** Claude Code in the devcontainer warned
"Cross-session messaging is off: … '/tmp' is world-writable without the sticky
bit (owner 0:0, mode 0777)". Sessions still worked, but they could not find
or message each other.

**Cause:**

- `/tmp` is `1777` in the base image
  (`mcr.microsoft.com/devcontainers/javascript-node:24-bookworm`) and in the
  first feature layers that the Dev Container CLI adds.
- It is `0777` in the built `vsc-ember-…` image and in the running container.
  One of the `python`, `go` or `github-cli` feature install steps clears the
  sticky bit. Those intermediate layers are not kept locally, and the feature
  scripts at the locked digests contain no `chmod` of `/tmp`, so the step was
  not pinned down.
- A `RUN --mount=type=bind` at `/tmp/build-features-src/…`, from the build
  context or from another stage, did not reproduce it under Podman 6.2.0.

**Change:** `.devcontainer/setup.sh` (`postCreateCommand`) runs
`sudo chmod 1777 /tmp` first. A `chmod` in the `Containerfile` would run
before the features.

Verification (Fedora 45 x86_64, rootless Podman 6.2.0, Claude Code 2.1.323):

| Check | Result |
| --- | --- |
| Interactive `claude --debug-file …` in a throwaway container, `/tmp` at `0777` | Screen and debug log show the sticky-bit refusal; "cross-session messaging is OFF for this session". |
| Same with `/tmp` at `1777` | No warning. |
| `bash -n .devcontainer/setup.sh`; the new line run as `node` (sudo) on a `0777` `/tmp` | Syntax OK; `/tmp` became `1777`. |

Not run: a devcontainer rebuild with the Dev Container CLI, and messaging
between two live sessions.
