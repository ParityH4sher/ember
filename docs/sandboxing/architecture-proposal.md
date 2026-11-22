# Ember sandboxing architecture proposal

- **Status:** Reviewed proposal; policy and runtime implementation are incremental.
- **Scope:** Security boundaries for service integrations, processing jobs,
  executable plugins, AI tools, and shared infrastructure.
- **Implementation status:** Ember ships a health API, prototype project/plugin
  configuration endpoints, and one fixed Weather-to-Open-Meteo operation. It
  does not implement authentication, capability authorization, a general plugin
  loader, a job runner, or an infrastructure controller. Do not expose the
  unauthenticated API to an untrusted network.

[Sandboxing overview](README.md) · [POC implementation brief](https://github.com/ParityH4sher/ember/blob/main/EMBER_POC_IMPLEMENTATION_BRIEF.md)

## Recommendation

Use defense in depth and capability-based authorization. Ember should broker
explicit capabilities, not give integrations or agents unrestricted control of
machines. Application authorization, runtime isolation, and infrastructure
management are separate boundaries and must be enforced separately.

```text
User / client
    |
    v
Ember API — identity, authorization, workflow
    |
    v
Policy and capability checks — logical boundary; may initially live in the API
    |
    +--> Service adapters — narrowly scoped authenticated APIs
    |
    +--> Job dispatcher — isolated, bounded processing tasks
    |
    +--> Infrastructure controller — separately authorized allowlisted changes
              |
              v
        Compose / future runtime adapters
```

Ember Core must not execute arbitrary shell commands or receive a container
runtime socket. Infrastructure changes should be declared, validated,
authorized and audited by a trusted controller. A manifest documents requested
capabilities; it never grants them by itself.

## Distinct integration models

| Model | Examples | Security boundary |
| --- | --- | --- |
| Service integration | Immich, Paperless, external APIs | Authenticate to the service API; apply user and service-side authorization. A connection does not imply lifecycle ownership. |
| Restricted processing | CAD conversion, rendering, document processing | Ephemeral job with explicit input/output mounts, non-root identity, resource limits, timeout, and no unnecessary network. |
| Untrusted execution | Generated scripts, unknown executable plugins | Disposable stronger boundary such as a microVM or qualified sandbox runtime; deny network by default and provide no host credentials. |

Ordinary containers share the host kernel and are not a strong hostile-code
boundary. Use stronger isolation when executing untrusted code. WebAssembly/WASI
is an option only for compatible workloads, not a universal replacement for
application containers.

## Capability and isolation rules

- Validate each requested capability against the authenticated caller, target
  resource, integration policy, and operation before dispatch.
- Enforce runtime restrictions independently of the manifest and application
  authorization.
- Give each integration its own least-privilege credentials; keep secrets out
  of manifests, logs, and general-purpose worker environments.
- Stage only the authorized input; make it read-only where practical. Export
  bounded outputs to a separate workspace and clean up temporary data.
- Deny unnecessary network access and prevent lateral access to sibling
  integrations, management APIs, and metadata services.
- Apply CPU, memory, process, disk, and wall-clock limits. Audit requester,
  action, policy/runtime profile, result, and correlation ID.
- Require explicit approval or a narrow allowlist for sensitive operations,
  including infrastructure changes and physical-device actions.
- Keep Ember instances' identities, policies, credentials, and job workspaces
  independent. Shared infrastructure does not imply shared authorization.

An illustrative future manifest may request capabilities and a profile:

```yaml
integration:
  id: ember.cad.converter
  version: 1.0.0
security:
  profile: restricted
  capabilities:
    - documents.read
    - workspace.write
    - jobs.execute
  filesystem:
    input: read-only
    output: read-write
    host_access: false
  network:
    policy: deny-by-default
  resources:
    memory: 2Gi
    cpus: 2
    timeout: 300s
```

This is illustrative, not a committed schema. It must not be accepted as
authorization without policy validation and runtime enforcement.

## Runtime guidance

Use hardened containers for trusted adapters and low-risk processing. Consider
rootless Podman/Docker where supported; drop capabilities, prohibit privilege
escalation, run non-root, use read-only filesystems and narrowly scoped mounts,
and set resource limits. Use systemd sandboxing for suitable native host
services, not as a replacement for isolating hostile code. Evaluate gVisor,
Kata, Firecracker, or WASI against workload compatibility and hardware before
selection. Kubernetes is a future option when multi-node scheduling justifies
its operational cost.

## POC review and implementation plan

The current POC brief is the source of truth for scope. This proposal does not
override its Compose-only deployment rule or bring arbitrary AI execution,
Kubernetes, a plugin marketplace, or autonomous install/upgrade/delete into
scope. Its systemd discussion is general guidance only; Ember application
services in this POC remain Compose-managed.

1. **Now:** harden the existing API container in Compose and drop unnecessary
   privileges from both existing containers. The API image runs non-root; the
   frontend image's user still needs separate hardening. Do not add a job runner.
2. **Next:** define integration identity/connection and capability contracts
   alongside authentication and authorization tests before implementing a real
   service adapter.
3. **When processing is selected:** implement an ephemeral restricted job with
   explicit staged input/output, network policy, resource/time limits, cleanup,
   and negative tests.
4. **Before untrusted code:** select and qualify a stronger disposable runtime
   on target Pi/server hardware; do not claim ordinary Compose containers
   safely contain malicious code.
5. **Later, when justified:** add a runtime adapter or separately governed
   infrastructure controller. Keep lifecycle authority out of Ember Core and
   require auditable allowlisted operations.

## Review notes and open decisions

- The existing sandboxing pages already cover microVMs, Kubernetes, network
  auditing, and filesystem scope. This page records the cross-cutting policy
  and aligns it with the POC rather than duplicating those runbooks.
- The POC currently has no authentication, capability broker, secret manager,
  processing runtime, or audit subsystem; these are design requirements, not
  implemented controls.
- Select a real service integration and credentials model before finalizing
  capability names or a manifest schema.
- Qualify network enforcement and stronger isolation on the selected Pi model
  before enabling executable untrusted workloads.
- Define multi-instance identity and authorization before claiming tenant
  isolation; separate credentials alone do not guarantee downstream data
  isolation.

## References

- [NIST SP 800-190: Application Container Security Guide](https://csrc.nist.gov/pubs/sp/800/190/final)
- [Kubernetes Pod Security Standards](https://kubernetes.io/docs/concepts/security/pod-security-standards/)
- [Docker Engine security](https://docs.docker.com/engine/security/)
- [Docker rootless mode](https://docs.docker.com/engine/security/rootless/)
- [systemd.exec](https://www.freedesktop.org/software/systemd/man/latest/systemd.exec.html)
- [WASI security](https://wasi.dev/security)
- [gVisor](https://gvisor.dev/) · [Kata Containers](https://katacontainers.io/) · [Firecracker](https://firecracker-microvm.github.io/)
