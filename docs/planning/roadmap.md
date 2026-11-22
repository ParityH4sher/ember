# Engineering roadmap

The first implementation milestone is **a health scaffold that passes its own tests and installs from the CI layout**. A gateway or task queue would not fix the current test compilation and build-context failures. Numbers indicate dependency order, not delivery dates. Size/risk estimates are relative to the current two-service scaffold.

| Order / outcome | Value | Size / risk | Dependencies | Acceptance evidence and rollback |
| --- | --- | --- | --- | --- |
| M1a. Restore the API test gate | Unblocks all CI bundles | Small / low | None | `go test ./...` passes and checks JSON keys, `status`, version type, method handling and an unknown route. Preserve the public contract; test-only changes can be reverted. |
| M1b. Resolve build contexts and make the checksum portable | Makes packaged sources verifiable and usable | Small / low | M1a for CI publication | From a fresh download directory verify the adjacent `.tar.gz` with `sha256sum -c`, then extract. Check source and extracted archive with the documented invocation. Both contexts resolve, loopback bind remains, real proxy returns the health payload. Do not change the operator's port or data. Roll back deploy files as a unit. |
| M1c. Complete provisioning defaults | Make the documented inventory usable | Small / medium | Compatible controller and collections | Both inventories have the required deployment variables and pass syntax checks. Validate package names and rerun provisioning on an approved disposable target before claiming idempotency. |
| M1d. Qualify patched Go release builder | Addresses observed standard-library scan findings without a framework or API change | Small / medium | M1a; image support on AMD64/ARM64 | Build with supported patch version, run API tests/race/vet and image/proxy smoke; keep `go.mod` language minimum separate. Compare binary scan before/after and document rollback exposure. |
| M1e. Recover proxy after API address changes | Avoid sustained outage after restarting one container | Small / low | Existing Compose DNS, M1b | Resolve `api` using the container runtime's configured DNS, with bounded refresh and connect timeout. Preserve `/api/` rewriting; verify recovery after a confirmed address change without reloading Nginx and continued health of the other instance. Fall back operationally to a scoped proxy reload. |
| M2. Bounded HTTP server lifecycle | Limits hung connections and supports clean update/stop | Small / low | M1a | Explicit read-header/idle limits and bounded shutdown; deterministic startup/bind errors and signal handling. Existing health route and loopback default preserved. Exercise in-flight drain and timeout; revert with one API rebuild. |
| M3. Minimum operational telemetry | Makes a failed request or shutdown diagnosable without logging private content | Small / low | M2 | Structured startup/stop/failure records and bounded route/status fields, no raw query or headers. Use standard library; add tests of redaction. Keep health cheap. |
| M4. Validate public contracts and publishing | Avoid drift between docs and code | Small–medium / low | ADR 006; approval for any new validator | Test actual JSON against the canonical schema; CI verifies contract changes and refreshes hosted reference. Keep generator and renderer versions compatible; avoid a second hand-maintained API spec. |
| M5. First authenticated read-only service connection | Proves instance isolation and integration reuse | Medium / high | DEC-001/002/005/006 | Two private connections with distinct upstream credentials; revoked or wrong-owner reads denied. Disconnect A; B and the service remain intact. Disable adapter to roll back. |
| M6. Durable local task plus fixture runner | Tests store, lease, cancellation, export and restart without paying a provider | Medium / high | DEC-003 and data/recovery tests | Duplicate claims and stale completions rejected; cancelled task releases resources; restore on empty host verified. No rollback across incompatible migrations without a restore plan. |
| M7. First real execution or model pilot | Tests useful design-tool output | Large / high | DEC-004/007 and required monitoring | Verified isolation, budget enforcement and useful declared artifact on target hardware. Remove execution grants to disable; preserve user-owned artifacts. |

If an approval blocks a milestone, work on a preceding independent verification or reliability issue. Do not add a fake persistence layer or public unauthenticated job endpoint just to exercise the UI.

## POC versus later work

**Current project priority: cross-run Markdown handoff.** First prove that a
second disposable workspace can resume from an index, a short handoff and a
linked topic after the first workspace is gone. Inspect the same notes as an
Obsidian vault. The [operator CLI and note conventions](../agent-project-contract.md#linked-project-notes-and-resuming-a-task)
are the starting point for runner integration. Thumbnail qualification is deferred.

Near term: one private host, one API build, one real read-only integration after identity, two independent consumers. Verify service outage degrades the operation and never transfers ownership. The POC brief also proposes an allowlisted controller action; it requires DEC-006 and is not implied by a successful read.

Later: independently scale runners, add qualified gateways and, if required, multi-node scheduling. Trigger this by observed queue age, saturation, inability to meet recovery objectives or an actual isolation requirement. A fleet manager, plugin marketplace, cross-instance federation, Kubernetes, paid inference and physical machine operation are not prerequisites to M1–M4.
