---
sidebar_label: Core installation
sidebar_position: 3
---

# Core installation and dependencies

This page distinguishes the **current core scaffold** from the services Ember
will need as agent and artifact features are implemented. It is not a promise
that planned services already ship in the installer.

## What the current installation needs

The current Compose installation runs three services:

- **API** (`apps/api`): Go HTTP service with health, project, Markdown note,
  and plugin-configuration endpoints. Projects and notes survive restarts in the
  Compose `projects` volume; plugin configuration is process-local.
  Sign-in is optional [OpenID Connect](authentication.md) and is off by
  default. Without it the API has no authentication; do not expose it to an
  untrusted network or configure real service credentials.
- **Web** (`apps/frontends/web`): the installable browser app. Its home page
  (`/`) shows API status, shared service links and plugin forms; Project notes
  (`/apps/notes/`) is the linked-note editor.
- **Gateway** (`deploy/gateway`, Compose service `frontend`): routes `/api/` to
  the API and every other path to Web.

The required host dependency is either Docker Engine with Compose v2 or Podman
with a working Compose provider. The host needs network access to fetch base
images during the initial build. No database, model provider, agent runtime,
CAD tool, or external account is required by this scaffold. The API includes
Immich configuration metadata but does not connect to Immich. Its example
Weather plugin calls the public Open-Meteo API when activated, so outbound
network access is needed only to use that operation.

Use `./install.sh` from the extracted bundle root, or run from the repository root:

```sh
docker compose -f deploy/compose.yaml up --build -d
```

The frontend binds to `127.0.0.1:3000` by default. See the
[deployment guide](deployment.md) for supported hosts, port configuration, and
security limitations. Unless [sign-in](authentication.md) is configured, the
API has no authentication; do not expose it directly to an untrusted network.

## Services planned for agent-enabled Ember

The following are architectural boundaries, **not current installation
requirements**:

| Capability | Planned component | External dependency |
| --- | --- | --- |
| Persist projects, tasks, permissions, and approvals | API plus persistent storage | Storage choice is not finalized |
| Run an agent task | AI worker in a constrained execution environment | Agent runtime/runner, to be selected |
| Call hosted or local language models | Model adapter | A configured provider or local model endpoint, only when used |
| Generate or validate CAD output | CAD worker/tool adapters | Relevant CAD applications and validators, installed only for enabled workflows |
| Display saved outputs | Frontend artifact views | Project artifact storage and supported browser formats |

These components should be enabled through explicit deployment configuration
and versioned contracts. A provider credential is only needed for the provider
being used and belongs in the trusted model adapter, not in a project file or
untrusted worker. Local-model deployments may need a reachable model server;
hosted-model deployments need provider connectivity. Neither is required for
the current health-check scaffold.

There is no defined core plugin catalog or mandatory third-party plugin set
yet. MCP or other tool integrations may be added as optional adapters; a tool
protocol does not replace Ember's project, task, artifact, or authorization
contracts. Do not install plugins as a prerequisite unless a specific enabled
workflow documents them.

## Installation readiness

Before treating an installation as a usable agent workspace, the deployment
must additionally provide and verify:

1. Persistent project and artifact storage with backups and bounded project
   paths.
2. An authenticated API and authorization policy.
3. A worker sandbox with resource limits and denied-by-default networking.
4. The selected model adapter and its secret configuration, if using an LLM.
5. Any CAD/export tools required by the chosen tasks.
6. A frontend route or viewer for each artifact media type the deployment
   intends to display.

Those capabilities are planned; the current installer does not provision them.
See [architecture](architecture/README.md),
[agent artifact guidance](agent-artifact-guidance.md), and
[sandboxing](sandboxing/README.md).
