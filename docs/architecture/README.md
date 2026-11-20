---
sidebar_label: Architecture
sidebar_position: 2
---

# Ember architecture

Ember is a portable software platform, separate from the host and physical
infrastructure on which it runs. The logical layers do not require separate
machines or independently deployed services.

```mermaid
flowchart TB
  U[Browser / PWA] -->|HTTP| F[Frontend]
  F -->|/api proxy| A[Go API]
  A --> DB[(Persistent project and task data - planned)]
  A --> W[Task execution interface - planned]
  W --> CAD[CAD worker - planned]
  W --> M[Model adapter - planned]
  D[Compose deployment] --> F
  D --> A
  I[Ansible host provisioning] --> D
  H[Linux host / Pi / server] --> I
```

## Implemented now

- `apps/api`: Go HTTP process exposing `GET /healthz`.
- `apps/frontend`: static status page; Nginx proxies `/api/` to the API.
- `deploy/compose.yaml`: runs the API and frontend on one host. The frontend is
  published on loopback by default at `http://127.0.0.1:3000`.
- `infra/ansible`: initial Debian/Ubuntu host preparation and Compose deployment.
- `contracts/openapi.yaml`: contract for the implemented health endpoint.
- `contracts/project-schema/`: initial portable project manifest schema.

## Planned boundaries

The AI worker, model adapter, and CAD worker have documented responsibilities
but are not running services yet. When implemented, they should communicate
through versioned contracts and task IDs, not through host paths or direct
database access.

Frontend and backend features should be modular and optional, with plugins
declaring their contracts and required capabilities. This is a design direction,
not an implemented plugin system. See [modularity](modularity.md) for boundaries,
compatibility rules, and the status of features mentioned in the supplied Ember
v2 review artifact.

The API owns authentication, authorization, project/task lifecycle, and
approvals. Model credentials belong to a trusted model adapter. Untrusted
generated code belongs in a separately constrained execution boundary. See the
[sandboxing overview](../sandboxing/README.md).

The longer-term plan for separately hosted personal applications and controlled
agent integrations is described in [shared self-hosted services](shared-services.md).
Those applications and their identity/ingress layer are not currently deployed
by Ember.

## Deployment portability

Compose is the initial deployment adapter for one host. Application contracts
must not depend on Compose, Docker, a specific Pi model, or a specific model
provider. Ansible provisions the host and deploys the Compose application;
it is not part of the application runtime. Kubernetes and multi-host scheduling
are deferred until there is a concrete need for them.

Ember services are deployed only as containers under Compose today. Kubernetes
is a future container-orchestration option; systemd units are not an Ember
deployment method.
