---
sidebar_label: API and integration contracts
sidebar_position: 4
---

# How Ember defines and documents APIs

- **Status:** Findings and recommended approach; implementation choices remain
  subject to review.
- **Research checked:** 2026-11-20.

Ember will connect a browser UI, backend services, plugin integrations, MCP
servers, and external APIs. They do not all speak the same protocol and should
not be forced into one API description format.

## Recommendation at a glance

| Boundary | Contract/source of truth | Where Ember-specific policy belongs |
| --- | --- | --- |
| Frontend ↔ Ember backend over HTTP | OpenAPI document in this monorepo | Ember API authorization, user/instance ownership, and lifecycle rules in the backend |
| Ember HTTP streaming/webhooks | OpenAPI for HTTP operations; AsyncAPI when describing event/message channels is a better fit | Event authorization, retention, and task ownership in Ember |
| MCP client ↔ MCP server | The versioned MCP specification and the server's MCP schemas/tool definitions | Ember-owned integration catalog, connection config, tool allowlist, risk policy, and grants |
| Ember adapter ↔ external REST API | Upstream vendor OpenAPI, when available; otherwise the vendor's published contract | Ember adapter manifest, credential reference, permission mapping, and normalized Ember behavior |
| External events/messages | AsyncAPI or the external provider's own event schema | Ember subscription, filtering, tenant mapping, and authorization |
| Ember plugin/integration package | Versioned Ember manifest and JSON Schema | Review status, capabilities, permissions, secrets, network policy, and execution boundary |
| Multi-call business workflow | Arazzo may describe deterministic API workflows that combine OpenAPI/AsyncAPI contracts | Ember workflow engine and human-approval policy |

**A protocol contract is not an authorization policy.** OpenAPI, AsyncAPI,
MCP tool schemas, and plugin manifests describe interfaces and data. Trusted
Ember code and infrastructure must separately decide who may invoke them.

## Findings

### 1. OpenAPI is the right contract for Ember's HTTP API

The OpenAPI Initiative describes OpenAPI as a language-independent description
for HTTP APIs used by people and tooling, including documentation generation,
client generation, server tooling, and tests. It is a good shared contract for
the Go API and browser frontend.

As of this research, **OpenAPI 3.2.1** is the latest published specification
(2026-09-10). It includes richer streaming/SSE descriptions. OpenAPI 3.1.x
continues to have broad support. Specification freshness alone is not a reason
to upgrade: the parser, linter, Swagger UI, code generator, and any gateway must
all support the features Ember uses.

This repository's `contracts/openapi.yaml` currently declares OpenAPI 3.1.0.
The hosted Swagger UI is pinned to `swagger-ui-dist` 5.17.14. Research found
Swagger UI added basic OpenAPI 3.2.0 support in 5.32.0; therefore, do not change
the contract to 3.2.x until the renderer and every CI/codegen tool are upgraded
and tested together. For the current health-only API, 3.1.x is sufficient.
Re-evaluate 3.2 when defining task event streaming.

### 2. AsyncAPI covers event-driven contracts, not all HTTP APIs

AsyncAPI 3.1.0 is a protocol-agnostic specification for message-driven APIs
including channels and operations over systems such as MQTT, Kafka, AMQP, and
WebSockets. It complements OpenAPI; it is not a reason to describe every
request/response endpoint twice.

Ember task lifecycle events could remain an OpenAPI-described SSE endpoint if
that is the simplest supported design. If Ember later formalizes task events,
device events, or broker-based communication as message channels, add an
AsyncAPI document and validate it in CI.

### 3. MCP is its own protocol contract

MCP uses JSON-RPC 2.0 and defines host/client/server roles, capability
negotiation, tools, resources, prompts, and optional extensions. MCP servers
describe tool inputs with JSON Schema. HTTP-based MCP authorization has a
separate protocol specification; it is not equivalent to Ember's user-level
tool authorization.

