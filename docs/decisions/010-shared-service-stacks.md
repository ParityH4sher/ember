# ADR 010: Deploy shared services as independent Compose stacks

Status: Accepted
Date: 2026-11-21
Scope: layers 1 and 2 in [shared services](../architecture/shared-services.md),
starting with Authentik, and the public ingress (Traefik and cloudflared).

**Problem.** Shared services such as the identity provider must outlive any one
Ember installation. They may serve several instances, and under
[DEC-006](../planning/human-decisions.md#dec-006---lifecycle-authority-and-sharing)
only the operator manages their lifecycle. Adding them to `deploy/compose.yaml`
or to the Ember bundle would tie them to Ember's `up`, `down` and
`down --volumes` (which the bundle smoke test runs). Writing them as Ansible
templates would make Ansible the only way to run them.

**Decision.**

- Each shared service is one Compose project in `infra/stacks/<name>/`, named
  `stack-<name>`, runnable with plain `docker compose`.
- Images are pinned by multi-architecture digest. Secrets are files under
  `./secrets` (DEC-002). A web UI is reachable only through the internal `edge`
  network, plus an optional loopback port for administration.
- A separate playbook, `playbooks/stacks.yml`, deploys the stacks listed in the
  inventory through one generic `stack` role. It never stops a stack or removes
  its volumes.
- Which services are defaults is inventory data (`shared_default_services`),
  not code: "default" describes a deployment profile, not a vendor.
- **Ingress (operator decisions, 2026-11-21/22):** a remotely managed
  Cloudflare Tunnel. cloudflared connects outbound to Cloudflare. This is the
  "Cloudflare ingress/tunnel" profile that
  [shared services](../architecture/shared-services.md#network-and-access-direction)
  set aside. It is opt-in: without `ingress_domain`, nothing is public.
- **Networks.** Two shared Docker *internal* networks (no internet access):
  - `tunnel`: cloudflared and Traefik only;
  - `edge`: Traefik and the front container of each routed service, by alias.

  Databases never join `edge`, and Traefik has no host networking and no
  published ports.
- **Routing.** Traefik routes two kinds of host:
  - curated `ingress_routes` from a file, with priority 10000;
  - containers on `edge` that opt in with the label `traefik.enable=true`
    (`exposedByDefault=false`).

  Label discovery goes through a read-only Docker socket proxy on a network
  private to Traefik. It refuses all POSTs, images, secrets, and networks.
- **Safeguards before publishing** (`stacks.yml` refuses otherwise):
  - Ember sign-in is on whenever Ember is routed;
  - Authentik's `akadmin` account has a usable password, so the initial-setup
    flow is closed;
  - every curated route targets an `http://<alias>:<port>` URL on `edge`.

**Alternatives.**

- *Add the services to Ember's Compose file:* rejected because the lifecycles
  are coupled.
- *Ansible-templated Compose files:* rejected because they cannot run by hand
  and are harder to review.
- *Traefik's Docker provider with the socket mounted:* rejected, because it
  gives the internet-facing container root-equivalent host access. The operator
  chose label routing (2026-11-22), so it runs through a read-only socket proxy
  with opt-in labels, and curated routes keep priority.
- *File routes only:* the first tunnel version. Safer, because every public
  host is reviewed in the inventory, but every new container needs an inventory
  change. Replaced at the operator's request.
- *Host networking for Traefik and cloudflared, with services on loopback
  ports:* no shared Docker networks. It was replaced because a compromised
  ingress container could then reach everything listening on the host's
  loopback.
- *Tailscale Serve for every service:* workable, but it offers one hostname per
  node, so each service would need its own port.
- *Tailnet-only Traefik with Let's Encrypt DNS-01 and DNS records pointing at
  the tailnet address:* implemented first and verified locally, then replaced at
  the operator's request. It stays private, but it cannot serve visitors outside
  the tailnet, and it needs a DNS API token, open tailnet ports, and an extra UFW
  rule for container-to-host traffic.

**Consequences.**

- Each stack has its own backup, restore and upgrade notes, and the operator
  runs one more playbook.
- **Routed services are reachable from the internet.** Each one must enforce
  its own authentication. Authentik's login page becomes a public target, so
  administrator MFA is recommended. Cloudflare can see the traffic, and whoever
  controls the Cloudflare account controls the tunnel's hostnames.
- **Labels publish.** With a wildcard tunnel hostname, starting a container on
  `edge` with `traefik.enable=true` makes it public without any review step.
  Through the proxy, Traefik can also read every container's configuration,
  including environment variables, so secrets belong in files.
- Stacks and Ember depend on the `edge` network existing. Ansible creates it;
  without Ansible, it must be created by hand.
- The host opens no inbound ports for the ingress. The Ember API reaches
  `auth.<domain>` through Cloudflare like any other client, so no firewall
  exceptions are needed. Cloudflare Access must not block `auth.<domain>`'s OIDC
  endpoints.
- With a domain set, Ember's public URL is its only working origin for sign-in,
  so Tailscale Serve no longer publishes Ember.
- Cross-service wiring comes from shared inventory values. Ember's sign-in
  client is rendered into Authentik as a blueprint, and the `ember` role writes
  the matching `auth.json`. The client secret is generated once by the
  Authentik stack and read by the `ember` role on the same host.

**Verification (2026-11-21, AMD64 workstation, Docker 29.9.2, ansible-core 2.22.0).**

- *Stacks:* Authentik started from plain Compose; its `file://` secrets,
  no-capability containers and healthchecks all worked.
- *Idempotency:* repeated `stacks.yml` runs changed nothing and kept the
  generated secrets. A missing operator secret stopped the run with a clear
  message.
- *Ember sign-in:* the Ember blueprint applied within seconds. The token
  endpoint accepted the generated client secret and rejected others. A
  scripted end-to-end sign-in through Authentik into the Ember API succeeded,
  including group policy and sign-out. Switching sign-in off removed the
  blueprint.
- *Publishing gate:* with `akadmin` unclaimed, `stacks.yml` refused to deploy
  the ingress. An earlier HTTP-based check passed wrongly and was replaced by
  the in-Authentik check. After the password was set, the ingress deployed.
  Routing Ember without sign-in was refused.
- *Networks (2026-11-22):* both networks were created as internal, and a rerun
  changed nothing. From `tunnel`, by name through `traefik:8081`:
  - Authentik advertised `https://auth.<domain>/application/o/ember/`;
  - Ember's home page, `/api/healthz`, and `/apps/notes/` were served through
    `ember-gateway`;
  - unlisted hosts returned 404.

  Blocked, as intended:
  - `tunnel` → `authentik`/`ember-gateway` directly;
  - `edge` → Authentik's PostgreSQL and Ember's API;
  - Traefik → the internet and the host's loopback.

  Traefik had no published ports.
- *Labels (2026-11-22):*
  - an opted-in container was routed and received `X-Forwarded-Proto: https`;
  - labels without `traefik.enable` returned 404;
  - a label claiming a curated host lost to the file route;
  - the socket proxy returned 403 for container creation, images, and secrets.
- *cloudflared:* the pinned image (amd64 and arm64) runs read-only as UID 65532
  without capabilities. It read the token file and rejected a dummy token.
- *Not tested:*
  - a real Cloudflare Tunnel connection;
  - the `ember` role's sign-in tasks outside check mode (they need root);
  - the Tailscale role on a host;
  - Raspberry Pi hardware.
