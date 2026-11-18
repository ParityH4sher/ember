# Scoping Ember filesystem access

[Sandboxing overview](README.md) · [MicroVMs](microvms.md) · [Kubernetes](kubernetes.md)

Project workspaces must not expose Ember's persistent memory, personal files,
calendar/email integration state, API session secrets, provider credentials,
orchestration configuration or runner-management sockets. The assistant may
retrieve selected personal context through Ember-owned authorization and
adapters; that does not justify mounting the underlying data into a code runner.

## Define an explicit mount contract

| Path inside the sandbox | Intended access | Lifecycle |
| --- | --- | --- |
| `/input` | Read-only curated source snapshot | Recreated for each input revision |
| `/workspace` | Writable task copy, if editing is needed | Per run or explicitly retained |
| `/output` | Writable artifact destination | Export selected files after completion |
| `/tmp` | Writable bounded scratch space | Discarded on completion |
| Root filesystem | Read-only toolchain | Rebuilt and versioned as an image |

Expose only the task's data. Leave the host home, `.ssh`, `.kube`, 1Password
socket, SSH agent socket, Docker/Podman sockets and unrelated repositories
outside the mount set. A read-only mount still permits reading and copying its data.

### Input and output handling

- Resolve source paths and reject paths outside the approved roots. Prefer a
  staged snapshot; a one-time `realpath` check alone does not prevent later
  symlink-swap races.
- Copy only selected content. Account for `.env` files, untracked files,
  repository history, Git hooks/configuration and credential-bearing caches.
- Give each run a separate writable workspace/cache. Keep shared caches
  read-only or mediate writes through a trusted service.
- Keep mount propagation private and inspect nested mounts. Ensure read-only
  submount behavior matches the chosen kernel/runtime.
- Export bounded regular files or a reviewed patch. Validate paths and symlinks
  during export; prevent archive traversal and writes outside the output root.
- Keep credentials in specifically scoped mounts or a broker. Environment
  variables and inherited file descriptors are also access channels.

## Rootless Podman example

This illustrates the mount contract using a pre-pulled image containing `sh`.
Set `INPUTS` to an absolute sanitized input directory, `OUTPUTS` to a separate
dedicated output directory, and `IMAGE` to a real digest-pinned image reference.

```sh
: "${INPUTS:?set an absolute sanitized input directory}"
: "${OUTPUTS:?set an absolute dedicated output directory}"
: "${IMAGE:?set a pre-pulled digest-pinned image}"
podman run --rm -it --pull=never \
  --network=none \
  --read-only --read-only-tmpfs=false \
  --userns=keep-id \
  --cap-drop=ALL --security-opt=no-new-privileges \
  --pids-limit=128 --memory=1g --cpus=1 \
  --mount "type=bind,src=$INPUTS,dst=/input,ro" \
  --mount "type=bind,src=$OUTPUTS,dst=/output,rw" \
  --tmpfs /tmp:rw,nosuid,nodev,size=256m,mode=1777 \
  --workdir /input \
  "$IMAGE" sh
```

This container shares the host kernel. The explicit `--read-only-tmpfs=false`
disables Podman's automatic writable tmpfs additions; the example adds only the
scratch mount it needs. Runtime-provided `/proc` and device mounts still require
inspection. The host output bind mount also needs an external disk quota if its
size must be bounded.

For editing, add a separate writable staged workspace instead of making the
original checkout writable. For stronger kernel isolation, apply the same
contract through the [microVM](microvms.md) or [Kubernetes](kubernetes.md) backend.

## Check effective access

Inspect the running runtime's actual mounts, identity and namespace configuration,
not just the launch arguments. With Podman, use `podman inspect CONTAINER_ID`;
inside the sandbox, use `id` and `cat /proc/self/mountinfo`.

Using disposable fixture files, confirm:

- Reads from `/input` succeed; writes to it fail.
- Writes to `/output` and the intended scratch/workspace paths succeed.
- An unrelated host canary file and sibling workspace are inaccessible.
- Runtime/credential sockets and API tokens are absent.
- Symlinks and nested directories cannot expand the export destination.
- Cleanup removes scratch data while preserving only the chosen artifacts.

OpenCode's session directory and permission rules express application policy;
OS/runtime mounts enforce the filesystem boundary. Mecatl worktrees within one
repository VM share that VM's isolation boundary. See the
[OpenCode](../decisions/002-opencode-sdk.md) and [Mecatl](../decisions/001-mecatl.md)
decisions for deployment implications.

## Sources

- [Podman run options](https://docs.podman.io/en/latest/markdown/podman-run.1.html)
- [Kubernetes security contexts](https://kubernetes.io/docs/tasks/configure-pod-container/security-context/)
- [Kubernetes volumes](https://kubernetes.io/docs/concepts/storage/volumes/)
- [OpenCode V2 permissions](https://opencode.ai/docs/permissions)
