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
