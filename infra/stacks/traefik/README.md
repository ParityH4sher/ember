# Traefik stack

[Traefik](https://traefik.io/traefik/) routes the public hostnames that arrive
through the [Cloudflare Tunnel](../cloudflared/README.md) to containers on the
shared internal `edge` network. It is deployed, together with `cloudflared`,
only when the inventory sets `ingress_domain`.

```text
visitor ─HTTPS─▶ Cloudflare ══tunnel══▶ cloudflared ─[tunnel net]─▶ traefik:8081 ─[edge net]─▶ Ember's gateway
                 (public TLS)                (outbound only)          (routing)              Authentik's server
                                                                          │                    <your container>
                                                              [docker-api net] socket-proxy (read-only)
```

| Network | Members | Purpose |
| --- | --- | --- |
| `tunnel` (internal, shared) | cloudflared, Traefik (`traefik`) | cloudflared can reach only Traefik |
| `edge` (internal, shared) | Traefik, and the front container of every routed service | Traefik reaches labelled containers at their `edge` address |
| `docker-api` (internal, this stack) | Traefik, socket-proxy | Label discovery |

Internal networks have no internet access. Databases and other internal
containers never join `edge`. Ansible creates `tunnel` and `edge`
(`roles/ingress_networks`). By hand, run `docker network create --internal`
once for each.

| Item | Value |
| --- | --- |
| Version | Traefik v3.7.19 and docker-socket-proxy v0.5.0, both pinned by digest for amd64 and arm64 |
| Entry point | `tunnel` on port 8081, reachable only on the internal networks; no published ports |
| Secrets | none |

## Routing with labels

Every route comes from container labels (ADR 012). A container is routed when
it is on `edge` **and** opts in with `traefik.enable=true` and a `Host()` rule.
Ember and Authentik label themselves:

| Service | Labels in | Host from |
| --- | --- | --- |
| Ember's gateway | `deploy/compose.edge.yaml`, added by `site.yml`, which refuses unless Ember sign-in is on | `EMBER_PUBLIC_HOST` (`ember_public_host`) |
| Authentik's server | `infra/stacks/authentik/compose.yaml`, active while `PUBLIC_ROUTE` is `true` | `PUBLIC_HOST` in its `env` (`authentik_public_host`) |

Your own container, in its Compose file:

```yaml
services:
  app:
    image: ...
    networks: [edge]
    labels:
      traefik.enable: "true"
      traefik.http.routers.app.rule: Host(`app.parityh4sher.org`)
      traefik.http.services.app.loadbalancer.server.port: "8080"

networks:
  edge:
    external: true
```

It becomes reachable as soon as it starts, with no Traefik restart, provided
Cloudflare sends the hostname to the tunnel (see
[wildcard hostnames](../cloudflared/README.md#setup)). Containers without
`traefik.enable=true` are never routed (`exposedByDefault=false`). Routers
attach to the `tunnel` entry point by default.

**A labelled container is on the public internet.** It must enforce its own
authentication. Review labels like any other change that publishes something.
Router names must be unique on the host: `ember` and `authentik` are taken.

**Nothing reserves a hostname.** Any container on `edge` can label itself with
``Host(`auth.<domain>`)`` too, and Traefik then has two routers for one host.
Only start containers on `edge` that you trust as much as Ember and Authentik.

`dynamic/` holds no routes. It contains only the `public-https` middleware
(`templates/dynamic/middlewares.yml.j2`), which the entry point applies to
every router.

## Security

- **No Docker socket in Traefik.** `socket-proxy` holds the socket and
  forwards only read-only container listing, inspection, events, version, and
  ping. Every `POST` (create, start, exec) and every other endpoint (images,
  secrets, networks) returns 403. Only Traefik shares its network.
- **Read access is still broad.** Through the proxy, Traefik can read every
  container's configuration on the host, including environment variables.
  Keep secrets in files, as the stacks here do, not in container environments.
- **HTTPS-aware applications.** Cloudflare terminates TLS, so the `tunnel` entry
  point sets `X-Forwarded-Proto: https` on every request (middleware
  `public-https@file`), whether the router comes from a file or from labels.
  That is why Authentik advertises its issuer as `https://auth.<domain>/...`.
  The visitor address that cloudflared forwards is kept.
- **No dashboard, API, or access log.** Request URLs can contain personal data.
- **Minimal privileges.** Traefik runs as UID 65532, both containers have a
  read-only root filesystem, no Linux capabilities, and `no-new-privileges`.

Verified locally:

- Ember and Authentik are routed by their labels alone (2026-11-23): Authentik
  advertises an `https://` issuer, and Ember serves its pages, health and
  session through Traefik;
- a labelled container is routed and receives the HTTPS header;
- labels without `traefik.enable` get 404;
- the proxy answers 403 to container creation, images, and secrets;
- `tunnel` members cannot reach `edge` services directly.

## Without Ansible

Traefik needs `dynamic/middlewares.yml`, because the entry point refers to the
`public-https` middleware. It must be readable by UID 65532: use a 0755
directory and a 0644 file.

```yaml
http:
  middlewares:
    public-https:
      headers:
        customRequestHeaders:
          X-Forwarded-Proto: https
```

Routes still come from labels. For Authentik, set `PUBLIC_ROUTE=true` and
`PUBLIC_HOST=auth.example.com` in its `.env`.

Hosts upgraded from the curated-route version have a `dynamic/routes.yml`;
`stacks.yml` deletes it. Without Ansible, delete it yourself.
