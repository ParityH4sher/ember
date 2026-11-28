# OpenCode worker

The standalone review panel that lived here (its own login, Compose project
and Nginx model route) is retired. Its OpenCode adapter moved into the Ember
API as the `opencode` harness ([ADR 015](../../../docs/decisions/015-harness-sessions.md)):
a chat with a session agent is one OpenCode session, driven by the API for
signed-in users.

```text
browser → Ember API → OpenCode worker (Kata) → LiteLLM → model providers
```

The deployment now lives with the other Compose overlays:

- [`deploy/compose.opencode.yaml`](../../../deploy/compose.opencode.yaml): the
  worker. Same pinned image (OpenCode 2.3.2 by digest), Kata runtime by
  default, tmpfs workspace, no published port, reachable only from the API.
- [`deploy/opencode/opencode.json`](../../../deploy/opencode/opencode.json):
  the worker's only model provider, the gateway, with a LiteLLM virtual key.
- [`deploy/compose.models.yaml`](../../../deploy/compose.models.yaml): the
  LiteLLM model gateway, which holds provider keys.

Set them up with the [models and workers guide](../../../docs/models.md).
