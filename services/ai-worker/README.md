# AI worker (planned)

This component is not implemented. Its future responsibility is to execute a
persisted Ember task using an agent runtime behind a replaceable interface.

For an interactive single-task experiment, see the
[OpenCode review panel](opencode/README.md). It has its own operator login,
Compose project, and disposable worker; it is not a persisted Ember task service.

## Required boundary

- Receive a task ID and a scoped, time-limited execution grant from the API.
- Never receive API/provider credentials unless a specific operation requires
  them; provider access should go through the model-adapter.
- Read and write only the task's declared project inputs and output area.
- Report structured progress and a terminal result; never expose host paths or
  arbitrary shell execution as an API feature.
- Run in a disposable sandbox with denied-by-default networking and resource
  limits. Container isolation alone is not a complete security boundary.

The [sandbox router prototype](../sandbox-router/README.md) exercises the
launch boundary locally, before a persisted task/grant protocol is available.
See [sandboxing](../../docs/sandboxing/README.md).
