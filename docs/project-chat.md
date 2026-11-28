---
sidebar_label: Project chat
sidebar_position: 7
---

# Project chat with a sandboxed agent

Project chat lets a user open a chat inside a project in the Notes app, send a
message, and read an agent's answer. The exchange is saved in the project as
`chats/<chat-id>.json`. An agent runs in one of two modes
([ADR 015](decisions/015-harness-sessions.md)):

- A **turn agent** runs **once** per message in the
  [sandbox router](https://github.com/ParityH4sher/ember/tree/main/services/sandbox-router)
  with the conversation so far (`ember.chat-turn/v1`).
- A **session agent** is a long-lived session in an agent harness (OpenCode)
  on a **worker**. The harness keeps the history, runs tools, asks for
  permissions and streams its progress; Ember keeps sign-in, project scoping
  and a copy of the transcript. Messages sent while it works steer the run.

The design, its limits and the remaining phases are in the
[implementation plan](planning/project-chat.md). Enabling it for real users is
an open decision: [DEC-009](planning/human-decisions.md#dec-009---agent-chat-from-the-web-api).

## What works today

| Works | Not yet |
| --- | --- |
| Chats per project, listed in the Notes app, deep links with `?project=…&chat=…` | Streaming for turn agents (only their start and end are events) |
| Streaming for session agents: live text, tool and permission events over SSE (`…/events`) | Model access for turn agents: they run with `--network=none` |
| Session agents: steering, interrupt, tool permissions, history kept by the harness | Project notes or files inside the sandbox or the worker (P10) |
| One agent run per message in the router, `trusted` (rootless Podman) or `isolated` (Kata) | Durable run records; events are not stored (P12) |
| Conversation history in each turn run, oldest messages left out above 32 KiB | Recovery of a turn run after an API restart |
| Transcripts as plain JSON files in the project; a session run's answer is recovered after an API restart | |
| Fixed, user-safe error messages; the router run ID is kept for the operator | Per-user agents, message authors, project-scoped roles |
| An offline fixture agent for testing the whole path | The `isolated` profile in the Ansible deployment |
| The Compose deployment with a host router (`deploy/compose.agents.yaml`, Ansible `ember_agents_enabled`) | |

Because agents have no network yet, an agent can only answer from what is in
its image and the envelope. The included fixture answers with a fixed
description of what it received. Real model answers need the model route
planned as P9.

## Turn on chat for local development

You need Linux with rootless Podman and cgroup v2, Go 1.24 or newer (or the
pinned `golang` image) and Node 24. Run these from the repository root.

1. **Build the fixture agent and start the router.** The socket directory must
   be yours and mode `0700`.

   ```sh
   run=$(mktemp -d) && chmod 0700 "$run"
   podman build -t localhost/ember-chat-fixture:dev services/sandbox-router/chat-fixture
   image=$(podman image inspect --format '{{.Id}}' localhost/ember-chat-fixture:dev)
   printf '{"chat-fixture":{"profile":"trusted","image":"sha256:%s"}}\n' "${image#sha256:}" > "$run/policy.json"
   (cd services/sandbox-router && go build -o "$run/router" .)
   "$run/router" -config "$run/policy.json" -socket "$run/router.sock" &
   ```

2. **Write the API's agents file.** `allowWithoutSignIn` is needed only because
   this local API has no sign-in. Leave it out anywhere else.

   ```sh
   printf '{"routerSocket":"%s","allowWithoutSignIn":true,"agents":[{"id":"chat-fixture","name":"Offline fixture"}]}\n' \
     "$run/router.sock" > "$run/agents.json"
   ```

3. **Start the API and the Notes app.**

   ```sh
   (cd apps/api && EMBER_AGENTS_CONFIG_FILE="$run/agents.json" go run .) &
   (cd apps/frontends/web && npm ci && npm run dev)
   ```

4. Open <http://localhost:5173/apps/notes/>, create a project, select
   **New chat**, type a message and select **Send**. "Agent is working…"
   appears, then the fixture's answer.

To stop, end the API, the dev server and the router (`kill %1 %2 %3` in the
same shell), then remove `$run`.

## Agents file

`EMBER_AGENTS_CONFIG_FILE` names a JSON file. Without it, chat runs are off:
existing transcripts stay readable, and **New chat** is hidden.

| Field | Meaning |
| --- | --- |
| `routerSocket` | Absolute path of the router's Unix socket. The API process must be able to connect to it. Required only when a turn agent is listed; without it, turn runs (including admin-added agents) fail with "The sandbox router is not reachable." |
| `workers` | Optional, at most 4 harness workers: `{"id", "harness": "opencode", "url", "passwordFile", "directory"}`. `url` is `http://host[:port]` with no path, query or user info. `passwordFile` holds the worker's password (at least 32 bytes; surrounding whitespace is ignored) and is read at startup. `directory` is the session working directory inside the worker (default `/workspace`). |
| `agents` | 0–16 entries of `{"id", "name", "worker"?}` (with none, admins add agents in the web app; see [Agents and models](#agents-and-models-for-admins)). Without `worker`, `id` is the agent's name in the router policy (`^[a-z][a-z0-9-]{0,62}$`). With `worker` (one of the worker IDs) it is a session agent. `name` is what users see. |
| `allowWithoutSignIn` | Optional, default `false`. While [sign-in](authentication.md) is off, the API **refuses to start** with an agents file unless this is `true`. Anyone who can reach the API can then start containers on the router's host, so use it only on loopback. |

The API cannot read the router's policy. A listed `id` that the router does not
know produces "This agent is not registered with the sandbox router." when a
message is sent.

A file with a session agent:

```json
{
  "routerSocket": "/run/ember-router/router.sock",
  "workers": [
    {"id": "opencode", "harness": "opencode", "url": "http://opencode-worker:4096",
     "passwordFile": "/run/secrets/opencode_password", "directory": "/workspace"}
  ],
  "agents": [
    {"id": "chat-fixture", "name": "Offline fixture"},
    {"id": "coder", "name": "Coder", "worker": "opencode"}
  ]
}
```

## Session agents

A chat with a session agent is the OpenCode session `ses_ember_<chat-id>` on
its worker. The API holds one event stream per worker and talks to the worker
with its password (basic auth, user `opencode`); browsers never reach the
worker. Every chat of a worker shares its workspace: chats are separated by
session, not by an OS boundary. Which model a worker uses is set in its
`opencode.json`, routed through the model gateway.

| Route (under `/v1/projects/{projectId}/chats/{chatId}`) | Session agent | Turn agent |
| --- | --- | --- |
| `POST …/messages` | No active run: saves the message and a pending answer, opens the session and prompts it. Active run: saves the message and steers the run. | One router run, as before |
| `GET …/events` | Server-sent events (below) | `run.started` and the run's end only |
| `POST …/interrupt` | Stops the session's run: `{"interrupted": bool}` | Cancels the run |
| `GET …/permissions` | `{"permissions": [{"id", "action", "resources"}]}` | Always empty |
| `POST …/permissions/{permissionId}` | `{"decision": "once"\|"reject"}`, 204 | 404 `permission_not_found` |

`GET /v1/agents` items and chat responses (`GET` chat, the 202 of
`POST …/messages`) carry `"mode": "session"|"turn"`; chat responses also carry
`"running"`.

**Events.** The stream starts with `data: {"type":"connected"}`, then sends
`data: {"type": …, "delta"?: …}` for this chat only: `text.delta`,
`tool.called`, `tool.succeeded`, `tool.failed`, `run.started`,
`run.succeeded`, `run.failed`, `run.interrupted`, `permission.asked`,
`permission.replied`. A `: keepalive` comment follows every 25 s. When the
worker's stream fails, the API sends `resync`, keeps the browser connection,
resubscribes with backoff (1 s up to 30 s) and sends `resync` again once it is
back. Clients reload the chat and its permissions on `connected`, `resync` and
every event except `text.delta`. Run end events are sent after the transcript
is written.

**Transcript copy.** When the worker reports that a run ended, the API reads
the session's newest 200 messages and writes the agent text produced since the
run's first message (at most 64 KiB) into the pending answer, or a fixed
failure message ("The agent run failed…", "The run was interrupted…"). Steer
messages follow the pending answer in the transcript. If the API restarts
while a run is active, the next `GET` of the chat asks the worker: an idle
session's answer is recovered; a busy session's run is tracked again.

**Limits.** At most 4 active session runs per worker (`429 agent_busy`
beyond); steering is not limited. A message, including a steer, is limited to
16 KiB. After 15 minutes the API interrupts the run and records "The agent did
not answer within the time limit." Worker errors are logged without their
body and shown as `502 worker_unavailable`.

## Turn on chat in a deployment

The router stays a host process, because it starts containers with the host's
rootless Podman. The API container (UID 65532) reaches its socket through a
shared group: the router's `-client-group` option makes the socket directory
mode `0710` and the socket `0660`, both owned by that group, and
`deploy/compose.agents.yaml` adds the API to the group and mounts the
directory at `/run/ember-router`. Chat needs [sign-in](authentication.md) in a
deployment; the installer and Ansible refuse it without.

### With Ansible

Set these for the host, for example in its `group_vars`, and run
`playbooks/site.yml`:

```yaml
ember_oidc_enabled: true      # already on with the Authentik stack
ember_agents_enabled: true
```

The `sandbox_router` role then installs Podman, creates the `ember-router` user
(with subordinate IDs and lingering) and the `ember-agents` group, builds the
router in the pinned Go image with no network and the offline fixture in
`ember-router`'s Podman storage, writes `/opt/ember-router/policy.json`, and
runs `ember-sandbox-router.service`. The `ember` role writes
`/opt/ember/config/agents.json` and adds the overlay. Every agent uses the
`trusted` profile. To register your own agents, build their images as
`ember-router` and list them in `sandbox_router_agents`
(`roles/sandbox_router/defaults/main.yml`); set `sandbox_router_fixture: false`
to drop the fixture.

Check it on the host:

```sh
systemctl status ember-sandbox-router
sudo ls -l /var/lib/ember-router-socket   # srw-rw---- ember-router ember-agents router.sock
```

then send a message in a project chat, signed in.

### With install.sh

Run the router as an operator account that belongs to a dedicated group, for
example `ember-agents`, with its socket in a persistent directory (not under
`/run`, which would be replaced on restart and leave the API's mount empty):

```sh
sudo install -d -o "$USER" -g ember-agents -m 0710 /var/lib/ember-router-socket
rm -f /var/lib/ember-router-socket/router.sock
router -config policy.json -socket /var/lib/ember-router-socket/router.sock -client-group ember-agents
```

Write the agents file with the **container** path of the socket:

```json
{"routerSocket": "/run/ember-router/router.sock",
 "agents": [{"id": "chat-fixture", "name": "Offline fixture"}]}
```

and install with sign-in configured as well:

```sh
EMBER_AUTH_CONFIG_FILE=… EMBER_OIDC_CLIENT_SECRET_FILE=… \
EMBER_AGENTS_CONFIG_FILE=/etc/ember/agents.json \
EMBER_ROUTER_SOCKET_DIR=/var/lib/ember-router-socket ./install.sh
```

`EMBER_ROUTER_GID` defaults to the group of `EMBER_ROUTER_SOCKET_DIR`. The
agents file must be readable by UID 65532. A changed agents file takes effect
when the API restarts.

## Packaging an agent for chat

A chat agent is an ordinary [router agent image](https://github.com/ParityH4sher/ember/tree/main/services/sandbox-router#bring-your-own-agent):
`/ember-agent` reads one UTF-8 document on stdin and writes its answer on
stdout. For chat that document is an `ember.chat-turn/v1` envelope:

```json
{
  "schema": "ember.chat-turn/v1",
  "project": {"id": "desk", "name": "Desk organizer"},
  "chat": {"id": "3f9c2a7b1d4e8f60", "title": "Which filament for the bracket?"},
  "model": {"id": "sonnet", "name": "Claude Sonnet"},
  "messages": [
    {"role": "user", "content": "Which filament for the bracket?"},
    {"role": "agent", "content": "PETG, because…"},
    {"role": "user", "content": "And outdoors?"}
  ],
  "omittedMessages": 0
}
```

- Answer the **last** message, which is always the user's. `messages` holds
  earlier user messages and completed answers, oldest first. Failed turns are
  left out.
- `omittedMessages` says how many older messages were left out to fit the
  router's 32 KiB input limit.
- `model` is present only for an agent an admin added with a model. `id` is
  the model name to request from the model gateway, never an endpoint or a
  key.
- Print the answer as plain text or Markdown, at most 64 KiB, and exit 0. Ember
  trims it, shows it as text (not HTML) and treats an empty answer as a
  failure.
- A wrapper translates the envelope to its CLI's prompt format.
  `services/sandbox-router/chat-fixture/agent.py` is a short example.
- The envelope is data. Nothing in it grants tools, files or network access;
  the router's profile and limits apply to every run.

## Agents and models for admins

Admins manage agents from the web app: **Admin → Agents & models**
(`/admin/ai/`, API `/v1/admin/ai`). Users see the result in the **New chat**
agent list. The page has three parts:

| Part | What an admin enters | Notes |
| --- | --- | --- |
| **Model providers** | Name, kind (Anthropic, OpenAI, Ollama, OpenAI-compatible), base URL, API key | Anthropic and OpenAI use their public API by default and are always *hosted* (https only). Ollama and OpenAI-compatible servers can be marked *local*, which allows plain http on your network. **Fetch models** lists the provider's models through the API. |
| **Models** | Name, provider, the provider's model ID (`qwen3:8b`, …) | The model's ID is the alias agents ask the gateway for. A model an agent uses cannot be deleted. |
| **Agents** | Name, harness (Claude Code, Codex, OpenCode, custom), router agent, models, default model, on/off | The router agent is the image's name in the sandbox router policy on the host; registering the image is still an operator task (see [Packaging an agent](#packaging-an-agent-for-chat)). |

To let admins add every agent, the operator's agents file may list no
agents, only `routerSocket`. Agents in that file stay read-only on the page and
their IDs cannot be reused.

**Keys.** The API stores provider keys in `EMBER_AI_CATALOG_FILE` (mode 0600, the
Compose `ai` volume) and never returns them: the page shows only "key set".
Saving a provider with a new base URL or kind and no new key removes the stored
key, so a key is only sent to the address it was entered for. Keys are never
put in the chat envelope or the sandbox.

**Subscriptions.** Claude Pro/Max and ChatGPT plans are not API keys. A harness
such as Claude Code or Codex signs in with them inside its own image; check
that your plan allows that use before relying on it (DEC-007).

**Not yet.** Agents still run with `--network=none`, so a model chosen here is
passed to the agent as a name only. Real model answers need the gateway route
(P9): LiteLLM configured from this catalog, holding the keys, with per-agent
virtual keys. Model choices are instance-wide; per-user approval of hosted
models (DEC-007) is also still to do.

## Using a chat

- **New chat** starts a chat with the selected agent. The first message
  becomes its title.
- With a turn agent, one message per chat can be in progress. The router runs
  one agent at a time, so a send while another chat is running is refused
  with "the agent runner is busy" and nothing is saved; send it again when the
  other answer arrives.
- With a session agent, a message sent while the agent works steers it, and
  **Stop** interrupts it. A tool call that needs approval waits until someone
  allows it once or rejects it.
- A message is limited to 16 KiB and a chat to 1 MiB. Start a new chat when a
  chat is full.
- If the API restarts while a turn agent is working, that answer is shown as
  "The run was interrupted before the agent answered." Send the message again.
  A session agent keeps working in its worker, and its answer appears when
  the chat is next opened.
- Failure messages name the cause without runtime details. The operator can
  match the **Run** ID with the router's log.

## Data and privacy

Transcripts are stored with the project, next to `notes/`, and are as private
as the project's notes. They contain whatever users type and whatever agents
answer. Deleting a project keeps its files, including chats, as it does for
notes. Remove `chats/<id>.json` by hand to delete a chat.

The API logs each run's chat ID, agent or worker, router run ID, outcome and
duration, never message text or text deltas. A session agent's worker keeps
its own copy of the conversation, and its model provider receives the message
text under the rules of DEC-007; the same applies to turn agents once they can
reach a model (P9).
