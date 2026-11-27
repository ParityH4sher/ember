---
sidebar_label: Modularity
sidebar_position: 3
---

# Modular architecture and feature boundaries

Ember should be modular at both the frontend and backend. The core provides
stable shared services and contracts; optional features and integrations should
be independently understandable, configurable, testable, and replaceable.
Modularity is a design direction, not a claim that the current scaffold already
has a plugin system.

## Core and modules

The core is responsible for cross-cutting behavior that must remain consistent:

- HTTP API and versioned contracts.
- Identity, authorization, and ownership.
- Task lifecycle and event delivery.
- Configuration and secret references.
- Approvals, audit records, and policy enforcement.

Features build on those shared services rather than duplicating them. Examples
of possible feature modules include projects, memory, chat/agent runs, voice,
calendar and tasks, artifact viewers, fabrication workflows, and device
adapters. These are examples from product direction; they are not implemented
by the current scaffold.

Backend modules should have explicit inputs, outputs, configuration, required
capabilities, and lifecycle. They should use the public API or internal
interfaces/contracts rather than reach into another module's private storage
or mutable global state. A module can be disabled without breaking unrelated
core behavior. Where a feature needs a worker or external service, that
dependency should be explicit and independently deployable when practical.

Frontend modules should be independently navigable and renderable, with their
own routes, UI components, and API client usage. They should use the same
versioned API and shared design/accessibility conventions, not call backend
internals or introduce feature-specific server routes outside the contract.
The shell can compose enabled modules into one experience; a module should not
require every other feature to be installed.

The web app has two small, compile-time extension points so far: a
[widget registry](../widgets.md) for dashboard widgets, and an
[app registry](../app-launcher.md) the app launcher reads. Both validate
entries and ignore bad ones with a warning. Neither loads code from outside the
build.

## Plugins and integrations

A plugin is an extension boundary, not an automatic grant of authority. An
integration package should declare its identifier and version, Ember
compatibility, configuration schema, capabilities, secret references, network
destinations, and approval/risk requirements. Ember must enforce those
permissions independently of plugin metadata. See
[API and integration contracts](api-contracts-and-integrations.md) for the
proposed manifest and review workflow.

Prefer adapters for provider- or device-specific behavior. Keep provider
credentials and upstream protocol details in trusted adapters; expose
provider-neutral behavior to the rest of Ember. Optional integrations should
not become mandatory dependencies of the core installation.

## Contract and compatibility rules

- Version public API, event, project, task-result, and module contracts
  independently where their compatibility needs differ.
- Make additive changes by default; document deprecations and migration windows.
- Treat configuration schemas as part of a module's public interface.
- Test a module in isolation and test its integration through the same
  contracts used by other clients.
- Keep data exportable in documented formats so removing a module does not
  strand user data.
- Never treat a prompt, manifest, plugin description, or frontend visibility
  rule as a security boundary.

## Current implementation status

The current repository has a compile-time plugin registry and adapter interface,
but no dynamic module loader or installable plugin packages. The notes client
uses SvelteKit and Vite; sign-in is optional OpenID Connect and there is no task system. The Go API exposes health,
filesystem-backed projects and notes, in-memory plugin configuration, and a versioned
Weather adapter; the Nginx frontend serves a linked-note editor and proxies `/api/`.
The API and frontend are separate Compose services, but that deployment split
alone is not a feature/plugin architecture. See
[architecture](README.md) and [core installation](../core-installation.md).

The supplied personal-Ember feature list is a **wishlist for product direction**,
not a report of capabilities implemented in this repository. See the
[feature catalogue](../feature-catalogue.md). Its example paths and access
methods describe the source wishlist/reference system; they are not paths or
interfaces in this codebase. The current repository status remains the small
scaffold described above.
