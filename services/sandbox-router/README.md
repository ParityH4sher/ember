# Sandbox router prototype

Run one approved agent process per request on a Linux development host. The
operator registers an immutable local OCI image ID and assigns its isolation
level. The client sends an agent name and task text. Both backends run the same
image contract; neither depends on a particular agent vendor.

| Operator profile | Backend | Intended use |
| --- | --- | --- |
| `trusted` | Local rootless Podman, cgroup v2 | Reviewed tools that treat task input as data. Shares the host kernel. |
| `isolated` | Local Docker with the `io.containerd.kata.v2` shim | Coding agents that may execute generated code. Each standalone run gets a fresh Kata guest kernel. |

The service listens on an operator-only Unix socket. Anyone with access may
invoke any registered agent; keep code-executing agents exclusively under
`isolated`. If Kata is unavailable, the request fails. No default runtime
retry. The unauthenticated Ember web API and the Compose installer are not
connected to this prototype.

## Try it

From `services/sandbox-router`, with Go 1.24+, curl and rootless Podman:

```sh
run=$(mktemp -d)
go build -o "$run/router" .
podman build -t localhost/ember-sandbox-probe:dev demo
image=sha256:$(podman image inspect --format '{{.Id}}' localhost/ember-sandbox-probe:dev | sed 's/^sha256://')
cat > "$run/agents.json" <<EOF
{
  "reviewed-probe": {"profile":"trusted","image":"$image"}
}
EOF
"$run/router" -config "$run/agents.json" -socket "$run/router.sock" &
router=$!
```

```sh
curl --fail-with-body --unix-socket "$run/router.sock" http://localhost/v1/runs \
  --json '{"agent":"reviewed-probe","input":"Hello Ember"}'
```

Expected: HTTP 200, a unique `run_id`, `profile: "trusted"` and output reporting
11 bytes and the kernel release. The fixture checks the non-root UID, read-only
root filesystem, dropped capabilities, no-new-privileges, loopback-only network
and writable scratch space before returning. It does not call a model.

### Exercise the VM route

Use a KVM-capable host with a configured [Kata Containers](https://github.com/kata-containers/kata-containers)
installation, guest kernel/image, VMM, and `containerd-shim-kata-v2` discoverable
by the local Docker daemon's containerd. Run `kata-runtime check` on that host.
Pin and record the versions and qualify the actual runtime configuration on the
target machine; Ember does not install or configure it. The operator needs
access to `/var/run/docker.sock` (root-equivalent for a rootful daemon).

```sh
podman save --format docker-archive localhost/ember-sandbox-probe:dev |
  docker --host unix:///var/run/docker.sock load
vm_image=$(docker --host unix:///var/run/docker.sock image inspect localhost/ember-sandbox-probe:dev --format '{{.Id}}')
```

Register the ID from **Docker's** store; archive conversion can change it.
Restart the router to load the updated policy:

```sh
kill "$router"; wait "$router"
cat > "$run/agents.json" <<EOF
{
  "reviewed-probe": {"profile":"trusted","image":"$image"},
  "coding-probe": {"profile":"isolated","image":"$vm_image"}
}
EOF
"$run/router" -config "$run/agents.json" -socket "$run/router.sock" &
router=$!
curl --fail-with-body --unix-socket "$run/router.sock" http://localhost/v1/runs \
  --json '{"agent":"coding-probe","input":"Hello Ember"}'
```

Check for `profile: "isolated"`, the expected **guest** kernel and 11 bytes.
If the selected runtime cannot start, expect HTTP 502, not a host-kernel
result. `429` means the single slot is occupied; `504` is the run deadline.
Stop the router with `kill "$router"; wait "$router"`, then remove the temporary
directory. The images remain cached.

## Bring your own agent

Package the CLI and its wrapper at `/ember-agent` in an operator-reviewed image
without `VOLUME` declarations or embedded secrets. It starts as UID/GID 65532
in `/work`, reads one UTF-8 task to EOF on stdin, writes its text result to stdout,
and exits zero on success. A wrapper can translate this to a CLI's headless
interface. Task text is never a host command line. Register the image ID in
the policy, load it into the selected engine's local store, then restart the
router. Neither backend pulls an image during dispatch.

Both profiles request one CPU, 512 MiB memory, 128 PIDs, a read-only root,
bounded scratch tmpfs, no external network or host project mounts, one minute
wall time, and at most 64 KiB stdout. Results return to the caller; scratch is
discarded. Input is limited to a 32 KiB JSON body. Runtime status is logged by
run ID without task text or output. Use a clean operator runtime configuration:
no site-injected mounts, hooks, credentials or proxies.

For an operator-run **Git snapshot → test report and patch** using the VM
profile, follow the [snapshot pilot](pilot/README.md). Hosted-model CLIs still
need an authorized model route. There is no model broker, interactive terminal,
automatic patch publication, or durable task queue.

The router requests removal on exit, client disconnect and timeout. After a
host or router crash, inspect leftover runs with `podman ps -a --filter
label=dev.ember.sandbox-router=v1` and the equivalent local `docker ps -a`
command. Remove confirmed abandoned runs by name with the matching engine's
`rm -f`. The VM path has no independent deadline supervisor if the router dies.

Run `go test -race ./...` for request-policy, routing and output-limit tests.
Real runtime checks require running the two requests above on the target host.
