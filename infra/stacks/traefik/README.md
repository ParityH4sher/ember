# Traefik stack

[Traefik](https://traefik.io/traefik/) routes the public hostnames that arrive
through the [Cloudflare Tunnel](../cloudflared/README.md) to containers on the
shared internal `edge` network. It is deployed, together with `cloudflared`,
only when the inventory sets `ingress_domain`.

```text
visitor ─HTTPS─▶ Cloudflare ══tunnel══▶ cloudflared ─[tunnel net]─▶ traefik:8081 ─[edge net]─▶ ember-gateway:8080
                 (public TLS)                (outbound only)          (routing)              authentik:9000
                                                                          │                    <your container>
                                                              [docker-api net] socket-proxy (read-only)
```

| Network | Members | Purpose |
| --- | --- | --- |
| `tunnel` (internal, shared) | cloudflared, Traefik (`traefik`) | cloudflared can reach only Traefik |
| `edge` (internal, shared) | Traefik, and the front container of every routed service | Traefik reaches services by alias |
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

## Two ways to route

**1. Curated routes (file provider).** `ingress_routes` in the inventory is
rendered into `dynamic/routes.yml`. Ember and Authentik are routed this way.
These routes have priority 10000, so a container label cannot take over their
hostnames. `stacks.yml` refuses to publish Ember unless sign-in is on.

**2. Container labels (Docker provider).** A container is routed when it is on
`edge` **and** opts in. For example, in its Compose file:

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

- the file route works and wins over a label claiming the same host;
- a labelled container is routed and receives the HTTPS header;
- labels without `traefik.enable` get 404;
- the proxy answers 403 to container creation, images, and secrets;
- `tunnel` members cannot reach `edge` services directly.

## Routes without Ansible

Write `dynamic/routes.yml` by hand. It must be readable by UID 65532: use a
0755 directory and a 0644 file. The `public-https` middleware must be defined
here, because the entry point refers to it.

```yaml
http:
  middlewares:
    public-https:
      headers:
        customRequestHeaders:
          X-Forwarded-Proto: https
  routers:
    authentik:
      rule: Host(`auth.example.com`)
      entryPoints: [tunnel]
      priority: 10000
      service: authentik
  services:
    authentik:
      loadBalancer:
        servers:
          - url: http://authentik:9000
```
