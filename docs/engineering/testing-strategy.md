# Engineering standards and verification

## What belongs in a change

Use Go's `net/http`, `context`, `log/slog` and explicit dependencies where appropriate. Keep conservative defaults at process startup; read and validate configuration once and never log resolved secrets. Separate constructing a handler from binding a listener. Introduce a package or interface where there is an actual caller and an independently testable responsibility, not just a box in a diagram. Third-party dependencies and reusable internal libraries need approval per [ADR 004](../decisions/004-go-and-library-policy.md).

For HTTP changes, start with `contracts/openapi.yaml`. Preserve `/healthz`; Nginx supplies the external `/api` prefix. Avoid changing the public prefix to match a proposed `/v1` sketch. Specify status, response fields and error semantics before introducing a resource. Bound input before decoding, distinguish invalid input from authorization failures and internal errors, and avoid leaking host paths or upstream payloads. Use stable low-cardinality route labels and service-generated correlation IDs; treat caller-supplied trace metadata as untrusted. Defer distributed tracing infrastructure until an operation crosses independently operated services.

For future durable commands, define the commit point, idempotency scope and key lifetime. Commit associated task/approval records in one transaction where possible. Never hold a transaction over a remote call. Document how to reconcile an unknown outcome. Run migrations once, version them, test old-data fixtures and restore; destructive or irreversible migrations require explicit authorization.

## Verification ladder

| Surface | Fast gate | Evidence still needed beyond fast gate |
| --- | --- | --- |
| API contract | `httptest` exercises production handler, JSON fields/types, content type, unsupported method, unknown route | Actual listener and Nginx path rewrite; HEAD semantics, proxy outage and recovery. |
| Lifecycle | Unit-test config and deterministic context cancellation; inspect `gofmt`, `go vet`, `go test -race ./...` | Bounded real listener shutdown during in-flight request, forced timeout, nonzero bind failure. Synchronize with channels; don't assume arbitrary sleeps prove a state. |
| Packaging | Extract archive in a new directory and render Compose model; assert each resolved Dockerfile exists and public binds are loopback | Rootless Compose smoke with unique project/port, proxied JSON and cleanup; qualify real ARM hardware separately. |
| Provisioning | Syntax with supported pinned collections, validate **keys only** of resolved inventory and avoid live secrets | Provision disposable target, run twice, inspect changes and restore prior access. `--check` can contact targets and is not a local syntax check. |
| Docs | Node 24+, `npm --prefix website ci`, `npm --prefix website run build` with Pages base path | Check served reference, embedded diagrams and contract URLs. Keep container/host dependencies separate. |
| Future integration | Local deterministic upstream fixture tests scoped operations, timeout, throttling, revoked credentials, off-origin redirects and sanitized errors | Two consumers, separate upstream identities, independent revocation, shared app outage; real upstream version qualification. |
| Future job/data | Duplicate delivery, lease expiry, stale attempt, simultaneous approval consumption, partial artifact commit, cancel and reconcile | Restore isolated instance from encrypted snapshot; no stale approval reuse or device replay; inspect runtime enforcement on actual hardware. |

Run commands from `apps/api` for Go and from repository root for Compose/docs. Separate compilation, handler behavior, image build, proxy reachability and hardware qualification in evidence. In particular, do not turn one green handler test into a claim that the installer or sandbox works.

## Review and release workflow

1. Check worktree, relevant ADR and pending decision; state scope, rollback and acceptance test.
2. Run focused checks for the changed boundary; run the full affected package/build once the focused test passes. Use offline/mocked services unless a real qualification is explicitly needed.
3. Review unexpected exposure, unbounded retries and shutdown, untrusted inputs in logs, and hidden dependencies on host paths.
4. Record exact commands, platform, failures and unexecuted checks in the progress log. Link contract, operational and policy changes to the same revision.
5. Publish a release only after approval, evidence for supported platforms, pinned inputs and a rollback plan. GitHub Pages publishing and a 30-day CI bundle do not deploy the application to a server.

### Updating the runtime images

The app Dockerfiles pin **multiarchitecture index** digests, not host-specific
image manifests. For an update, inspect the proposed tag with `skopeo inspect
--raw`, verify that the index includes Linux AMD64 and ARM64, and change the
descriptive tag and digest together. Align the workflow Go version if the
builder changes. Check upstream advisories, rebuild and run `deploy/test-bundle.sh`
on an assembled bundle. Verify proxy name refresh after replacement, health
payload/version, API nonroot operation and the target architecture. Recheck
pins when security fixes ship; an immutable vulnerable image stays vulnerable.
