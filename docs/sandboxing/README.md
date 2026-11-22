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

## Try the agent sandbox router

The [sandbox router prototype](https://github.com/ParityH4sher/ember/tree/main/services/sandbox-router)
starts a fresh agent process from an operator-approved image. A private
`POST /v1/runs` request names the agent; host policy selects rootless Podman
for reviewed tools or a Kata VM for agents that can execute code. A missing VM
runtime fails the run rather than selecting a weaker boundary. Both profiles
are offline and disposable. See `services/sandbox-router/README.md` in the
checkout for setup, an offline test agent, and a request for each profile.
This operator-side experiment is separate from the Compose stack; model access
and end-user task authorization are still planned.

## Run the offline thumbnail proof

From the Ember source root, on Linux with rootless Podman, cgroup v2, Bash
and Python 3, run:

```sh
bash services/thumbnail/qualify.sh
```

The build needs registry access on first use for its pinned Go builder.
The job itself has no external network interface. It accepts one staged PNG
(up to 4 MiB and 4 megapixels), center-crops it and writes a 128×128 PNG to
stdout. The host bounds the export; no host output directory is writable from
the worker. Each run uses disposable inputs and removes the fixture and result
on exit. Podman retains its build cache.

The script runs a separate probe with the same runtime restrictions to check
read-only input and root, non-root identity, dropped capabilities, loopback-only
networking and absence of a host credential canary. It runs the actual worker
separately and checks the export. Run it on the target Pi before qualifying
that host. This is an operator-run fixture, not an API job endpoint, queue
consumer, or connection to Immich.

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
