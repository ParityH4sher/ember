# Diagnosing the health scaffold

Start from the repository or extracted bundle root. These checks distinguish path resolution, engine availability and a responding proxy. They do not require Ansible access.

| Symptom | Check | Interpretation / next step |
| --- | --- | --- |
| Bundle is missing from Actions | Inspect the **test** job before the bundle job. Locally run `go test ./...` from `apps/api`. | A production binary may build even when a test import prevents test compilation. A 30-day-old artifact may also have expired. |
| Compose accepts YAML but cannot find a build directory | `docker compose -f deploy/compose.yaml config` and inspect each `build.context` for a real Dockerfile | In the current layout, paths are relative to the manifest in `deploy/`. Do not override the project directory to the parent. Preserve `apps/` and `deploy/` as siblings. |
| Compose provider exists but cannot connect to the engine | `docker compose version` or `podman compose version`, then `compose ps` with the chosen runtime and manifest | A version response checks the CLI, not its socket. Use the operator's intended local rootless/rootful context. Do not grant the app access to the socket. |
| Static page loads, API shows unavailable | `curl -i http://127.0.0.1:3000/api/healthz` (substitute the configured port); check `docker compose -f deploy/compose.yaml ps` and scoped service logs | `/api/healthz` goes through Nginx. `404` can mean a wrong path; `502` points to an unavailable upstream. A JSON `200` confirms only process/proxy reachability. |
| A second instance replaces the first or fails to bind | Inspect effective Compose name, network and host port **before** `up` | A unique `-p` and a distinct `EMBER_HTTP_PORT` are both necessary. Do not tear down an existing project to free its port. |
| Provisioner cannot resolve a module | Install `infra/ansible/requirements.yml` using compatible `ansible-core`; run `ansible-playbook ... --syntax-check` | On the examined devcontainer, bundled `community.docker` 3.4.7 lacks `docker_compose_v2`. Newer unpinned collections parsed but warned that core 2.14.18 is unsupported. Syntax success is not runtime compatibility. |
| Provisioner uses an unexpected directory or port | Check the Ember role defaults and selected group or host overrides for `ember_install_dir` and `ember_http_port` | Defaults apply to both inventories. Keep live inventory private; printing the full resolved inventory can disclose passwords. |

### Recovery scope

Current images refresh the API name using the runtime's local DNS and a
five-second validity window. A brief gateway error can occur during
replacement. Retry the probe after the new API starts.

**Older images:** on rootless Podman an API restart changed
its container IP. The running Nginx returned no response before the client's
deadline, although a fresh request from the frontend directly to
`http://api:8080/healthz` succeeded. For that specific symptom, confirm the
project name and run `docker compose -p "$PROJECT" -f deploy/compose.yaml exec
frontend nginx -s reload` from the bundle root. Query the proxied endpoint
again. This recovered the tested instance without touching the API or its
sibling deployment.

The two baseline services have no app data volumes. Record the source revision, **actual image IDs**, Compose provider/version, project name and port before changing an installation. Preserve a previously working bundle or images outside the short-lived Actions artifact. Rebuilding from the same source with floating image tags is not a byte-identical rollback.

For a change to the health scaffold, rehearse starting the prior images and configuration under a **different** project name and unused loopback port, then query the proxy. Do not replace a running instance until the operator approves a cutover. Once a database or shared application is involved, use the [data recovery gates](../architecture/deployment-model.md); reverting a binary cannot undo a schema or a device action.
