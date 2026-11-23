---
sidebar_label: Project chat
sidebar_position: 7
---

# Project chat with a sandboxed agent

Project chat lets a user open a chat inside a project in the Notes app, send a
message, and read an agent's answer. Each message runs the chat's agent
**once** in the [sandbox router](https://github.com/ParityH4sher/ember/tree/main/services/sandbox-router) with the
conversation so far. The exchange is saved in the project as
`chats/<chat-id>.json`.

The design, its limits and the remaining phases are in the
[implementation plan](planning/project-chat.md). Enabling it for real users is
an open decision: [DEC-009](planning/human-decisions.md#dec-009---agent-chat-from-the-web-api).

## What works today

| Works | Not yet |
| --- | --- |
| Chats per project, listed in the Notes app, deep links with `?project=…&chat=…` | Streaming; the browser polls every 1.5 s |
| One agent run per message in the router, `trusted` (rootless Podman) or `isolated` (Kata) | Model access from the sandbox: agents run with `--network=none` |
| Conversation history in each run, oldest messages left out above 32 KiB | Project notes or files inside the sandbox |
| Transcripts as plain JSON files in the project | Durable run records, recovery of a run after an API restart |
| Fixed, user-safe error messages; the router run ID is kept for the operator | Per-user agents, message authors, project-scoped roles |
| An offline fixture agent for testing the whole path | The router in the Compose deployment |

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
| `routerSocket` | Absolute path of the router's Unix socket. The API process must be able to connect to it. |
| `agents` | 1–16 entries of `{"id", "name"}`. `id` is the agent's name in the router policy (`^[a-z][a-z0-9-]{0,62}$`); `name` is what users see. |
| `allowWithoutSignIn` | Optional, default `false`. While [sign-in](authentication.md) is off, the API **refuses to start** with an agents file unless this is `true`. Anyone who can reach the API can then start containers on the router's host, so use it only on loopback. |

The API cannot read the router's policy. A listed `id` that the router does not
know produces "This agent is not registered with the sandbox router." when a
message is sent.

The Compose deployment does not mount a router socket yet (plan phase P8). The
API container runs as UID 65532 and cannot reach the router's operator-only
socket.

## Packaging an agent for chat

A chat agent is an ordinary [router agent image](https://github.com/ParityH4sher/ember/tree/main/services/sandbox-router#bring-your-own-agent):
`/ember-agent` reads one UTF-8 document on stdin and writes its answer on
stdout. For chat that document is an `ember.chat-turn/v1` envelope:

```json
{
  "schema": "ember.chat-turn/v1",
  "project": {"id": "desk", "name": "Desk organizer"},
  "chat": {"id": "3f9c2a7b1d4e8f60", "title": "Which filament for the bracket?"},
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
- Print the answer as plain text or Markdown, at most 64 KiB, and exit 0. Ember
  trims it, shows it as text (not HTML) and treats an empty answer as a
  failure.
- A wrapper translates the envelope to its CLI's prompt format.
  `services/sandbox-router/chat-fixture/agent.py` is a short example.
- The envelope is data. Nothing in it grants tools, files or network access;
  the router's profile and limits apply to every run.

## Using a chat

- **New chat** starts a chat with the selected agent. The first message
  becomes its title.
- One message per chat can be in progress. The API runs one agent at a time,
  so a send while another chat is running is refused with "the agent runner is
  busy" and nothing is saved; send it again when the other answer arrives.
- A message is limited to 16 KiB and a chat to 1 MiB. Start a new chat when a
  chat is full.
- If the API restarts while an agent is working, that answer is shown as "The
  run was interrupted before the agent answered." Send the message again.
- Failure messages name the cause without runtime details. The operator can
  match the **Run** ID with the router's log.

## Data and privacy

Transcripts are stored with the project, next to `notes/`, and are as private
as the project's notes. They contain whatever users type and whatever agents
answer. Deleting a project keeps its files, including chats, as it does for
notes. Remove `chats/<id>.json` by hand to delete a chat.

The API logs each run's chat ID, agent, router run ID, outcome and duration,
never message text. Once agents can reach a model (P9), message text will go
to that model's provider under the rules of DEC-007.
