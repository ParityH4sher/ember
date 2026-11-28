---
sidebar_label: "015: Harness sessions"
---

# ADR 015: Chat agents are long-lived harness sessions behind Ember

**Date:** 2026-11-28  
**Status:** Accepted  
**Replaces:** project chat phases P9 (partly), P12 (streaming part) in the
[project chat plan](../planning/project-chat.md). Builds on
[ADR 002](002-opencode-sdk.md), which keeps OpenCode out of Ember's assistant core.

## Context

Project chat ran each message as a one-shot process (`ember.chat-turn/v1` on
stdin, the answer on stdout) and replayed the whole conversation into every
run. Each feature an agent with a model and tools needs (streaming, steering,
tool permissions, interrupt, server-side history) would have been added to
that envelope by hand, although agent harnesses such as OpenCode already
provide them. At the same time, the separate review panel
(`apps/api/cmd/agent-panel`) already drove OpenCode sessions with all of these
features in about 300 lines.

Model providers were also about to have three owners: the admin catalog (with
API keys), LiteLLM (DEC-007) and each harness's own config.

## Decision

1. **A chat with a session agent is one harness session.** Ember keeps
   sign-in, project scoping, the sandbox and a copy of each transcript. The
   harness owns the agent loop, history, tools, permissions and streaming. The
   review panel's OpenCode adapter moves into the API as the `opencode`
   harness; the separate panel is retired.
2. **Ember's internal harness interface is modelled on OpenCode's session
   API** (open, prompt/steer, interrupt, messages, permissions, events), so
   other harnesses can sit behind it later. `ember.chat-turn/v1` stays for
   existing one-shot router agents and gets no new features.
3. **Providers have one owner: the model gateway (LiteLLM).** Provider
   endpoints and keys live in the gateway's configuration. The admin catalog
   only offers gateway models under a display name and maps agents to them; it
   stores no keys.

## Contract

### Go interface (`apps/api/internal/harness`)

```go
type Harness interface {
    // Open creates the session, or resumes it if it exists. Idempotent.
    Open(ctx context.Context, s Session) error
    // Prompt starts a run, or steers the active run with more input.
    Prompt(ctx context.Context, sessionID, text string) error
    // Interrupt stops the active run and reports whether one was stopped.
    Interrupt(ctx context.Context, sessionID string) (bool, error)
    // Messages returns up to limit of the newest messages, oldest first.
    Messages(ctx context.Context, sessionID string, limit int) ([]Message, error)
    Permissions(ctx context.Context, sessionID string) ([]Permission, error)
    Reply(ctx context.Context, sessionID, permissionID string, d Decision) error
    // Subscribe streams events for all sessions of this worker. The channel
    // closes when the stream fails; the caller resubscribes and resyncs.
    Subscribe(ctx context.Context) (<-chan Event, error)
}

type Session struct{ ID, Directory string }
type Message struct {
    Role  string // "user", "agent", "tool", "run"
    Text  string // text, "name: status" for tools, outcome for runs
}
type Permission struct {
    ID        string   `json:"id"`
    Action    string   `json:"action"`
    Resources []string `json:"resources"`
}
type Decision string // "once" or "reject"
type Event struct {
    Session string `json:"-"`
    Type    string `json:"type"`
    Delta   string `json:"delta,omitempty"`
}
```

Event types are Ember's names, not the harness's:
`text.delta`, `tool.called`, `tool.succeeded`, `tool.failed`, `run.started`,
`run.succeeded`, `run.failed`, `run.interrupted`, `permission.asked`,
`permission.replied`. The OpenCode harness maps `session.text.delta`,
`session.tool.called|success|failed`, `session.execution.started|succeeded|failed|interrupted`,
`permission.asked|replied` onto them and drops everything else.

The OpenCode session for a chat has the ID `ses_ember_<chatId>` (chat IDs are
16 lowercase hex characters).

### Operator configuration (agents file, `EMBER_AGENTS_CONFIG_FILE`)

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

- `routerSocket` is required only when a router agent is configured.
- A worker `url` is `http://host[:port]` with no path, query or user info. The
  password file holds at least 32 bytes and is read at startup. `directory` is
  the session working directory inside the worker (default `/workspace`).
  At most 4 workers; `harness` is `opencode` for now.
