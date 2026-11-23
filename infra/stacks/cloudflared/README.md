# cloudflared stack

[cloudflared](https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/)
connects this host to a remotely managed **Cloudflare Tunnel**. You choose the
public subdomains in the Cloudflare dashboard, and every one points at
[Traefik](../traefik/README.md) (`http://traefik:8081`), which decides what is
actually routed. The host makes only outbound connections and opens no inbound
ports.

`cloudflared` uses two networks:

- its own project network, for the outbound connection to Cloudflare;
- the internal `tunnel` network, where its only peer is Traefik.

It cannot reach Ember, Authentik, or any other service directly.

> **This publishes services on the public internet.** Before you enable it,
> `stacks.yml` checks that Ember sign-in is on (when Ember is routed) and that
> Authentik's administrator account has been claimed. Read
> [Security](#security) before adding hostnames.

| Item | Value |
| --- | --- |
| Version | 2026.11.1, pinned by digest for amd64 and arm64 |
| Secret | `secrets/cloudflare_tunnel_token`, from `CLOUDFLARE_TUNNEL_TOKEN` in the repository's gitignored `.env` |
| Health | `cloudflared tunnel ready` against the container-local metrics endpoint `127.0.0.1:20241` |

It runs as UID 65532 with a read-only root filesystem, no Linux capabilities,
automatic updates disabled, and Cloudflare's remote diagnostics (pprof)
switched off.

## Setup

1. **Zone.** Your domain must be a zone in Cloudflare. Use the zone itself as
   `ingress_domain` (for example `example.com`). Cloudflare's free Universal SSL
   certificate covers `ember.example.com` but not `ember.home.example.com`.
2. **Tunnel.** In *Cloudflare Zero Trust → Networks → Tunnels*, create a
   *Cloudflared* tunnel. Copy its token: the long `eyJ…` value in the install
   command. Put it in the repository's gitignored `.env` (see `.env.example`):

   ```sh
   CLOUDFLARE_TUNNEL_TOKEN=eyJ...
   ```

   The dashboard's `docker run … tunnel --no-autoupdate run --token …` command
   is what this stack's `compose.yaml` runs. Do not run it separately as well:
   a second connector would also receive your traffic.
3. **Public hostnames.** In the tunnel's *Public Hostnames*, add each host with
   service type **HTTP** and URL **`traefik:8081`**. Use plain HTTP: TLS ends at
   Cloudflare, and the hop to Traefik never leaves the internal `tunnel`
   network. With the homelab inventory, the hosts are:

   | Hostname | Purpose | Inventory variable |
   | --- | --- | --- |
   | `ember-poc.<domain>` | Ember (home page, notes, API) | `ember_public_host` |
   | `auth.<domain>` | Authentik sign-in | `authentik_public_host` |

   **Wildcard (optional).** Add `*.<domain>` → HTTP `traefik:8081` as well. A
   container started with Traefik labels for `something.<domain>` then works
   without another dashboard change; unknown names get Traefik's 404.
   Cloudflare's free certificate covers `*.<domain>`, one level deep.

   Cloudflare creates the proxied DNS records. If you created "DNS only" A
   records for these names earlier, delete them.
4. **Inventory.** Set `ingress_domain`, and the host variables above if you
   want other names. To publish another service, give its container Traefik
   labels and put it on `edge` (see the
   [Traefik stack](../traefik/README.md#routing-with-labels)). Without the
   wildcard, also add its hostname here.
5. **Roll out.**
   1. Run `playbooks/stacks.yml`. On a fresh Authentik it stops before
      publishing anything.
   2. Claim the admin account over SSH, as the message explains, and run
      `stacks.yml` again. It then deploys Traefik and `cloudflared`.
   3. Run `playbooks/site.yml` to configure Ember's sign-in.

To stop publishing, stop the tunnel:
`sudo docker compose --project-directory /opt/stacks/cloudflared down`.
The stack role never stops stacks. You can also disable the tunnel in the
Cloudflare dashboard.

## Security

- **Everything routed is reachable by anyone on the internet.** Ember enforces
  its own sign-in, and Authentik is the login page. Any other container you
  label must protect itself.
  With the wildcard hostname, starting a labelled container is enough to
  publish it.
- **Protect the admin account.** Enable MFA for Authentik administrators. Its
  login page is now a public target.
- **Cloudflare Access (optional).** You can put Access in front of
  `ember-poc.<domain>`, but **not** in front of `auth.<domain>`. The Ember API calls
  Authentik's discovery, token, and JWKS endpoints server-to-server, and Access
  would block those calls. If you protect `auth.<domain>` anyway, add a *Bypass*
  policy for `/application/o/`.
- **Trust in Cloudflare.** The tunnel is remotely managed: whoever controls
  the Cloudflare account controls which hostnames reach the tunnel. Traefik
  still decides what those hostnames reach: opted-in containers on `edge`,
  nothing else. Treat the tunnel token like a
  password; rotate it in the dashboard if it leaks, then rerun `stacks.yml`.
- **The tailnet path for Ember goes away.** With a domain set, Ember's public
  URL is the only place sign-in works, so Tailscale Serve no longer publishes
  Ember. SSH over the tailnet is unaffected.
