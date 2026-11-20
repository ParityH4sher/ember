# Ember sandboxing

[Architecture notes](../README.md) · [Ember proposal](../IDEA.md)

Ember handles personal assistance and engineering work. These have different
trust requirements: a calendar assistant needs narrowly brokered personal
actions, while a repository runner may execute untrusted project code. Do not
put both roles in one broadly privileged agent process.

## Topic pages

| Topic | Questions it answers |
| --- | --- |
| [MicroVMs](microvms.md) | When does a local task need a separate guest kernel? |
| [Kubernetes](kubernetes.md) | How could a later multi-node deployment isolate runner jobs? |
| [Network auditing](network-auditing.md) | How to audit runner and host-service egress separately. |
| [Filesystem scope](filesystem-scope.md) | How project workspaces differ from Ember memory and personal data. |
| [Architecture proposal](architecture-proposal.md) | Cross-cutting security model, review notes, and staged implementation plan. |

## Choose the execution boundary

| Approach | Boundary | Useful for |
| --- | --- | --- |
| Rootless Podman/container | Host kernel, namespaces, capabilities and syscall restrictions | Trusted adapters or low-risk jobs with exact mounts |
| gVisor | A userspace application kernel mediating workload system calls | Additional isolation with container-style operation; check application compatibility |
| MicroVM | Hardware virtualization and a separate guest kernel | Potentially untrusted repository code on a self-hosted workstation/server |
| Kubernetes | Scheduling and policy management; isolation comes from the configured runtime and node controls | Later multi-node or multi-user deployments; adds operational burden for one host |

Record the isolation unit (task, repository or user) and threat model. Separate
Git worktrees are working-copy management, not a security boundary when jobs
share the same VM, kernel, mounts or credentials.

## Proposed execution profile

Each profile should name:

- Runtime and immutable image/kernel versions.
- Input project snapshot, writable task workspace, artifact destination and retention.
- Allowed network destinations, DNS path and audit collection points.
- Workload identity and any task-specific, short-lived credential.
- CPU, memory, disk, process and wall-clock limits.
- Run/session ID, policy version, image digest and cleanup owner.

Keep Ember's database, memory archive, API/session secrets, integration broker,
model gateway credentials and runner-management socket outside project runners.
Personal actions go through narrow host-side adapters with independent
authorization and user confirmation, not by mounting personal accounts into a
coding sandbox. Map the authenticated client/user to an authorized project and
task; use short-lived, audience-specific workload credentials only if needed.

## Candidate implementations

- [Mecatl decision](../decisions/001-mecatl.md): possible engineering runner
  backend, not an Ember personal-assistant replacement.
- [OpenCode V2 decision](../decisions/002-opencode-sdk.md): possible coding
  runner adapter controlled through Ember's task lifecycle.

If an engineering-runner prototype is prioritized, start with one disposable
repository, an untrusted-code microVM, deny-by-default network, a small artifact
export path and logs outside the guest. Keep personal-assistant and
integration-broker access disabled in that prototype. Add destinations only
after documenting the need.
