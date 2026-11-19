---
slug: /
sidebar_label: Overview
sidebar_position: 1
---

# Ember documentation

Decision records and sandbox design notes for the Ember proposal in
[IDEA.md](IDEA.md). Ember is proposed as a self-hosted AI design and fabrication
workspace; these are proposals, not implemented capabilities. See the
[inspiration list](inspiration.md) for related projects and products.

All project documentation and planning live under `docs/` and are served by
Docusaurus. See the [documentation workflow](documentation.md) for the
devcontainer and hot-reload setup.

## Sandboxing

Start with the [sandboxing overview](sandboxing/README.md), then choose a topic:

- [MicroVMs](sandboxing/microvms.md): VM boundaries, local execution and lifecycle.
- [Kubernetes sandboxes](sandboxing/kubernetes.md): runtimes, identity, volumes and network policy.
- [Network auditing](sandboxing/network-auditing.md): capture points, flow logs and verification.
- [Filesystem scope](sandboxing/filesystem-scope.md): mount allowlists, scratch space and artifacts.

## Product direction

- [Inspiration](inspiration.md): design tools, fabrication systems, workshop
  integrations, physical interfaces, and related AI workspaces.
- [Agent project contract](agent-project-contract.md): portable project
  definitions, artifact formats, and requirements for agent work and reporting.

## Implementation policy

[Go and library choices](decisions/004-go-and-library-policy.md): prefer Go where
appropriate; evaluate libraries and obtain human approval before adoption or
development.

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

## Extending these notes

Add topic pages beside the existing ones; Docusaurus adds them to the sidebar
automatically. Link key topics here. Record technology
choices under `decisions/` with the date, recommendation, evidence, tradeoffs,
and conditions that would change the decision.

These decision records supersede the initial workstation-oriented notes. They
are reframed for Ember and should be revisited as the design proposal becomes
an implementation with concrete deployment and threat-model requirements.
