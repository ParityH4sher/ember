---
slug: /
sidebar_label: Overview
sidebar_position: 1
---

# Ember documentation

Decision records, architecture, operations, and product planning for Ember, a
self-hosted AI design and fabrication workspace. The implementation status is
called out on each page; the product proposal in [IDEA.md](IDEA.md) describes
the intended destination, not currently implemented capabilities. See the
[inspiration list](inspiration.md) for related projects and products.

All project documentation and planning live under `docs/` and are served by
Docusaurus. See the [documentation workflow](documentation.md) for the
devcontainer and hot-reload setup.

## Where to start

- Developers: [architecture overview](architecture/overview.md), [baseline evidence](architecture/current-state.md) and [testing strategy](engineering/testing-strategy.md).
- Operators: [deployment](deployment.md), [operations](operations/README.md) and [release blockers](planning/risks.md).
- Contributors and coding agents: [continuation checkpoint](progress/current-status.md), [roadmap](planning/roadmap.md) and [decisions requiring approval](planning/human-decisions.md).

## Platform and design references

- [Architecture](architecture/README.md): service boundaries, current status, and
  deployment portability.
- [Modularity](architecture/modularity.md): intended frontend/backend feature
  boundaries, plugin principles, and a status-qualified inventory of features
  mentioned in the Ember v2 review artifact.
- [AI gateway architecture](architecture/ai-gateway.md): gateway boundaries,
  candidate evaluation, model routing, and security requirements.
- [Shared self-hosted services](architecture/shared-services.md): planned
  personal services, private ingress, identity, monitoring, and least-privilege
  agent access.
- [API and integration contracts](architecture/api-contracts-and-integrations.md):
  where HTTP APIs, MCP tools, plugin manifests, and upstream API contracts live.
- [Operations](operations/README.md): local Compose setup, Ansible provisioning,
  CI artifacts, and known operational limitations.
- [Build and deploy](deployment.md): API bundle installation and requirements.
- [Multi-display](multi-display.md): opening Notes with a blank workspace on
  every display, browser limits, and troubleshooting.
- [Always on top](always-on-top.md): "Pin to top" in Notes and operating
  system options for keeping windows above others.
- [Sign-in (OIDC)](authentication.md): optional OpenID Connect sign-in, its
  configuration file, and provider setup.
- [Models and workers](models.md): configuring providers in the LiteLLM model
  gateway, offering its models in Ember, and running an OpenCode worker.
- [API reference](api-reference.md): hosted OpenAPI contract and interactive
  Swagger UI.
- [POC implementation handoff](https://github.com/ParityH4sher/ember/blob/main/EMBER_POC_IMPLEMENTATION_BRIEF.md): detailed
  proposed scope, architecture, acceptance criteria, and agent instructions.

The current implementation is intentionally small: a health-check API and a
static frontend. Sign-in is optional OpenID Connect, off by default. AI, CAD,
authorization, and task workflows are not implemented. Future service boundaries are documented in the repository
under `services/`.

## Sandboxing

Start with the [sandboxing overview](sandboxing/README.md), then choose a topic:

- [Sandboxing architecture proposal](sandboxing/architecture-proposal.md):
  integration trust classes, capability policy, review notes, and implementation plan.
- [MicroVMs](sandboxing/microvms.md): VM boundaries, local execution and lifecycle.
- [Kubernetes sandboxes](sandboxing/kubernetes.md): runtimes, identity, volumes and network policy.
- [Network auditing](sandboxing/network-auditing.md): capture points, flow logs and verification.
- [Filesystem scope](sandboxing/filesystem-scope.md): mount allowlists, scratch space and artifacts.

## Product direction

- [Feature catalogue](feature-catalogue.md): the wished-for product capabilities,
  grouped by area and distinguished from what this repository implements today.
- [Plugins](plugins.md): the list of wanted plugins and the Weather plugin as
  the worked example for adding one.
- [Widgets](widgets.md): the web app's widget contract and how to build and
  register a widget for the dashboard.
- [App launcher](app-launcher.md): registering apps (at startup and at
  runtime), launch targets, keyboard shortcuts and configuration.
- [Gaps and improvements](gaps-and-improvements.md): prioritized lessons from
  the reference system, kept separate from this repository's implementation status.
- [Inspiration](inspiration.md): design tools, fabrication systems, workshop
  integrations, physical interfaces, and related AI workspaces.
- [Agent project contract](agent-project-contract.md): portable project
  definitions, artifact formats, and requirements for agent work and reporting.

## Implementation policy

[Go and library choices](decisions/004-go-and-library-policy.md): prefer Go where
appropriate; evaluate libraries and obtain human approval before adoption or
development.

[Go standard library first](decisions/005-go-standard-library-first.md): prefer
standard packages and justify third-party dependencies against concrete needs.

[OpenAPI contract](decisions/006-openapi-contract.md): maintain a versioned
backend API contract and host its generated reference with the documentation.

## Integration decisions

Evaluated against upstream documentation on **2026-11-18**. These are design
recommendations based on documentation review; runtime qualification is still pending.

| Candidate | Decision | Best fit |
| --- | --- | --- |
| [Mecatl](decisions/001-mecatl.md) | Defer for the initial prototype; retain as an engineering-runner candidate. | Sandboxed repository tasks if that feature becomes concrete. |
| [OpenCode V2 SDK/client](decisions/002-opencode-sdk.md) | Not part of assistant core; evaluate for repository coding tasks. | Coding sessions behind Ember’s task API. |
| [AI gateway shortlist](decisions/003-ai-gateway-shortlist.md) | Require system-wide monitoring and alerts; defer gateway selection pending qualification. | Provider routing, budget controls, and integration with activity and network auditing. |

The assistant/orchestrator and isolated engineering runner are distinct roles.
Keep Ember’s API, memory, personal integrations and authorization in Ember;
evaluate an agent runtime only as a replaceable implementation for project tasks.

## Contracts

The backend contract is maintained in [`contracts/openapi.yaml`](pathname:///openapi.yaml)
and the project manifest schema in
[`contracts/project-schema/`](pathname:///project-schema/README.md). Update
contracts alongside implementation changes; do not treat planned endpoints or
schemas as implemented behavior.

## Extending these notes

Add topic pages beside the existing ones; Docusaurus adds them to the sidebar
automatically. Link key topics here. Record technology
choices under `decisions/` with the date, recommendation, evidence, tradeoffs,
and conditions that would change the decision.

These decision records supersede the initial workstation-oriented notes. They
are reframed for Ember and should be revisited as the design proposal becomes
an implementation with concrete deployment and threat-model requirements.
