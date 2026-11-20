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
3. Copy and extract the archive on a host with Docker Compose v2 or Podman Compose.
4. Run `./install.sh` from the extracted bundle directory.

The workflow retains artifacts for 30 days. They are CI artifacts, not permanent
releases. They contain source/build contexts, not prebuilt images; installation
requires a working container engine and network access to fetch base images.
Tagged release publishing and versioned upgrade/rollback behavior have not been
implemented yet.

## Server requirements and configuration

- Docker Engine with Compose v2 or Podman with a working Compose provider.
- A supported container host architecture for the upstream Go and Nginx images
  (including AMD64 and ARM64).
- Network access to fetch the base images on first build.

The frontend is published on `127.0.0.1:3000` by default, and proxies API
requests internally. Set `EMBER_HTTP_PORT` before running `./install.sh` to
change the host port. The API has no user authentication; keep the frontend
loopback-bound unless you have configured a trusted reverse proxy and suitable
authentication and network controls.

Useful Compose commands (run from the extracted bundle directory):

```sh
docker compose -f deploy/compose.yaml ps
docker compose -f deploy/compose.yaml logs -f
curl http://127.0.0.1:3000/api/healthz
docker compose -f deploy/compose.yaml down
```

## Local development

Install Go 1.24 or newer, then run:

```sh
cd apps/api
go test ./...
go run .
```

The API currently exposes only `GET /healthz`. No project, task, database, or
agent functionality is claimed by this scaffold.

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
