---
sidebar_label: Plugins
sidebar_position: 4
---

# Plugins: wishlist and example

This page holds the list of plugins wanted for Ember and the Weather plugin,
which is the worked example for building them. Add, remove, or reorder rows as
priorities change. The list records intent. A row is not implemented until its
status says so.

An Ember plugin is a compile-time adapter in the Go API (`apps/api/internal/plugins/`).
It connects Ember to an independently operated service. Every plugin is
discovered, configured, activated, and invoked through Ember's own
`/api/v1/plugins` routes. The browser never calls the upstream service directly.
For the design rules (shared vs. configurable services, versioning, and
authorization), see [shared services](architecture/shared-services.md) and
[modularity](architecture/modularity.md).

## Wanted plugins

Open **Plugins** on Ember Home (`/`) to see the adapters included by the API.
Expand **Settings** to configure one, select **Activate**, and open **Operations**
to run an advertised operation. An adapter with no operations still has a settings
card. Adding a registered adapter that uses the supported schema fields does
not require a new frontend component.

**Shared services** on the same page is a directory of browser links. To show
Gitea and Authentik, set the following on the Compose host, replacing the URLs
with your own, then recreate the API:

```sh
export EMBER_SERVICE_LINKS='[{"name":"Gitea","url":"https://git.example.org/"},{"name":"Authentik","url":"https://id.example.org/"}]'
# Optional: include a read-only Gitea adapter for one public repository.
export EMBER_GITEA_REPOSITORY_URL='https://git.example.org/team/widget'
docker compose -f deploy/compose.yaml up -d --build
```

The directory accepts additional named HTTP(S) links and is empty by default.
Use browser-facing URLs without secrets. Adding a link opens the external app;
it grants no repository access and does not configure identity. For actual
Authentik sign-in, use the [OIDC configuration](authentication.md).

| Plugin | Upstream service | What Ember would use it for | Status |
| --- | --- | --- | --- |
| Weather | [Open-Meteo](https://open-meteo.com/) | Current conditions by coordinates | **Implemented example** (see below) |
| Immich | [Immich](https://immich.app/) | Browse and attach photos/videos from a self-hosted library | Implemented read-only: **Server info** (Immich version and the user that owns the API key) and **List albums** (albums that user owns or that are shared with it; metadata only, at most 200). Use a dedicated non-admin Immich account. The key is sent only as `x-api-key`, redirects are not followed, and it must be re-entered after an API restart. |
| Gitea | [Gitea](https://about.gitea.com/) | Repository name, description and default branch | Implemented read-only for one operator-selected public repository. Activate, then run **Repository details**. No credentials, write access or upstream lifecycle controls. |
| Paperless-ngx | [Paperless-ngx](https://docs.paperless-ngx.com/) | Search and reference stored documents | Wanted |
| Mealie | [Mealie](https://mealie.io/) | Recipes and meal planning | Wanted |
| Home Assistant | [Home Assistant](https://www.home-assistant.io/) | Workshop sensors, lights, and power state, read-only at first | Wanted |
| 3D printer | [OctoPrint](https://octoprint.org/) or [Moonraker](https://moonraker.readthedocs.io/) | Printer status and job progress. Starting a print requires explicit approval | Wanted |

The **Wanted** rows were collected from candidates already named in
[shared services](architecture/shared-services.md) and the
[inspiration list](inspiration.md). Keep only the ones you actually want.

Tools that *execute* work, such as CAD applications, Blender, or coding agents,
are not plugins of this kind. They belong behind a sandboxed worker boundary.
See [sandboxing](sandboxing/README.md) and `services/`.

## Example plugin: Weather

Weather is the reference implementation. Copy its structure when adding a
plugin from the list above.

| Piece | Location |
| --- | --- |
| Definition and adapter | `apps/api/internal/plugins/weather/weather.go` |
| Registration | `NewHandler` in `apps/api/internal/httpapi/server.go` |
| Tests against a local upstream fixture | `apps/api/internal/httpapi/weather_test.go` |
| Generic plugin routes | `contracts/openapi.yaml` (`/v1/plugins/...`) |
| Browser UI | The status page at `/` (`apps/frontends/home/integrations.js`) renders configuration, activation, and operation forms from the plugin's schemas |

What it does:

- Declares plugin `version` `1.0.0` and frontend-facing `apiVersion` `1`, with a
  single operation, `current`.
- Needs no configuration and no API key, so it counts as configured immediately.
  It still has to be **activated** before it can be invoked.
- Sends a fixed `GET` to Open-Meteo's forecast endpoint. The client cannot supply
  a URL, host, path, or method.
- Validates the coordinates before calling upstream. It uses an 8-second timeout,
  refuses redirects, caps the upstream response at 1 MiB, and maps the reply to
  a stable output shape that includes source and attribution fields.
- Returns a generic `502 upstream_error` when the upstream call fails, without
  passing upstream details through.

Open-Meteo's free API is for non-commercial use and requires
[CC BY 4.0](https://creativecommons.org/licenses/by/4.0/) attribution. Activation state is
held in memory, so reactivate the plugin after the API restarts.

Try it against a running stack (default port 3000):

```sh
curl -X PUT http://127.0.0.1:3000/api/v1/plugins/weather/activation \
  -H 'Content-Type: application/json' -d '{"active":true}'

curl http://127.0.0.1:3000/api/v1/plugins/weather/api   # operation catalog

curl -X POST http://127.0.0.1:3000/api/v1/plugins/weather/api/v1/current \
  -H 'Content-Type: application/json' \
  -d '{"body":{"latitude":47.6,"longitude":-122.3}}'
```

## Adding a plugin from the list

1. Create `apps/api/internal/plugins/<id>/` with a `Definition()` and an adapter
   that implements `plugins.Adapter`. Provide a `NewWithClient(endpoint, client)`
   constructor so tests can point the adapter at an `httptest` server.
2. Describe configuration and operation inputs with JSON Schema. The API's
   validator (`internal/httpapi/schema.go`) supports only `type`, `properties`,
   `required`, `additionalProperties: false`, nested objects, `minimum`/`maximum`,
   `enum`, and `format: uri`. Mark credentials `writeOnly` so that Ember never
   returns them.
3. Map each `operationId` to a fixed upstream method and path. Bound timeouts and
   response sizes, refuse redirects to other origins, and return sanitized
   errors.
4. Register the plugin in `NewHandler`. Test it with `NewHandlerWithPlugins` and a
   local fixture rather than the real service.
5. Bump `version` for every release. Bump `apiVersion` only for breaking changes to
   the operation contract.
6. Update the status column above.

Plugin metadata describes an interface. It does not grant authority. Plugin
configuration is shared by the whole instance. With
[sign-in](authentication.md), only people with the
[`admin` role](authentication.md#roles) can change it or activate a plugin, and
every signed-in user can call the operations of an active plugin. Without
sign-in, anyone who can reach the API can do both. Do not configure real
credentials on an instance reachable from an untrusted network.
