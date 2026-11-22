---
sidebar_label: "003: AI gateway"
---

# ADR 003: Require activity monitoring and defer AI gateway selection

[Companion notes](../README.md) · [Sandboxing](../sandboxing/README.md)

**Date:** 2026-11-18  
**Decision (updated 2026-11-21):** Require system-wide activity monitoring and alerting and a shared gateway for all model calls. [DEC-007](../planning/human-decisions.md#dec-007---model-routing-privacy-and-spending) selects LiteLLM Proxy for the first pilot: automatic task-based routing within each user's configured models and read-only MCP tool access through the same gateway. Production adoption remains open.

**Candidate evidence:** Upstream project and feature documentation reviewed 2026-11-18; no software installed or benchmarked.

## Recommendation

Compare gateway candidates for model routing, credential isolation, budget
attribution, policy enforcement, and integration with system-wide monitoring:

1. **LiteLLM** — broad provider support, routing/failover, virtual keys and spend
   budgets; likely the most direct general-purpose proxy pilot.
2. **agentgateway** — strongest candidate if Ember moves to Kubernetes and needs
   gateway-level identity, MCP policy and OpenTelemetry integration.
3. **Bifrost** — compare if a self-hosted dashboard, caching and bundled gateway
   features matter more than Kubernetes-native integration.

Selection requires runtime qualification. Monitoring and alerting are mandatory
regardless of gateway implementation.

## Mandatory monitoring and alerting

- Record client API requests, task lifecycle events, model calls, tool execution,
  spawned processes, file and data access, integration/device actions, and
  configuration or permission changes.
- Continuously observe allowed and denied network activity from Ember host
  services, sandboxes, plugins, and desktop companions, including traffic that
  bypasses the gateway. Capture connection metadata, byte counts, and available
  DNS context.
- Correlate events by timestamp, component/workload identity, operation, target,
  outcome, and policy verdict. Include user, agent, session, and task IDs where
  applicable; retain unattributed events and flag attribution gaps.
- Collect primary audit records outside agent-controlled workspaces. Define
  retention and access rules, redact credentials, and configure payload capture
  explicitly.
- Alert on unapproved destinations or actions, authentication and permission
  failures, abnormal traffic volume, cost/resource thresholds, and repeated
  execution failures.
- Make alert rules, thresholds, severity, and delivery channels configurable.
  Show alerts in the notification inbox and deliver them through configured
  channels, with the triggering rule, affected component/task, and audit evidence.
- Independently monitor collector health, dropped events, export backlogs, audit
  tampering, and notification delivery failures. Report gaps in coverage and use
  a fallback notification channel when configured.

The gateway observes routed requests. Host, runtime, filesystem, and network
instrumentation must cover other activity. Document observation points and blind
spots, including encrypted payloads and unavailable remote-service internals.
See [network auditing](../sandboxing/network-auditing.md) for capture points and
verification.

## Conditional comparison

| Gateway | Strong fit | Key qualification |
| --- | --- | --- |
| **LiteLLM** | Broad provider interface; retries/fallbacks; virtual-key model access, spend tracking and budgets; optional cache and MCP gateway | Audit logs are documented as Enterprise-only. Requires Postgres for virtual-key state; Redis is the production default for shared response cache. Semantic caching is a poor default for multi-turn/agentic traffic and needs careful correctness/privacy review. |
| **agentgateway** | Open-source Apache-2.0 proxy with standalone and Kubernetes modes; JWT/OAuth, CEL authorization, MCP OAuth/tool policy, LLM routing, OpenTelemetry and cost attribution | Its current docs say per-key dollar/token budgets require standalone mode and are not carried by the Kubernetes API; K8s uses token-bucket rate limits instead. A completed request is charged after response, so the request that crosses its budget can still succeed. No semantic response cache was established in the reviewed feature docs. |
| **Bifrost** | Apache-2.0, Go-based gateway with OpenAI-compatible API, provider routing/fallback, virtual keys/budgets, exact/semantic cache, dashboard and MCP features | Upstream README puts clustering, adaptive load balancing, guardrails and MCP gateway under Enterprise deployments. Verify precisely which edition includes each required feature and whether the free build meets HA/security needs. |

## Why not include the others?

- **Portkey:** worthwhile if its paid self-hosted/enterprise controls are acceptable;
  not a first pilot while advanced key management, MCP and tracing are tier-dependent.
- **AxonHub / New API:** consider when simpler API-key/quota/provider management
  is enough. The supplied comparison lists no MCP or caching; verify identity,
  authorization, deployment maturity and audit retention against requirements.
- **Agent Router:** revisit if provider routing and MCP aggregation become the
  dominant need; compare its policy, support and operations fit with agentgateway.
- **Higress / APISIX:** elevate if already operating that gateway platform or
  needing its conventional API-gateway plugin ecosystem. Avoid introducing a
  second general-purpose gateway just for a small LLM pilot.

## Gateway implementation decision

First test the shared gateway interface with two users and mock providers.
Submit the same task for both users without specifying a model; verify that
each selection comes from that user's configuration and that an empty eligible
set makes no upstream call. Use the same fixtures to evaluate existing gateway
products. Record which task-routing rules each supports through configuration.

With a hosted mock approved for one user, verify it is eligible across that
user's ordinary projects. Mark one project local-only and make its local
endpoint unavailable: even on retry, the hosted mock must receive no call.

Before selecting one, decide:

- Whether it handles only model API traffic or also MCP; keep those trust
  boundaries and credentials distinct.
- Which events it exports, which require independent collectors, and how the
  combined system demonstrates monitoring coverage and alert delivery.
- Whether provider secrets stay outside engineering-runner guests and how
  requests are attributed to user, conversation, task and agent.
- Which prompt/response fields are logged, retained, redacted and accessible;
  Ember traffic can contain highly sensitive personal data.
- Whether budgets are hard pre-request reservations or post-response accounting,
  and how concurrent requests avoid overspending.
- Whether OIDC is for human-to-Ember login or workload-to-gateway auth; do not
  confuse gateway authentication with user authorization inside Ember.
- The minimum deployment and maintenance burden acceptable for a self-hosted
  single-user system.

## Qualification pilot

Do not enable semantic caching initially. First test routing and governance with
a mock/local provider, then controlled real-provider calls:

1. Route one Ember provider adapter to one gateway endpoint; do not couple the
   initial experiment to Mecatl or OpenCode.
2. Use distinct, revocable keys for development, automated jobs and any agent
   service. Keep upstream provider credentials only on the gateway.
3. Test model allowlists, request/token rate limits, spend limits, fallback
   behavior, streaming, tool calls, interruption and error propagation.
4. Send a run/session correlation ID; verify per-request provider/model, tokens,
   cost, caller/key and policy decision arrive in the telemetry backend.
5. Confirm whether prompt/response bodies are retained, redactable and disabled
   by default where sensitive source code could appear. Check log access and TTL.
6. Measure overhead, restart behavior, credential rotation, Postgres/cache
   recovery, and (if required) HA. Record licensing for every feature.
7. Test the [LiteLLM MCP gateway](https://docs.litellm.ai/docs/mcp) with a
   read-only Immich tool. Verify that each instance can read only its own test
   photos, and that tool-service credentials never reach the model.
8. Exercise model, process, file, integration, and network actions; verify
   correlated audit records, including traffic that bypasses the gateway.
9. Trigger policy denials and usage thresholds; verify alerts. Interrupt a
   collector and a notification channel; verify gap detection and delivery recovery.

Use gateway rate/budget enforcement as an additional control, not as the only
protection against runaway work. Apply workload-level timeouts, concurrency
limits and a hard external spend alert/cap where provider billing allows.

## Candidate by future project shape

- **Initial Ember routing prototype:** exercise the shared router with mock
  providers and verify per-user selection and monitoring before enabling real calls.
- **Kubernetes service with OIDC and MCP policy:** trial agentgateway first,
  while documenting the Kubernetes-mode budget limitation.
- **Self-hosted gateway with built-in cache/UI/MCP:** trial Bifrost, after a
  feature-by-feature OSS-versus-Enterprise check.
- **Primary need is sandboxed engineering execution:** qualify the runner
  separately; a model gateway does not supply a sandbox.

## Sources

- [LiteLLM virtual keys and budgets](https://docs.litellm.ai/docs/proxy/virtual_keys)
- [LiteLLM routing/fallbacks](https://docs.litellm.ai/docs/proxy/load_balancing)
- [LiteLLM response caching](https://docs.litellm.ai/docs/proxy/caching)
- [LiteLLM audit logs and license note](https://docs.litellm.ai/docs/proxy/audit_logs)
- [Bifrost upstream repository/README](https://github.com/maximhq/bifrost)
- [Bifrost governance](https://docs.getbifrost.ai/features/governance/budget-and-limits)
- [Bifrost retries and fallbacks](https://docs.getbifrost.ai/features/retries-and-fallbacks)
- [Bifrost semantic caching](https://docs.getbifrost.ai/features/semantic-caching)
- [agentgateway upstream repository](https://github.com/agentgateway/agentgateway)
- [agentgateway LLM cost controls](https://agentgateway.dev/docs/standalone/latest/documentation/llm/cost-controls/)
- [agentgateway Kubernetes budget limitation](https://agentgateway.dev/docs/standalone/latest/documentation/llm/cost-controls/budget-limits/)
- [agentgateway MCP authorization](https://agentgateway.dev/docs/standalone/latest/documentation/mcp/mcp-authz/)
- [agentgateway LLM observability](https://agentgateway.dev/docs/standalone/latest/documentation/llm/observability/)