Do not make a second OpenAPI document that attempts to redefine the MCP
protocol. Instead:

- Pin and document the MCP specification version supported by Ember's MCP
  client/gateway.
- Validate interoperability against that version's protocol schemas and
  conformance tests.
- Treat discovered server metadata and tool descriptions as untrusted input.
- Store Ember-specific server registrations and permissions in a reviewed
  catalog/manifest.
- Re-check authorization on every tool call; discovery is not permission.

The official MCP Registry is useful for public server metadata and package
discovery, but its docs currently label it preview, say it does not support
private servers, and say its own codebase is not designed for self-hosting.
It is not an authorization service, package-security guarantee, private
catalog, or runtime dependency Ember must use. A future Ember catalog may
consume public metadata while maintaining its own reviewed/approved inventory.

### 4. External APIs need adapters and upstream contract references

An external service's API belongs to that service. If the provider publishes
OpenAPI or another machine-readable contract, pin the upstream version/release
and record its URL, retrieval date, and license/redistribution constraints.
Do not silently copy mutable `main`/`latest` specs into Ember and treat them as
Ember's API.

The Ember integration adapter should:

- Isolate upstream authentication and HTTP details from the core API.
- Use a generated or hand-written client against the pinned upstream contract.
- Map upstream data/errors into a small, documented Ember-facing behavior.
- Declare which upstream operations Ember actually uses and which permissions
  they require.
- Test against provider sandbox fixtures or a mock server, and run compatibility
  checks when the pinned upstream API changes.
- Keep one adapter implementation capable of serving multiple configured
  connections, with separate endpoints and credential references.

If an external API has no formal schema, document the exact endpoints, version,
authentication, data mapping, failure behavior, and tested examples in that
integration's directory.

### 5. Plugin metadata is not plugin authority

An Ember integration manifest should identify the plugin and its compatibility
requirements, configuration schema, supported capabilities, secret references,
network destinations, data sensitivity, and risk/approval requirements.
Validate manifests with a versioned JSON Schema.

The manifest is declarative metadata—not executable policy. The backend and
execution boundary must enforce permissions. Merely listing a capability in a
manifest must never grant it to every user or agent.

## Proposed repository organization

Keep the existing `contracts/openapi.yaml` as the Ember HTTP API source for now.
Do not move it just to create a more elaborate tree. Grow the repository as
there are real contracts and integrations:

```text
contracts/
├── openapi.yaml                 # Ember-owned public HTTP API
├── project-schema/
│   └── v1.schema.json
├── plugin/
│   └── manifest.schema.json     # Ember integration/plugin metadata
└── events/
    └── asyncapi.yaml            # Add when Ember has formal event channels

integrations/
├── README.md                    # Adapter authoring and review policy
├── paperless/
│   ├── integration.yaml         # Ember adapter manifest/config schema
│   ├── README.md                # Upstream mapping, auth, risks, test guide
│   ├── upstream-api.lock.yaml   # Pinned upstream spec URL/version/hash
│   └── tests/
└── immich/
    └── ...

docs/
└── architecture/
    └── api-contracts-and-integrations.md
```

Do not create empty integration directories or vendor upstream specifications
until an integration is selected. The lock file is a proposed pattern, not an
existing implemented standard; define it when the first real adapter is added.

### Suggested integration manifest fields

This is a design sketch, not a finalized schema:

```yaml
manifestVersion: 1
id: paperless
version: 0.1.0
displayName: Paperless-ngx
compatibility:
  emberApi: ">=0.1.0"
upstream:
  documentation: https://docs.paperless-ngx.com/api/
  apiVersion: "record-the-tested-upstream-version"
configurationSchema:
  # Reference to a JSON Schema file in this integration package.
  $ref: ./config.schema.json
capabilities:
  - id: documents.search
    risk: read
    upstreamOperations:
      - document-list
    requiredScopes:
      - documents:read
network:
  destinations:
    - user-configured-service-origin
```

