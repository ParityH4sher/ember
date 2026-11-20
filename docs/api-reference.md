---
sidebar_label: API reference
sidebar_position: 4
---

# API reference

The canonical API contract is [`openapi.yaml`](pathname:///openapi.yaml). The
documentation build publishes the same file at `../openapi.yaml`; no copy is
maintained.

- [Interactive Swagger UI](pathname:///swagger-ui/)
- [Download/view the OpenAPI document](pathname:///openapi.yaml)

The current contract only describes `GET /healthz`. Authentication, project,
task, event, and artifact endpoints are future work and must be added to the
contract as they are implemented.

See [API and integration contracts](architecture/api-contracts-and-integrations.md)
for how Ember plans to define frontend/backend APIs, MCP tools, plugin manifests,
and external service contracts.
