---
sidebar_label: Shared services
sidebar_position: 5
---

# Shared self-hosted services (planned)

## Goal and status

The server is intended to host useful personal services alongside Ember, with
carefully controlled access for people and agents. The services below are a
product/deployment direction, **not part of the current installer**. Ember
currently ships only a health API and static frontend; it does not provision
these applications, authenticate users, or expose agent tools.

## Candidate service catalog

| Area | Candidate | Intended use |
| --- | --- | --- |
| Photos | Immich | Personal photo and video library |
| Documents | Paperless-ngx | Document archive and search |
| Household | Mealie | Recipe and meal planning |
| Identity | Authentik (or an equivalent) | Human sign-in and identity provider |
| Ingress | Traefik (or an equivalent) | TLS termination and routing |
| Source hosting | Forgejo or Gitea | Private Git repositories |
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
