---
sidebar_label: "004: Go and libraries"
---

# ADR 004: Prefer Go and review library choices

[Documentation overview](../README.md) · [Project proposal](../IDEA.md)

**Date:** 2026-11-18  
**Status:** Accepted.  
**Basis:** Project-owner direction.

## Language choice

- Prefer Go for services, command-line tools, and integrations when it fits the
  requirements and established engineering practice.
- Use another language when platform support, available tooling, or a mature
  ecosystem makes it the better choice. Document the technical reason.
- Prefer the Go standard library where sufficient, then maintained Go libraries
  that meet the requirements.
- Avoid reimplementing mature tools or libraries solely to use Go.

## Library evaluation and approval

Evaluate each proposed third-party library before adoption:

- Required functionality, alternatives, compatibility, and API stability.
- Maintainer capacity, release and issue history, security response, and tests.
- License, transitive dependencies, upgrade cost, and a replacement or maintenance
  plan if upstream support ends.

Low commit frequency alone does not establish that a mature library is abandoned.
Assess maintenance quality and project stability.

Record the evaluated version, evidence, tradeoffs, and recommendation in a
decision record or linked dependency review. Obtain explicit human approval
before adding the library; record the approver and approval date.

## Internal libraries and ongoing maintenance

- Create a reusable internal library only for a documented need that existing
  libraries do not adequately meet.
- Assign a maintainer and define tests, compatibility expectations, and a
  maintenance plan. Obtain human approval before development.
- Re-evaluate dependencies when maintenance, ownership, licensing, or security
  support changes. Obtain human approval for replacements.
