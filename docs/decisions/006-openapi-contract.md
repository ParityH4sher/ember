---
sidebar_label: OpenAPI contract
sidebar_position: 6
---

# Maintain an OpenAPI contract for the backend API

- **Status:** Accepted
- **Date:** 2026-11-20

## Decision

Maintain a versioned OpenAPI contract for Ember's backend API in the monorepo.
Use OpenAPI 3.0 or the latest OpenAPI version supported by the chosen tooling,
and document the exact specification version in the contract.

The contract is the public description of the HTTP API: paths, methods, request
and response schemas, error responses, and authentication requirements. Keep it
in sync with the implementation through review and automated validation.

Host the API reference as part of Ember's documentation site, generated/rendered
from the committed contract. The contract remains the source of truth; generated
reference pages must not become a separately maintained copy.

## Consequences

- Add the OpenAPI document alongside the backend contracts in the repository.
- Validate the document in CI and include contract changes in code review.
- Publish the generated API reference with the documentation site.
- Avoid undocumented endpoints and breaking contract changes without an explicit
  compatibility and migration plan.
- Select a rendering and validation tool when implementing the first API
  endpoints; prefer the existing Go standard-library-first policy where practical.
