---
sidebar_label: Go standard library first
sidebar_position: 5
---

# Go standard library first

- **Status:** Accepted
- **Date:** 2026-11-20

## Decision

Prefer the Go standard library whenever it can meet Ember's requirements safely
and maintainably. This keeps the initial platform small, portable, and easy to
build reproducibly across server architectures.

Add a third-party dependency only when there is a concrete requirement that the
standard library does not meet well. Before adoption, document the use case,
maintenance and security posture, license, alternatives, and operational cost;
obtain human approval in line with
[decision 004](004-go-and-library-policy.md).

## Consequences

- The initial API uses `net/http` and other standard packages.
- Avoid dependencies added only for convenience or speculative future features.
- This is a preference, not a ban: security, correctness, and maintainability
  take priority over minimizing dependency count.
- Revisit dependencies as actual platform requirements emerge.
