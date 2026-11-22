---
sidebar_label: Shared services
sidebar_position: 5
---

# Service layers

## Goal and status

Ember separates services by lifecycle owner and whether Ember depends on them.
This is a target architecture, not a claim about the current deployment:
the installer does not yet provision the services below. The current Go API
has prototype project/plugin endpoints, but has no authentication or durable
storage and does not connect to upstream services.

## 1. Shared default services

Shared default services are deployment-level dependencies configured by the
Ember deployment script/operator. They are not optional per-plugin integrations:
they are expected to be present and must not be deactivated through Ember's
plugin API. They may serve multiple Ember instances, so identity and data
boundaries must be enforced by the service and its adapters.

Examples under consideration:

| Service | Example | Intended role |
| --- | --- | --- |
| Identity provider (IdP) | Authentik or equivalent | Human identity and sign-in |
| Remote Git hosting | Gitea/Forgejo or equivalent | Remote project source and collaboration |
| AI gateway | LiteLLM Proxy for the first pilot | Route every model call, selecting a model automatically for the task from the requesting user's eligible models |
| Persistent control-plane storage | Database selected for the deployment | Durable Ember configuration, project/task records, permissions, and audit metadata |
| Project and artifact storage | [Instance-owned filesystem](../decisions/009-project-file-storage.md) | Portable project sources and generated artifacts, with project-relative paths |

“Default” describes the deployment profile, not a hard-coded vendor. The
deployment operator selects the implementation and configures its endpoint and
credentials. For the AI gateway and storage, the specific product and topology
remain undecided; these rows describe required capabilities, not selected
products. The Ember API does not own these services' lifecycle.

