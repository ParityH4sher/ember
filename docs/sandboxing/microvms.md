# MicroVMs for Ember engineering tasks

[Sandboxing overview](README.md) · [Network auditing](network-auditing.md) · [Filesystem scope](filesystem-scope.md)

## Fit for Ember

A microVM is most relevant to Ember's **engineering runner**: repository code,
build tools and dependencies may be untrusted. It offers a separate guest kernel
without requiring Ember's API, memory database, integration broker or model
gateway to move into a VM. It is likely disproportionate for trusted, narrow
personal-service adapters.

Treat it as defense in depth, not proof of isolation. The host still owns the
VMM, disks, networking, runner lifecycle and every service exposed to the guest.
Maintain a separate threat model and review host/guest data crossings.

### Candidate building blocks

- **Firecracker:** a minimal KVM-based VMM. Its API configures guest CPUs, memory,
  block devices and network interfaces; a launcher must supply lifecycle,
  workspace import/export and network enforcement.
- **Kata Containers:** a container-compatible runtime backed by lightweight VMs,
  useful when the surrounding deployment is Kubernetes/containerd.
- **Mecatl local microVM execution:** an integrated repository-agent workflow,
  but with its own harness and repository-scoped VM design. It is not a generic
  microVM API for Ember; see the [Mecatl decision](../decisions/001-mecatl.md).

## Host and lifecycle design

1. Verify virtualization support, access to `/dev/kvm` and compatibility of the
   selected runtime. A VM-hosted workstation also needs supported nested virtualization.
2. Pin the VMM, guest kernel and root filesystem as one tested combination.
3. For Firecracker, use its jailer, default seccomp filtering, unprivileged
    execution and cgroup limits. Restrict the management socket to the runner
    supervisor. Do not let agents access the VMM API or lifecycle authority.
4. Give each isolation unit a clean root image and dedicated writable data.
   Set read-only backing drives where appropriate; avoid shared writable disk images.
5. Track the instance, disks, network endpoint, task, user and logs under one run ID.
6. On completion, export bounded artifacts and dispose of per-run resources.
   For resumable work, specify which disks are retained and who can reattach them.

Snapshots may contain memory, credentials and application state. Create reusable
templates before injecting task credentials, and re-establish identity on resume.
Benchmark startup, memory overhead and real build time for the intended toolchain.

## Network design

For Firecracker, guest network devices use host TAP devices. **Firecracker does
not filter network traffic**: enforce egress in the host network path. Ember's
proposed deny-by-default policy means a prototype should start with no guest
egress and require an explicit operator policy for each needed destination.

- Start disconnected or with default-deny egress.
- Permit only the DNS resolver, artifact mirror or proxy needed by the task.
- Apply policy to guest-to-host traffic as well as forwarded traffic.
- Cover IPv4 and IPv6; record unsupported address families explicitly.
- Keep VMM control sockets and host management services unreachable from the guest.
- Capture flows outside the guest so its processes cannot erase the primary record.

The runner's provider/model route must be intentionally designed: either let the
guest reach a narrowly scoped gateway endpoint, or have a host-side broker make
requests on its behalf. Do not mount the gateway's provider credential in the
guest. A host broker still needs request attribution, task identity, limits and
its own egress logs.

Other microVM backends may use userspace networking rather than TAP. Identify
their actual enforcement and observation points instead of assuming Firecracker's
network layout. Host-side model/MCP calls need their own audit coverage.

## Filesystem design

Prefer a curated source snapshot and per-run scratch disk. For directory sharing,
limit exports to exact task directories and document read/write permissions.
Export results through a constrained file/patch channel rather than mounting a
guest-modified filesystem directly into the workstation kernel.

Verify that a task can change its workspace and export results while unrelated
host directories, credential files and runtime sockets remain inaccessible.
Repeat the checks after resume, not just on the initial boot.

## Sources

- [Firecracker design and host integration](https://github.com/firecracker-microvm/firecracker/blob/main/docs/design.md)
- [Kata Containers architecture](https://github.com/kata-containers/kata-containers/blob/main/docs/design/architecture/README.md)
- [gVisor architecture](https://gvisor.dev/docs/architecture_guide/intro/)
- [Mecatl local microVM guide](https://mecatl.dev/docs/operating/microvm-environments)
