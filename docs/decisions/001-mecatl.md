---
sidebar_label: "001: Mecatl"
---

# ADR 001: Evaluate Mecatl as an optional Ember engineering runner

[Companion notes](../README.md) · [Sandboxing](../sandboxing/README.md) · [OpenCode decision](002-opencode-sdk.md)

**Date:** 2026-11-18  
**Decision:** Defer for the initial Ember prototype; retain as a candidate for a later engineering-runner pilot.  
**Evidence:** Upstream documentation and release metadata; no deployment or benchmark performed.

## Could it help?

**Potentially, for engineering tasks.** Ember is a self-hosted personal assistant
and engineering workspace, not a workstation bootstrapper or a shared coding
service by definition. Mecatl’s agent loop, permissions, recorded events, OIDC
caller identity, durable sessions and selectable execution environments may
help isolate repository work. They do not replace Ember’s assistant/orchestrator,
memory, personal integrations or action authorization.

| Requirement | Documented capability | Work still owned by the deployment |
| --- | --- | --- |
| Local microVM execution | `microvm-local` runs filesystem and shell tools in a repository-scoped VM | Verify host prerequisites, release artifacts, workspace scope and egress |
| Kubernetes execution | Separate execution provider/controller, pinned templates, executor pods and retained PVCs | Supply CNI enforcement, suitable RuntimeClass, PKI, storage and lifecycle operations |
| OIDC / OAuth2 integration | OIDC issuer/audience validation and verified `(issuer, subject)` ownership | Register the application/client, configure login and authorization, define workload credentials |
| Durable sessions | Redis-backed state/event log and Kubernetes single-writer leases in `mecak8s` | Operate Redis, backups, recovery and retention |
| Auditing | Agent events, attribution and permission decisions | Collect network flows and filesystem/runtime evidence separately |

## Material boundaries

- The qualified local microVM path targets **Linux amd64 with KVM**, one local
  operator and Git repositories. It requires Git, Python 3, writable `/dev/kvm`
  access and unprivileged user namespaces.
- Sessions get separate worktrees within a **shared repository VM**. This separates
  Git state, not mutually hostile processes in the same VM. Retained rootfs,
  guest home and caches also matter to the isolation model.
- Providers, MCP, hooks, credentials, memory and the server remain on the host.
  Guest IPv4 egress is permissive by default; the operator can choose deny-all
  or an allowlist. External guest IPv6 is documented as unsupported/unrouted.
  Host-side traffic is outside the guest egress policy.
- Source capture has bounded accounting: 256 MiB across capture phases and
  100,000 cumulative entries. Large monorepos or dirty trees need a real trial;
  the byte budget is not a simple maximum checkout size.
- `mecak8s` defaults to no filesystem access. Its general chart does not supply
  general NetworkPolicies; the separate execution-provider chart supplies
  execution-specific policy and resource controls.
- The Kubernetes execution guide targets trusted teams and explicitly leaves
  hostile-tenant isolation and general production qualification unresolved.
  The current provider also limits delegated/background execution and retains
  workspace PVCs beyond session deletion.
- OIDC authenticates ownership; filesystem isolation still comes from the
  execution deployment. Upstream also tracks broader agent-identity/authority work.

## Revisit and pilot criteria

Do not make Mecatl a prerequisite for Ember’s first personal-assistant
prototype. First prove Ember’s core API, data ownership and narrow personal
integration flows without exposing them to repository-execution workers.

If engineering-agent work becomes an explicit requirement, start that evaluation
with the local Linux amd64 microVM path on a disposable, representative
repository. Use a release with matching verified host binaries and signed
microVM artifacts, then check `mecated microvm doctor`.

Evaluate:

1. Source capture, normal edits, patch export and retained-session resume.
2. Filesystem boundaries, sibling-worktree visibility and host credential exclusion.
3. Deny-all/allowlist guest networking plus independent observation of host
   provider, MCP and hook traffic.
4. Runtime failure: verify execution stops rather than falling back to host tools.
5. Startup time, build compatibility, CPU/memory use and retained disk growth.

If remote or multi-user engineering execution is needed, run a second pilot with OIDC, Redis and the
separate Kubernetes execution provider. Exercise identity ownership, session
recovery, policy enforcement and PVC retirement. Its RuntimeClass restrictions
are provider-specific: current docs require class scheduling selectors/tolerations
to be empty and put executor scheduling in the operator template.

## Why defer default adoption?

Mecatl adds a separate agent runtime and operating model. Ember’s proposal does
not yet establish a requirement for durable distributed coding sessions or
Kubernetes workers. Adopting it as the main assistant would also risk conflating
personal-assistant authority with untrusted repository execution. Local platform
restrictions, retained shared VM state and Kubernetes costs need measurement.

Prefer its service APIs for an initial integration. Embedding its Go engine
requires following its separate compatibility contract: reference adapters and
the root module are outside that public stability promise.

Treat Mecatl as a possible engineering-runner product and OpenCode as a possible
coding runtime; do not assume they are interchangeable or that Mecatl’s
execution provider automatically sandboxes OpenCode. Keep a small Ember-owned
task/runner interface so either can be omitted or replaced.

**Revisit when:** Ember has a concrete engineering-agent use case that requires
repository execution; the pilot meets filesystem/network boundaries and real
toolchain needs; and someone owns identity, state, runtime updates and retention.

## Version and sources

GitHub's latest-release endpoint reported **v0.0.51**, published **2026-10-26**.
The tagged local microVM guide was also checked. Current operator docs can move
ahead of a release; pin and verify the matching chart, binary, image and API
contracts before implementing the pilot.

- [Project overview](https://github.com/stacklok/mecatl)
- [Release v0.0.51](https://github.com/stacklok/mecatl/releases/tag/v0.0.51)
- [Tagged microVM guide](https://github.com/stacklok/mecatl/blob/v0.0.51/user-docs/operating/microvm-environments.md)
- [Local microVM operation](https://mecatl.dev/docs/operating/microvm-environments)
- [Kubernetes runtime](https://mecatl.dev/docs/operating/mecak8s)
- [Kubernetes execution provider](https://mecatl.dev/docs/operating/mecak8s/native-execution)
- [OIDC and service exposure](https://mecatl.dev/docs/operating/mecated/secure-and-expose)
- [Engine compatibility contract](https://github.com/stacklok/mecatl/blob/main/engine/COMPATIBILITY.md)
