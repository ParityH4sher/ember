# Kubernetes sandboxes for Ember engineering tasks

[Sandboxing overview](README.md) · [Network auditing](network-auditing.md) · [Filesystem scope](filesystem-scope.md)

This is a possible later execution plane for Ember's engineering runner, not a
requirement for the initial self-hosted, single-user prototype. Kubernetes
schedules jobs; the selected runtime, kernel, node configuration and policy
determine isolation. Do not place Ember's personal-assistant API or memory in
the same trust domain as arbitrary repository jobs by default.

## Runtime and namespace design

A namespace organizes resources and permissions. A `RuntimeClass` selects a
runtime handler that must already be installed and configured on the nodes.
Use Kata for a VM-backed boundary or evaluate gVisor for userspace-kernel isolation.
Standard container runtimes share the node kernel.

- Use a dedicated execution namespace with the **Restricted** Pod Security
  Standard enforced through admission.
- Constrain the allowed runtime, volume types and execution images through
  admission; the workload should not be able to replace these controls.
- Give the workload a dedicated ServiceAccount with no RBAC grants unless needed,
  and set `automountServiceAccountToken: false`.
- Keep orchestration credentials in the launcher/controller.
- Use ResourceQuota, per-container limits, bounded volumes and Job deadlines.
  Configure PID limits at the node/runtime layer.
- Separate mutually untrusted workloads into distinct runtime sandboxes and
  use dedicated nodes/clusters when required by the isolation model.

## Pod template example

This is a fragment for `Job.spec.template.spec`. Substitute an existing
RuntimeClass, ServiceAccount and a real digest-pinned tool image. The workspace
starts empty; populate it through a separately scoped input step.

```yaml
runtimeClassName: kata
serviceAccountName: sandbox-runner
automountServiceAccountToken: false
restartPolicy: Never
hostNetwork: false
hostPID: false
hostIPC: false
securityContext:
  runAsNonRoot: true
  runAsUser: 65532
  runAsGroup: 65532
  fsGroup: 65532
  seccompProfile:
    type: RuntimeDefault
containers:
  - name: runner
    image: registry.example.com/team/runner@sha256:REPLACE_WITH_DIGEST
    workingDir: /workspace
    securityContext:
      allowPrivilegeEscalation: false
      readOnlyRootFilesystem: true
      capabilities:
        drop: ["ALL"]
    resources:
      requests:
        cpu: 100m
        memory: 256Mi
        ephemeral-storage: 512Mi
      limits:
        cpu: "1"
        memory: 1Gi
        ephemeral-storage: 2Gi
    volumeMounts:
      - name: workspace
        mountPath: /workspace
      - name: tmp
        mountPath: /tmp
volumes:
  - name: workspace
    emptyDir:
      sizeLimit: 1Gi
  - name: tmp
    emptyDir:
      sizeLimit: 256Mi
```

Set `Job.spec.activeDeadlineSeconds` and a retention/TTL policy on the complete
Job. For resumable work, use a task-owned PVC and an explicit deletion lifecycle.
Read-only root filesystems do not make mounted volumes read-only.

## Network baseline

With a CNI that enforces NetworkPolicy, establish this baseline in the execution
namespace before starting work:

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: default-deny
  namespace: sandbox-dev
spec:
  podSelector: {}
  policyTypes: ["Ingress", "Egress"]
  ingress: []
  egress: []
```

Add narrow allow policies for actual dependencies. DNS generally requires both
UDP and TCP port 53 to the configured resolvers; match the cluster's CoreDNS or
NodeLocal DNS setup.

Policies are additive: another allow policy can reopen access. Standard
NetworkPolicy is primarily L3/L4, has node/host-network exceptions and
plugin-dependent behavior for other protocols. Verify enforcement against the
actual CNI, including node services, metadata addresses and IPv6.

For hostname-aware egress, use a supported CNI extension or an enforced egress
proxy. Setting `HTTP_PROXY` alone does not prevent direct connections.
The generic `mecak8s` chart and the separate Mecatl execution-provider chart have
different policy responsibilities; see [ADR 001](../decisions/001-mecatl.md).

## Inspect an existing sandbox

Run against the intended cluster and substitute the real namespace/pod:

```sh
kubectl get runtimeclass
kubectl -n sandbox-dev get pods -o wide
kubectl -n sandbox-dev get networkpolicies
kubectl -n sandbox-dev get pod RUNNER_POD -o yaml
```

Check the selected RuntimeClass and node runtime, mounted volumes, effective
identity, token mounts and policy selectors. Then exercise allowed and denied
connections using the [network audit procedure](network-auditing.md).
Inspect inside the actual runtime as well as the manifest.

## Sources

- [RuntimeClass](https://kubernetes.io/docs/concepts/containers/runtime-class/)
- [Pod Security Standards](https://kubernetes.io/docs/concepts/security/pod-security-standards/)
- [Security contexts](https://kubernetes.io/docs/tasks/configure-pod-container/security-context/)
- [NetworkPolicy semantics](https://kubernetes.io/docs/concepts/services-networking/network-policies/)
