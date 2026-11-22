# Ember Compose source bundle

This bundle contains a Go API, the Home and Notes frontends, their Nginx gateway and
Compose configuration. The source revision is in `apps/api/version.txt`.
The target host builds the images; it needs Docker with Compose v2 or Podman
with a compatible Compose provider and access to the pinned base images.

From this extracted directory:

```sh
./install.sh --check
./install.sh
curl --fail http://127.0.0.1:3000/api/healthz
```

The JSON response's `version` should match `apps/api/version.txt`. The default
host port is loopback-only. Set `EMBER_HTTP_PORT` to change it, and set
`EMBER_COMPOSE_RUNTIME=podman` to explicitly select Podman. For a second
instance, export **both** a distinct `COMPOSE_PROJECT_NAME` and a distinct
port; use the same settings when inspecting or stopping that instance.

For Docker, use `docker compose -f deploy/compose.yaml ps`, `logs api frontend`
with the same Compose prefix, or `docker compose -f deploy/compose.yaml down`.
For Podman, substitute `podman compose`. No database, project data or agent
runtime ships in this archive. See the [deployment guide](https://parityh4sher.github.io/ember/deployment/)
for operational limits and the checksum procedure.
