# Current Engineering Status

## Session

- Start: 2026-11-21 00:34:06 UTC. Initial baseline: `76baa4c`, clean `main`.
- Phase 2 began at the original 02:34:06 UTC gate. At 02:44 UTC approximately 130 minutes had elapsed since session start.
- Implementation gate: 02:34:06 UTC; feature freeze: 05:19:06 UTC; deadline: 05:34:06 UTC, all on 2026-11-21. These are wall-clock gates, not estimates of work already performed.

## Architecture

Go standard-library first and versioned OpenAPI are established decisions (ADRs 004–006). The only application route is process health. The POC brief and worker directories describe proposals. Identity, execution isolation, storage and shared-service authority are open.

## Completed Work

Inspected tracked files, history, runtime, CI bundle layout, inventory/role configuration and documentation. Ran the baseline API and end-to-end proxy on AMD64. Published [architecture and component overview](../architecture/overview.md), [human decisions](../planning/human-decisions.md), [roadmap](../planning/roadmap.md), [risks](../planning/risks.md), [verification standards](../engineering/testing-strategy.md), ADRs 007–008 and [baseline experiment log](implementation-log.md).

## Current Task

The installer now checks the selected runtime, port and proxied API before announcing success. Local assembly now uses committed source only. Latest AMD64 native bundle smoke passed. The first disposable Ubuntu VM deployed successfully twice but reported one unnecessary change on the second run. Testing the revised fingerprint-based convergence/recovery on another VM; a separate post-fix two-instance recovery study with distinct markers ends by 05:14 UTC. Neither uses repository inventory targets.

## Verification

- Standalone API: Go 1.24.13 in the locally cached devcontainer built AMD64 and Linux ARM64 binaries. On AMD64, `GET /healthz` returned the documented JSON, `POST /healthz` returned 405 and `GET /` returned 404. The ARM64 binary also served the expected JSON under QEMU user-mode emulation. No Pi hardware test.
- `go test -race ./...`, `go vet ./...`, formatting and cross-build passed after the API repair. Lifecycle tests verify an occupied address, drained active request and forced close. Access log tests verify server-generated IDs and redaction.
- Fixed both Compose build contexts, checksum filename and Ansible project root. Extracted bundle installation/health/recreation passed with native Docker and rootless Podman. Native Docker first exposed a startup connection reset; the smoke now retries failed idempotent probes with a deadline. The patched image reports its full source revision when built from the CI bundle.
- Baseline Docusaurus install and production build with `/ember/` base path passed in a temporary copy. Existing workspace dependencies left intact.
- Binary `govulncheck` found 21 standard-library findings with Go 1.24.13 and none with 1.27.4 on the same source. Website lockfile audit reported 26 affected packages. See [risks](../planning/risks.md); do not equate dependency counts with runtime exploitability.
- Ubuntu 24.04.5 AMD64 guest, Ansible 2.22.0: baseline role qualification passed with successful proxied JSON and working SSH/UFW, but the second run unnecessarily rebuilt the services. Historical devcontainer Ansible 2.14 is unsupported by the qualified collection versions.

## Known Problems

The role now supplies deployment defaults for either inventory. The operator identified its inventory credential as a testing fixture; exposure assessment is out of scope. Physical Pi behavior, signed release rollback and production identity are unqualified. Full OpenAPI schema tooling remains unselected. Removed the incompatible `find.checksum_algorithm`. Added Buildx and an Ansible controller preflight; verify both on the next disposable Ubuntu run.

## Next Actions

1. Finish installer, release and contract verification; record any VM provisioning failure before claiming host support.
2. Compare pre/post proxy behavior from the two-instance study. Stop starting features at 05:19:06 UTC. Do not duplicate work already logged.
3. Use the temporary Go toolchain at `/tmp/opencode/ember-toolchain.IR2rKd/go/bin` with `GOCACHE=/tmp/opencode/ember-go-build`; the cached devcontainer has the minimum toolchain. Keep identity, persistence and execution decisions pending.

## Continuation Instructions

From `ember/` run `git status --short --branch`, read this page and `docs/architecture/README.md`, and compare `date -u` with the gates above. Distinguish baseline evidence from subsequent verification. A later session outside the deadline requires a new budget.
