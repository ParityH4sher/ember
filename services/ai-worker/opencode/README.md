# Run the Ember review panel

One operator controls one disposable OpenCode V2 session. The panel is an
optional Ember frontend with its own Go controller. It uses a separate origin
and login from the main gateway. No agent or provider is added to the core install.

```text
browser → panel/controller → OpenCode in Kata → fixed chat route → your model
                 ↑                |
                 └─ session events┘
```

## Start

Use a Linux Docker Compose host with a qualified `io.containerd.kata.v2` shim
(see the [sandbox router prerequisites](../../sandbox-router/README.md)).
The worker requires Kata and has no published port. Supply an operator-controlled,
OpenAI-compatible chat-completions endpoint reachable **from the gateway
container**, with a model that supports streaming and tools. A loopback-only
model on the Docker host isn't reachable as `localhost` in the container.
The supplied gateway is for a private model endpoint or an existing broker
that owns provider credentials. It forwards no worker authorization header.

From the Ember source root, in one shell:

```sh
cd services/ai-worker/opencode
export EMBER_PANEL_PASSWORD="$(openssl rand -hex 32)"
export OPENCODE_PASSWORD="$(openssl rand -hex 32)"
export REVIEW_MODEL_ORIGIN='http://model.lan:11434' # operator's origin, no path
export REVIEW_MODEL='qwen3:8b'                    # exact upstream model ID
docker compose up --build -d
```

Open **http://127.0.0.1:3010/apps/agent/**. Log in as `operator` with the
value of `EMBER_PANEL_PASSWORD`. For a remote Docker host, forward this
loopback port over SSH. The controller-to-worker password is separate.

1. Ask “Review `example.go` and explain how it handles invalid input.”
2. While the run is active, send “Focus on overflow.” Both messages target
   the same session; OpenCode receives the second with `delivery: steer`.
3. The panel shows live text and tool activity. Reply to a tool request with
   **Allow once** or **Reject**. Reloading the page recovers the last 50
   messages and pending requests from the worker. EventSource reconnects and
   resyncs after a dropped connection.
4. **Stop run** calls the session interrupt endpoint and reports its actual
   boolean result. This is not guest teardown. When finished:

   ```sh
   docker compose down --volumes
   unset EMBER_PANEL_PASSWORD OPENCODE_PASSWORD
   ```

The worker has a read-only example source plus temporary writable workspace,
home and scratch. Recreating it loses the session and edits; the main project
vault is never mounted. The only application route out to the model is
`POST /v1/chat/completions`; the model endpoint should enforce its own budget
and model policy. This prototype doesn't provision a model, schedule per-user
VMs, persist tasks outside the guest, or publish artifacts.

## Checks

`go test ./cmd/agent-panel` from `apps/api` exercises session reuse, steering,
stream filtering, recent history, interruption and the operator boundary with
a test worker. To check the actual V2 protocol without a model account, run
`OPENCODE_TEST_BINARY="$(command -v opencode)" go test -count=1 -run '^TestOpenCodeProtocol$' ./cmd/agent-panel`
from `apps/api`. This starts a private foreground server with an empty temporary
home and a scripted local model. It checks live text, history recovery, a
steering follow-up and an active interrupt. Use the deployment steps above to
qualify a real model and Kata on the target host.
API shapes target [OpenCode V2](https://opencode.ai/docs/api); worker image:
2.3.2, pinned by digest.
