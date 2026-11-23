# ADR 012: Route every public hostname through container labels

Status: Accepted  
Date: 2026-11-23  
Scope: how Traefik learns which public hostnames to route. Supersedes the
routing part of [ADR 010](010-shared-service-stacks.md). The networks, the
socket proxy and the Authentik claim check are unchanged.

**Problem.** Traefik had two route sources. Curated routes (`ingress_routes`
in the inventory) were rendered into a file with priority 10000, and covered
Ember and Authentik. Container labels covered everything else. The operator
wants one mechanism: a service is published by starting a container with
Traefik labels, not by editing the inventory.

**Decision.**

- Every route comes from container labels on the internal `edge` network
  (`traefik.enable=true`, `exposedByDefault=false`).
- Ember's gateway labels itself in `deploy/compose.edge.yaml` with
  `EMBER_PUBLIC_HOST`. Authentik's server labels itself in its stack, active
  while `PUBLIC_ROUTE` is `true`, with `PUBLIC_HOST`. The homelab inventory
  derives both from `ingress_domain`.
- The file provider stays, for the `public-https` middleware only, which the
  `tunnel` entry point applies to every router.
- Traefik keeps reading labels through the read-only socket proxy. The operator
  considered mounting the Docker socket into Traefik and kept the proxy
  (2026-11-23), because a compromised Traefik could then control Docker, which
  is root on the host.
- The "no Ember without sign-in" safeguard moves to where Ember is labelled:
  the `ember` role refuses `ember_edge_enabled` without `ember_oidc_enabled`.
  `stacks.yml` checks Authentik's `PUBLIC_HOST` and deletes the old
  `dynamic/routes.yml`.

**Consequences.**

- No hostname is reserved. A container on `edge` can label itself with Ember's
  or Authentik's host, and Traefik then has two routers for one host. Before,
  curated routes always won. Only start containers on `edge` that you trust
  like Ember and Authentik, and review labels like any change that publishes
  something.
- The inventory no longer validates route targets (`http://<alias>:<port>`).
  Traefik routes to each labelled container's address on `edge`.
- Router names must be unique per host (`ember`, `authentik`), so a second
  Ember instance on the same host needs its own router name.
- Upgrading an existing host: run `stacks.yml` (it removes the curated routes),
  then `site.yml` (it labels Ember). Ember is unreachable in between.
