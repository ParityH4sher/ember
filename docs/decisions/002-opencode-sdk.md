---
sidebar_label: "002: OpenCode SDK"
---

# ADR 002: Evaluate OpenCode V2 as an Ember engineering runner

[Companion notes](../README.md) · [Sandboxing](../sandboxing/README.md) · [Mecatl decision](001-mecatl.md)

**Date:** 2026-11-18  
**Decision:** Do not make OpenCode part of Ember’s personal-assistant core. If
Ember adds repository coding tasks, evaluate the V2 client against a separately
managed worker; embed the SDK only inside a deliberately isolated worker.  
**Evidence:** Official V2 documentation; no integration or runtime qualification performed.

## Could it help?

**For the engineering-workspace portion.** OpenCode can drive coding sessions,
stream progress and manage repository tasks without parsing terminal output.
It is not Ember’s general personal-assistant orchestrator: personal memory,
calendar/email actions, identity, scheduling and confirmation policy remain
Ember-owned. Keep coding tools and their permissions separate from those
personal capabilities.

## SDK versus client

| Package | Documented behavior | Recommended use |
| --- | --- | --- |
| `@opencode/sdk` | `OpenCode.create()` hosts OpenCode in-process, routing API calls in memory without an HTTP listener | An application running inside a dedicated worker and owning its lifecycle |
| `@opencode/client` | Typed calls to an existing OpenCode HTTP server, with async-iterable event streams | A UI/controller outside the execution worker |
| `@opencode/client/service` | Node-side discovery, startup and authentication for a local background service | Deliberate integration with a local desktop service |

For sandboxed automation, select an explicit per-worker endpoint rather than
accidentally discovering the developer's broadly privileged desktop service.
Use versions compatible with the worker's V2 API; several API surfaces are
documented as experimental.

## Proposed architecture

```text
Ember client / engineering task API
        |
Ember authorization + per-project/task policy
        |
Run controller using @opencode/client
        |
Dedicated OpenCode worker in a microVM (initially) or configured Kubernetes sandbox (later)
        |
Scoped workspace + explicit provider/tool egress + external audit collection
```

The Ember task controller maps the authenticated user to an allowed project,
starts or selects the worker, and records its run/session ID. Treat caller identity, service
authentication and provider credentials as separate concerns. Keep long-lived
user login credentials in the controller; use scoped workload credentials or a
provider broker for model access.

If in-process embedding is useful, put the SDK host inside that worker:

```ts
import { OpenCode } from "@opencode/sdk"

await using opencode = await OpenCode.create()
const session = await opencode.sessions.create({
  location: { directory: "/workspace" },
})

await opencode.sessions.prompt({
  sessionID: session.id,
  text: "Review the staged project and summarize the changes.",
})
```

This is the documented V2 API shape. Configure the worker's provider access,
state paths and permissions separately. Where explicit resource management is
unavailable, close the host with `await opencode.close()`.

## Execution and audit boundaries

- A session's `location.directory` selects its working context. It is not an OS
  filesystem sandbox. The official permissions guide states that shell tools
  run with the host user's filesystem, process and network authority.
- Put the runtime and all locally executing tools/plugins inside the intended
  boundary. Scope mounted source, output, state and credentials using the
  [filesystem profile](../sandboxing/filesystem-scope.md).
- OpenCode permission rules are useful application controls. Enforce network
  destinations, mounted paths and resource limits at the runtime/platform layer.
- The SDK guide documents embedding, not a complete shared-service OIDC or
  tenant-isolation design. Implement and verify caller-to-project/session
  authorization in the application or the selected deployment platform.
- Client event subscriptions are **live-only**, without automatic reconnection
  or replay. Persist run records, resubscribe after failure and resynchronize
  session state through the API; the live stream alone is incomplete audit evidence.
- Cancelling a client request or stopping event consumption is not proof that
  remote work stopped. Implement the session interrupt/lifecycle flow and verify
  tool-process termination before destroying or reusing a worker.
- Correlate session/tool events with independent network and runtime records.
  Provider traffic, remote MCP execution and shell subprocesses need explicit coverage.

## Tradeoffs and decision

For a coding-task feature, this reuses OpenCode's sessions, tools, plugins and
client contract. A separate worker can keep Ember's memory, personal integrations,
controller credentials and filesystem out of tool execution. This is not a
reason to put OpenCode in the critical path for ordinary assistant requests.

The deployment still needs worker provisioning, identity mapping, quotas,
audit retention and failure recovery. Compare Mecatl only if distributed
execution/session management becomes a requirement; it is a separate runtime,
not an OpenCode sandbox adapter.

**First proof of concept (only after coding tasks are in scope):** use one
disposable project and one worker; create a session, submit a review, record
progress/results, reconnect after a dropped stream, interrupt a running task,
and verify filesystem/network boundaries. Measure compatibility and cost before
expanding to concurrent users.

**Revisit the package choice when:** an HTTP boundary becomes unnecessary
(embed the SDK), or execution moves to remote workers (use the client).
Reassess the backend if operating distributed state and worker infrastructure
becomes the dominant requirement.

## Sources

- [OpenCode V2 embedded SDK](https://opencode.ai/docs/build/sdk)
- [OpenCode V2 TypeScript client and service lifecycle](https://opencode.ai/docs/build/client)
- [V2 API reference](https://opencode.ai/docs/api)
- [V2 permissions and shell authority](https://opencode.ai/docs/permissions)