Never put credentials, tokens, resolved secrets, or real private hostnames in a
committed manifest. Secret references are identifiers resolved by a trusted
secret mechanism, not secret values.

## Frontend ↔ backend workflow

Use a contract-first workflow for the Ember HTTP API:

1. Propose API changes in `contracts/openapi.yaml` with request/response
   schemas, status/error responses, auth requirements, examples, and
   deprecation/compatibility notes.
2. Review the contract before implementation, so frontend and backend work
   against the same agreement.
3. Generate the frontend TypeScript types/client from that contract; do not
   hand-maintain duplicate request/response types.
4. Implement the API and test that observed behavior matches the contract.
5. Build the hosted API reference and downloadable OpenAPI file from that same
   contract.
6. Run lint, schema validation, client generation/drift checks, and breaking
   change detection in CI.

Orval can generate TypeScript clients, request functions, and mocks from
OpenAPI; `openapi-typescript` is a lighter alternative when only types are
needed. Select one after the frontend stack is chosen. Generated output should
be marked as generated and never edited manually. Pin the generator version.

The backend implementation remains authoritative for runtime behavior; OpenAPI
is the reviewed interface contract. Test both directions: every documented
operation is implemented, and no unintended public operation is exposed.

## MCP/plugin authoring and approval workflow

### Connection lifecycle proposed for the first read-only adapter

Keep four records distinct: the **definition** (reviewed adapter code and
version), **connection** (instance-owned origin and secret reference),
**grant** (who may perform a capability on that connection), and optional
**ownership record** (who may administer the upstream service). Removing one
instance's connection must not remove the definition or the service.

Register an explicit connection as *disabled*. Validate compatibility, origin
and exact capability mapping before enabling it. A health probe should be a
bounded, read-only call with separate reachability, authentication and upstream
version results. A failed probe marks the connection degraded; it must not
restart the upstream, broaden credentials or prevent the API serving unrelated
requests. On disable or revocation reject new calls, fence in-flight results,
close streams and release the connection's secret handle.

Prefer request/response reads initially. Before accepting webhooks, map a
verified publisher and subscription to an instance **on the server**, verify a
signature and timestamp on bounded bytes, deduplicate event IDs, handle
out-of-order delivery and revoke the subscription independently. A tenant ID
in a payload is not routing authority. Validate MCP tool schemas/version on
upgrade; quarantine newly discovered or changed capabilities until reviewed.
An adapter that requires executable third-party code needs a separately approved
runtime boundary, not dynamic loading into `apps/api`.

### Reviewing an MCP server

For each MCP server or plugin Ember adopts:

1. Identify the publisher, source repository, package/image, exact version, and
   integrity digest where available.
2. Review source/provenance, license, known risks, network requirements, and
   data handling. A public registry entry is discovery metadata, not a security
   review.
3. Pin the server and record supported MCP protocol version and transport.
4. Discover tools/resources/prompts in a controlled environment and review
   descriptions and schemas as untrusted content.
5. Map selected tools to Ember capability IDs, risk classes, approval rules,
   and per-user/agent/instance permissions.
6. Store endpoint and secret references per service connection. Never place
   upstream credentials in a general-purpose agent sandbox.
7. Verify positive and negative authorization cases, credential revocation,
   audit logging, and network isolation.

Do not automatically install or enable every server found in a registry. Do not
interpret MCP annotations or tool descriptions as trusted authorization rules.

## CI and documentation lifecycle

Recommended checks as the API surface grows:

- Validate OpenAPI and JSON Schema files against their declared versions.
- Lint contracts with a pinned tool/version and Ember-owned rules.
- Fail on breaking HTTP API changes unless explicitly approved and versioned.
- Generate TypeScript clients and fail if committed generated output is stale
  (or generate in CI without committing, but use one consistent policy).