- An agent with `worker` is a **session agent**; without, it is a **turn
  agent** whose `id` is its router policy name.
- Admin catalog agents: harness `opencode` requires `worker` (one of the
  configured worker IDs) instead of `routerAgent`.

### HTTP API (all routes need the `user` role; POSTs keep the existing cross-origin check)

| Route | Session agent | Turn agent |
| --- | --- | --- |
| `GET /v1/agents` | items gain `"mode": "session"` | `"mode": "turn"` |
| `POST …/chats/{chatId}/messages` | No active run: append the user message and a pending agent message, open the session, prompt, 202. Active run: append the user message only and steer, 202 (no `chat_busy`). | Unchanged |
| `GET …/chats/{chatId}/events` | SSE (below) | SSE with `run.started` and `run.succeeded`/`run.failed`/`run.interrupted` only |
| `POST …/chats/{chatId}/interrupt` | `{"interrupted": bool}` | Cancels the run, `{"interrupted": bool}` |
| `GET …/chats/{chatId}/permissions` | `{"permissions": [Permission]}` | `{"permissions": []}` |
| `POST …/chats/{chatId}/permissions/{permissionId}` | body `{"decision": "once"\|"reject"}`, 204 | 404 `permission_not_found` |

`…` is `/v1/projects/{projectId}`. Chat responses (`GET` chat and the 202 of
`POST messages`) gain `"mode"` and `"running"` (boolean).

**Event stream.** `text/event-stream`, `Cache-Control: no-store`. The first
event is `data: {"type":"connected"}`; every later event is
`data: {"type": <event type>, "delta"?: string}` for this chat only. A comment
line (`: keepalive`) every 25 s. When the upstream harness stream fails the
API sends `data: {"type":"resync"}` and keeps the browser connection; clients
reload the chat and permissions on `connected`, `resync` and every non-delta
event. Message text deltas are never logged.

**Transcript copy.** When a session run ends (`run.succeeded`, `run.failed`,
`run.interrupted`), the API reads the session's newest messages and writes the
agent text produced since the run's first user message into the pending agent
message (`completed`), or the fixed user message for the failure (`failed`).
If the API restarts while a run is active, the next `GET` of the chat asks the
harness: an idle session's answer is recovered; otherwise the run is tracked
again. Transcripts keep the existing `ember.chat/v1` format.

**Limits.** At most 4 active session runs per worker (`429 agent_busy`
beyond). A steer message obeys the 16 KiB message limit. Run deadline for
session agents: 15 minutes, after which the API interrupts the session.

### Model gateway (`EMBER_MODEL_GATEWAY_URL`, `EMBER_MODEL_GATEWAY_KEY_FILE`)

- The catalog file becomes `ember.ai-catalog/v2`:
  `{"schema", "models": [{"id", "name"}], "agents": [...]}`. A model `id` is
  the gateway's model name. Opening a v1 file drops its providers and keys,
  keeps model IDs and names and agents, and rewrites the file.
- `GET /v1/admin/ai` returns `{persisted, chatEnabled, harnesses, workers:
  [{id, harness}], gateway: {configured: bool}, operatorAgents, models, agents}`.
- `GET /v1/admin/ai/gateway/models` lists the gateway's models
  (`GET /v1/models` with the gateway key): `{"models": [{"id"}]}`; 503
  `gateway_unavailable` when not configured or unreachable.
- Provider routes (`/v1/admin/ai/providers…`) are removed.
- `agents.ModelRef` in `ember.chat-turn/v1` is reduced to `{id, name}`.
- Provider endpoints and keys are configured in the LiteLLM overlay
  (`deploy/compose.models.yaml`), never in Ember.

## Consequences

- Session agents need a worker deployment (`deploy/compose.opencode.yaml`)
  and a model route through the gateway. The worker's workspace is shared by
  every chat of that worker: chats are separated by session, not by an OS
  boundary. Project files are still not mounted (P10).
- Which model an OpenCode worker uses is set in its `opencode.json`; the
  catalog's model choice applies to turn agents' envelopes for now.
- The admin page can no longer add provider keys; operators configure
  providers in LiteLLM. This removes key custody from the API.
- Live events depend on the API holding one upstream stream per worker; events
  are not durable (P12 still owns durable run records).