**Where they are defined.** Each shared service, default or configurable, is
an independent Compose stack in
[`infra/stacks`](https://github.com/ParityH4sher/ember/tree/main/infra/stacks).
`playbooks/stacks.yml` deploys it, separately from Ember
([ADR 010](../decisions/010-shared-service-stacks.md)). The inventory lists the
defaults in `shared_default_services`. Implemented stacks are Authentik
(identity), plus Traefik and cloudflared as the public ingress through a
Cloudflare Tunnel, which `ingress_domain` enables.

## 2. Shared configurable services

These are independently deployed or externally operated services that Ember
can integrate with. The deployment profile may run some by default and omit
others. Only adapters actually included in a deployment are returned by the
plugin discovery endpoint; unavailable adapters and their client code are not
delivered to the browser.

An available adapter can be configured and activated or deactivated for that
Ember instance through the plugin REST API. A user or agent may change
activation only when the instance's authorization policy explicitly allows it;
an agent is not implicitly trusted to change integrations. This controls
Ember's use of the service; it does **not** install, stop, update, or delete the
upstream service.
Ember must continue to work when any one of these integrations is absent or
deactivated. Each adapter has a common configuration envelope and its own
configuration schema.

Each included adapter also declares a frontend-facing operation catalog.
`GET /api/v1/plugins/{pluginId}/api` returns the operations, input/output
schemas, and Ember API route templates the frontend can use. The frontend calls
those Ember routes; it never calls the upstream service directly. The backend
maps a stable `operationId` to a fixed, plugin-owned upstream method and path,
uses the configured service credential, validates inputs, and sanitizes
responses. For example, an Immich adapter may expose a small approved subset
of Immich operations rather than proxying the entire Immich API.

Each plugin has two versions: a semantic implementation `version` and an
independent `apiVersion` for its frontend-facing contract. Invocation routes
include the plugin API major version
(`/api/v1/plugins/{pluginId}/api/{apiVersion}/{operationId}`). Compatible,
additive contract changes keep the API version; breaking changes require a new
API version while the implementation version advances for each plugin release.
The core API's `/v1` version is a separate version boundary.

The invocation route must not accept an arbitrary upstream URL, host, path, or
HTTP method from the client. The operation catalog is for frontend discovery,
not authorization: the backend must independently enforce instance/user/agent
permissions, plugin activation, upstream scopes, destination restrictions,
timeouts, response-size limits, and audit policy on every call. These operation
discovery/invocation routes are implemented for the example Weather plugin;
Immich remains configuration-only and performs no upstream requests.

Examples:

| Area | Example | Notes |
| --- | --- | --- |
| Photos and video | Immich | Optional connection to an independently operated library |
| Documents | Paperless-ngx | Optional document/search integration |
| Food and household | Mealie | Optional recipe and meal-planning integration |
| Design tools | CAD applications, Blender | Optional tools/services; execution placement and sandbox are explicit |
| Public web API | Open-Meteo Weather API | External hosted API; the example adapter calls it through Ember and is non-commercial-only under the free API terms |

The default deployment may include selected adapters and omit others. Inclusion
makes an adapter available; it does not activate it, grant upstream permissions,
or imply that an agent can invoke it. An agent may only use separately
authorized capabilities. Deactivation revokes Ember-side use and must not
affect other consumers of a shared service.

## 3. Self-maintained services

An agent may set up a service inside its own assigned, isolated environment
when the task requires it and the deployment explicitly permits that capability.
Such a service is owned by that task/environment, not registered as a shared
default or shared configurable service. It must not be assumed to survive the
task or be available to other agents or Ember instances.

Self-maintenance is not permission to administer the host. Agents must not
receive a general Docker/Podman socket, host credentials, shared-service
administrator credentials, or unrestricted network access. Provisioning,
resource limits, allowed images, ports, persistence, cleanup, and egress must
be bounded by the trusted runner/operator. If a service must persist or be
shared, promote it through an explicit operator-owned deployment decision
instead of silently moving it into the shared-service layer.

## Candidate shared-service catalog

| Area | Candidate | Intended use |
| --- | --- | --- |
| Photos | Immich | Personal photo and video library |
| Documents | Paperless-ngx | Document archive and search |
| Household | Mealie | Recipe and meal planning |
| Identity | Authentik (or an equivalent) | Human sign-in and identity provider |
| Ingress | Traefik (or an equivalent) | TLS termination and routing |
| Source hosting | Forgejo or Gitea | Private Git repositories |
| AI gateway | Shared, provider-neutral router | Task-based model selection and per-user policy enforcement; no vendor selected |
| Persistent storage | SQLite/PostgreSQL plus project-file storage, subject to decision | Durable control-plane records, project sources, and artifacts |
| Operations | Uptime Kuma, Dozzle, Prometheus, Grafana | Availability, container logs, metrics and dashboards |

This is a shortlist, not a commitment to install every candidate or select a
particular vendor. Evaluate maintenance, backup/restore, resource usage,
upstream security, API maturity, and licensing before enabling each service.

## Network and access direction

Initially, remote access is intended to use a private Tailscale network. Services
should not be published directly to the public internet by default. A future
deployment may use a Cloudflare-managed domain and an appropriately secured
Cloudflare ingress/tunnel arrangement. That is a separate deployment profile,
not a reason to weaken the private-network default.

**Status:** the Cloudflare Tunnel profile is implemented and opt-in. Without
`ingress_domain`, nothing is public. With it, cloudflared publishes only the
allowlisted `ingress_routes` through Traefik, after the safeguards in
[ADR 010](../decisions/010-shared-service-stacks.md) pass. See the
[cloudflared stack](https://github.com/ParityH4sher/ember/tree/main/infra/stacks/cloudflared#security)
for the security trade-offs.

An ingress proxy should route only explicitly enabled hostnames and paths.
Identity-aware access should protect interactive human access; service-to-service
and agent access must use separate machine identities and authorization. A
successful login at the proxy is not sufficient authorization for an agent API
operation.

## Shared service layer and agent access

The intended shared-service layer hosts independently operated applications
that can be used by Ember and, where explicitly approved, an agent. Access should
be mediated by narrow, versioned adapters (including MCP where appropriate),
not by exposing an entire service network or handing an agent an administrator
credential.

Each integration should define:

- A dedicated service identity per agent/integration, with only the required
  upstream scopes and actions.
- Explicit read/write/delete permissions, resource scope, and approval rules;
  default to read-only and deny unlisted operations.
- Short-lived or revocable credentials stored outside project files, prompts,
  source control, and worker environments that do not need them.
- A documented API/tool schema, input validation, audit trail, rate/resource
  limits, and a revocation procedure.
- Network policy allowing only the adapter to reach the required service API.
  Untrusted task workers should not receive general access to the shared
  service network.
- Human confirmation for consequential actions such as deleting or bulk
  modifying personal data, unless a separately reviewed policy says otherwise.

MCP is a protocol boundary, not a security boundary by itself. An MCP server
must enforce authentication, authorization, input limits, and audit logging
server-side. Never rely on model instructions or UI hiding to enforce access.
See the [sandboxing overview](../sandboxing/README.md) and
[API/integration contracts](api-contracts-and-integrations.md).

## Operations and trust boundaries

Treat each application as an independently updated and potentially vulnerable
component. Keep databases and administration interfaces private; separate
application networks and secrets; pin and update images deliberately; and
monitor security and service health. Monitoring dashboards, container logs, and
metrics can contain sensitive data, so access to Uptime Kuma, Dozzle, Prometheus,
and Grafana also needs authentication and least privilege.

Plan encrypted backups for application data and configuration, test restoration,
and document retention and recovery objectives before calling a service
production-ready. Do not assume a container volume is a backup.

## Phased implementation

1. Define the host/network model, threat model, identity source, backup plan,
   and service enablement mechanism.
2. Establish private ingress and human authentication; verify that admin
   surfaces remain private.
3. Add selected applications with independent data stores, secrets, health
   checks, and tested backup/restore.
4. Add monitoring with access controls and alerting.
5. Implement one narrowly scoped agent adapter at a time, starting read-only;
   test denied operations and credential revocation before enabling writes.
6. Assess a public-domain/Cloudflare profile separately, including DNS, TLS,
   origin exposure, identity enforcement, and failure modes.

These phases describe desired work, not current runtime behavior.
