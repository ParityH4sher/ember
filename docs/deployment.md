---
sidebar_label: Build and deploy
sidebar_position: 6
---

# Build and deploy

The application is deployed as containers with Compose. GitHub Actions tests the
Go API and assembles a portable source bundle containing both app build contexts,
the Compose manifest, and an installer. The target server builds images for its
own architecture, so the same artifact works on supported AMD64 and ARM64 hosts.

## Download a GitHub Actions artifact

1. Open the repository's **Actions** tab and select **Build and test Ember**.
2. Select a successful run and download `ember-compose-bundle`.
3. Place the `.tar.gz` and `.sha256` files from that run together on the target
   host. From that directory run `sha256sum --check ember-<revision>.tar.gz.sha256`
   with the actual revision. This checks for corruption, not publisher identity.
4. Extract the verified tarball and run `./install.sh` from the extracted bundle root.

The workflow retains artifacts for 30 days. They are CI artifacts, not permanent
releases. They contain source/build contexts, not prebuilt images; installation
requires a working container engine and network access to fetch base images.
Tagged release publishing and versioned upgrade/rollback behavior have not been
implemented yet.

The bundle workflow verifies the checksum in a fresh directory, extracts the
archive, checks both effective build contexts and loopback exposure, runs the
installer and requests JSON through Nginx before uploading the artifact.

To reproduce a bundle locally, run `bash deploy/build-bundle.sh /path/to/output
REVISION` in a source checkout (Bash, Git, GNU tar, gzip and GNU coreutils).
Omit `REVISION` to select `HEAD`. The script packages committed files from
that revision; uncommitted changes and unrelated files are not included.
Archive order, timestamps, ownership, permissions and gzip metadata are
normalized. Existing, different files under the same bundle name are not
replaced. Image-build reproducibility is separate from source archive equality.

## Server requirements and configuration

- Docker Engine with Compose v2 or Podman with a working Compose provider.
- A supported container host architecture for the upstream Go and Nginx images
  (including AMD64 and ARM64).
- Network access to fetch the base images on first build.

The frontend is published on `127.0.0.1:3000` by default, and proxies API
requests internally. Set `EMBER_HTTP_PORT` before running `./install.sh` to
change the host port. The API has no user authentication; keep the frontend
loopback-bound unless you use a private access layer such as Tailscale Serve.
The Ansible setup installs Tailscale but requires manual device enrollment;
see the [operations guide](operations/README.md#ansible-host-provisioning).

The `frontend` gateway routes `/api/` to `api` and every other path to `web`,
the single browser app: the home page at `/` and Project notes at
`/apps/notes/`.

The installer accepts decimal ports `1` through `65535` without leading zeros.
`EMBER_COMPOSE_RUNTIME=podman` or `docker` selects a specific provider; the
default `auto` prefers the Docker Compose CLI. `./install.sh --check` parses
the effective Compose configuration without starting services. Normal installation
checks the Nginx-to-API path before reporting success. Use `compose ps` to
inspect frontend health; a healthy result measures that path, not any planned
worker. An unhealthy status alone does not trigger Compose's restart policy.

Useful Compose commands (run from the extracted bundle directory):

```sh
docker compose -f deploy/compose.yaml ps
docker compose -f deploy/compose.yaml logs -f
curl http://127.0.0.1:3000/api/healthz
docker compose -f deploy/compose.yaml down
```

## Local development

The module retains Go 1.24 language compatibility. CI and the release image
use Go 1.27.4; use a supported patched toolchain for local builds. Run:

```sh
cd apps/api
go test ./...
go run .
```

The API stores active project records and Markdown notes under `EMBER_PROJECT_ROOT`
(`./projects` for a local binary; `/var/lib/ember/projects` in the Compose
`projects` volume). Open `/apps/notes/` on the frontend to edit them. Removing
the volume removes its data. Removing a project record retains its notes on disk.
Plugin configuration is lost on API restart. Sign-in is optional
[OpenID Connect](authentication.md), off by default. There is no database, task execution, or agent
behavior. Weather sends coordinates to Open-Meteo; its free API is
non-commercial and requires attribution. Do not expose this prototype to an
untrusted network or configure real upstream credentials. The Immich plugin
reads only the server version, the key's user and album metadata; give it a
dedicated non-admin Immich account (DEC-005).

`EMBER_LISTEN_ADDR` defaults to `127.0.0.1:8080` when running directly. Compose
sets it to `0.0.0.0:8080` inside the unpublished API container. The server
sets read-header/read/write/idle timeouts of 5/10/15/60 seconds. On SIGTERM
or interrupt it stops accepting connections and drains active requests for
up to five seconds, then closes remaining connections and reports a failure
if the deadline expired. A bind failure is also a nonzero process exit.
Startup and shutdown records are JSON on stderr. Each API response handled by
the application includes an `X-Request-ID` generated by the server; correlate
it with the API's JSON access record (`route`, `status`, `duration_us`).
Unmatched paths use the label `unmatched`. Do not use the ID as an identity
or authorization token. Nginx logs a separate JSON access event with the same
upstream ID (when available), status and elapsed seconds. Neither access
format records the raw URI or query. Error and container-engine logs remain
separate and may include diagnostic request details.

`version` in the health response identifies the source: `development` for a
normal checkout, the full Git commit for a CI source bundle. CI stamps only
the copied `apps/api/version.txt`, which Go embeds at build time; running the
API does not need Git, repository history or a writable root filesystem.

## Multi-service Compose deployment

The API and static frontend can run together from the repository root:

```sh
docker compose -f deploy/compose.yaml up --build -d
```

The UI is available at `http://127.0.0.1:3000` and checks the API through its
same-origin `/api/healthz` proxy. This avoids exposing the API port directly.
The Compose deployment builds both services locally; it is currently a
development/POC deployment, not a signed release bundle.

For Debian/Ubuntu host setup and automated Compose deployment, see the
[Ansible operations guide](operations/README.md#ansible-host-provisioning).
For initial Ubuntu Server setup on a Raspberry Pi, see the
[Raspberry Pi first-boot guide](raspberry-pi-first-boot.md).

## Artifact and security scope

The CI bundle contains the application build contexts and Compose deployment
files; it does not include the documentation site, external services, or a full
Ember product. CI artifacts expire after 30 days. For durable distribution,
configure a tagged GitHub Release workflow in a later iteration.

Ember services are run through Compose, not systemd. Compose is not by itself a
strong security boundary. The API is not authenticated; do not expose it to
untrusted networks or use it to execute agent-generated code.