- Run backend conformance/integration tests and frontend client type checks.
- Validate integration manifests and upstream lock records.
- Run adapter tests against mocks/provider sandboxes; test authorization denials.
- Validate AsyncAPI documents when introduced.
- Generate API reference pages from canonical contracts; do not manually copy
  endpoint tables into docs.
- Publish the docs site and machine-readable contract from the same revision.

For multi-call API workflows, evaluate the OpenAPI Initiative's Arazzo
specification. It describes workflows composed from OpenAPI and AsyncAPI
operations; it does not replace implementation or authorization.

Consumer/provider contract tests (for example Pact) are most useful when a
consumer and provider deploy independently and need to verify their shared
assumptions. They are likely unnecessary for every frontend/backend change in
this single monorepo; start with OpenAPI validation plus API integration tests.
Revisit Pact for independently deployed adapters/services or external
compatibility where Ember controls both test sides.

## Versioning and compatibility rules

- Version the Ember API independently from the OpenAPI specification version.
  Keep `/api/v1` and `info.version` meanings explicit.
- Prefer additive, backward-compatible changes within a supported API major
  version. Deprecate before removing an operation or field and document a
  migration window.
- Treat JSON Schema/project manifest and plugin manifest versions as their own
  contracts.
- MCP protocol version is negotiated according to MCP; it is separate from the
  Ember plugin version.
- Track upstream integration API versions independently. A vendor's breaking
  change must not silently change Ember's public API.
- Never assume a schema or generated client performs runtime authorization.

## Immediate recommendations for this repository

1. Keep `contracts/openapi.yaml` as the one canonical Ember HTTP contract and
   continue hosting it through the docs site.
2. Keep OpenAPI 3.1.x for now; upgrade Swagger UI and validate the whole toolchain
   before adopting OpenAPI 3.2.x. Re-evaluate when the task SSE contract exists.
3. Add CI lint/validation and implementation contract tests before adding
   project/task endpoints.
4. Generate the frontend client only once a real frontend framework/client layer
   is selected; avoid introducing a generator into the current static status
   page prematurely.
5. Add an Ember integration manifest schema before implementing the first
   integration. Keep manifests, user connections, secrets, and granted
   permissions as distinct data.
6. Use the MCP protocol and its schemas for MCP interoperability; add Ember-owned
   policy and a reviewed server catalog around it.
7. Add AsyncAPI only when Ember defines message/event channels that need it.
8. Record tool choices as ADRs after a small evaluation; these recommendations
   do not select a gateway, registry, plugin runtime, or API client generator.

## Sources

Sources reviewed 2026-11-20:

- [OpenAPI Specification 3.2.1](https://spec.openapis.org/oas/v3.2.1.html)
- [OpenAPI specification versions](https://spec.openapis.org/oas/)
- [Swagger announcement: OpenAPI 3.2 support](https://swagger.io/blog/swagger-launches-support-for-openapi-3-2-0/)
- [Swagger UI 3.2 support issue/release note](https://github.com/swagger-api/swagger-ui/issues/10575)
- [AsyncAPI Specification 3.1.0](https://www.asyncapi.com/docs/reference/specification/v3.1.0)
- [MCP specification](https://modelcontextprotocol.io/specification/2026-07-28)
- [MCP HTTP authorization](https://modelcontextprotocol.io/specification/2026-07-28/basic/authorization)
- [MCP tools](https://modelcontextprotocol.io/specification/2026-07-28/server/tools)
- [MCP Registry overview and limitations](https://modelcontextprotocol.io/registry/about)
- [Orval overview](https://orval.dev/docs)
- [Redocly CLI lint](https://redocly.com/docs/cli/commands/lint)
- [OpenAPI Overlay overview](https://learn.openapis.org/overlay/)
- [Arazzo Specification](https://spec.openapis.org/arazzo/latest.html)
- [Pact documentation](https://docs.pact.io/)
