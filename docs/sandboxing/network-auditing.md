# Auditing Ember network access

[Sandboxing overview](README.md) · [MicroVMs](microvms.md) · [Kubernetes](kubernetes.md)

Ember has at least two distinct egress planes: trusted host-side assistant
integrations/model calls and potentially untrusted engineering-runner traffic.
Audit and authorize them separately. Runner logs must not be treated as the
authoritative record of guest network activity.

Continuous monitoring and alerting are required by the
[gateway decision](../decisions/003-ai-gateway-shortlist.md#mandatory-monitoring-and-alerting).
Collect flows continuously for host services and runners; the commands below are
diagnostic spot checks.

## Define the traffic to account for

Record the run ID where applicable, time window, workload/process identity,
source and destination addresses, port/protocol, direction, byte counts, DNS
context when available, and allow/drop verdict.
Distinguish workload traffic from launcher, provider, MCP, hook and telemetry traffic.
An agent's tool history alone cannot account for connections made inside shell commands.

| Environment | Useful observation point |
| --- | --- |
| TAP-based microVM | Host TAP/bridge, firewall counters/logs and egress gateway |
| Userspace-networked microVM/container | Backend network telemetry, proxy and host process flows |
| Kubernetes | CNI flow logs, e.g. Cilium Hubble; egress gateway/proxy |
| Remote tools/providers | Host/service egress plus the remote service's own logs |

Collect primary records outside the workload and correlate NAT-translated
connections with their original run/pod identity.

## Local microVM / Linux

For a TAP-backed instance, substitute its actual host-side interface. These
commands inspect policy and print a bounded packet summary:

```sh
SANDBOX_IF=tap-sandbox0
ip -details link show "$SANDBOX_IF"
sudo nft list ruleset
sudo timeout 60 tcpdump -i "$SANDBOX_IF" -nn -q -c 200 '(ip or ip6)'
```

Inspect the relevant firewall counters before and after an allowed request and
a deliberately denied request. An observed SYN records an attempt; correlate
with the enforcement verdict and destination logs to establish delivery.
Account for host input and forwarding paths, not just internet egress.

For userspace networking, choose the backend's observation point: there may be
no TAP interface, and host flows can appear under the proxy/helper process.
`ss` inside the guest shows current sockets but is not a historical audit trail.

## Kubernetes with Cilium Hubble

Prerequisites: Cilium installed, Hubble enabled, CLI access to its API, and
permission to inspect the selected namespace.

```sh
hubble status
hubble observe --namespace sandbox-dev --since 5m
hubble observe --namespace sandbox-dev --since 5m --verdict DROPPED
hubble observe --namespace sandbox-dev --since 5m --protocol dns
```

DNS and HTTP/L7 visibility depend on configured observation/policy features.
Hubble's available flow history is bounded; export it to retained storage for
later analysis and monitor lost events. For other CNIs, use their equivalent
flow logs and policy-verdict facilities.

Kubernetes API audit logs record control-plane requests such as creating a pod
or invoking `exec`. They complement CNI logs; they do not record arbitrary pod
network connections.

## Verify the policy

Use endpoints you control and run checks from the actual sandbox identity:

1. Confirm a required destination succeeds and appears in the expected logs.
2. Confirm an unlisted destination/port is blocked and produces a policy verdict.
3. Test direct-IP access as well as DNS names, and IPv6 where available.
4. Check cluster/node services, host services and metadata endpoints against the
   intended policy.
5. Check DNS, alternate resolvers, proxy bypass and redirects to unlisted destinations.
6. Repeat after sandbox restart, credential renewal and policy changes.
7. Trigger a configured destination or traffic-volume alert; verify its identity,
   supporting flow records, and delivery to the configured notification channel.
8. Interrupt collection or export and simulate dropped events; verify that
   independent monitoring reports the resulting coverage gap.

A connection timeout alone does not prove policy enforcement: the destination
may simply be unavailable. Preserve the policy version and observed verdict.

## Visibility and retention

- TLS packet captures normally reveal connection metadata, not plaintext HTTP
  paths or bodies. Use controlled proxy/application logs for that detail.
- DNS logs miss cached resolutions, direct-IP connections and encrypted DNS.
- A permitted destination can still receive data. Restrict which files and
  credentials the workload can access using the [filesystem profile](filesystem-scope.md).
- Store the smallest useful record with bounded retention. Raw packet captures,
  URLs and command output can contain credentials or project data.
- A model-provider request may carry source content even when guest egress is
  blocked. Record and govern the host-side provider route separately.

For Mecatl specifically, guest egress controls exclude host provider, MCP, web,
hook, artifact and telemetry traffic. See [ADR 001](../decisions/001-mecatl.md).

## Sources

- [Hubble CLI flow inspection](https://docs.cilium.io/en/stable/observability/hubble/hubble-cli/)
- [Kubernetes auditing](https://kubernetes.io/docs/tasks/debug/debug-cluster/audit/)
- [NetworkPolicy semantics and limitations](https://kubernetes.io/docs/concepts/services-networking/network-policies/)
- [Firecracker host networking](https://github.com/firecracker-microvm/firecracker/blob/main/docs/design.md)
