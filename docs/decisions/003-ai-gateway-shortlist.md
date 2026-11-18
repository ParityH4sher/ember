---
sidebar_label: "003: AI gateway"
---

# ADR 003: Defer AI gateway selection for Ember

[Companion notes](../README.md) · [Sandboxing](../sandboxing/README.md)

**Date:** 2026-11-18  
**Decision:** Do not select a gateway for the initial Ember prototype. Build a small provider-adapter boundary first; compare LiteLLM, agentgateway and Bifrost only when a concrete operational need appears.  
**Evidence:** Upstream project and feature documentation reviewed 2026-11-18; no software installed or benchmarked.

## Recommendation

If Ember demonstrates the need for centralized model routing, provider credential
isolation, budget attribution or policy enforcement, compare:

1. **LiteLLM** — broad provider support, routing/failover, virtual keys and spend
   budgets; likely the most direct general-purpose proxy pilot.
2. **agentgateway** — strongest candidate if Ember moves to Kubernetes and needs
   gateway-level identity, MCP policy and OpenTelemetry integration.
3. **Bifrost** — compare if a self-hosted dashboard, caching and bundled gateway
   features matter more than Kubernetes-native integration.

These are not current implementation recommendations. Ember’s proposal does not
yet establish multiple providers, concurrent users, model credential brokering,
or a need for a separately operated gateway. A gateway adds another privileged
service and may log sensitive personal prompts, memory, calendar context or
source code. It is not a replacement for task sandboxing.

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

## Gate for introducing a gateway

First establish a narrow Ember-owned provider interface and store provider
credentials in the host-side secret store. Add a gateway only if requirements
such as multi-provider routing, per-agent budgets, key isolation, centralized
model policy or reliable telemetry cannot be met proportionately in that layer.
Avoid building a gateway merely because the project may someday use MCP or
Kubernetes.

Before selecting one, decide:

- Whether it handles only model API traffic or also MCP; keep those trust
  boundaries and credentials distinct.
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

## Suggested pilot (only after the gate is met)

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
7. Add MCP only if centralized tool auth/allowlists are needed. Validate caller
   identity propagation and ensure tool-service credentials never reach the model.

Use gateway rate/budget enforcement as an additional control, not as the only
protection against runaway work. Apply workload-level timeouts, concurrency
limits and a hard external spend alert/cap where provider billing allows.

## Candidate by future project shape

- **Initial single-user Ember prototype:** use a minimal provider adapter;
  direct provider configuration is simpler unless credential isolation or
  accounting is already a demonstrated requirement.
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
