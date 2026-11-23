# Ember frontends

Ember has one browser frontend: `web/`, an installable SvelteKit + Vite app
written in TypeScript. It owns the whole site behind the gateway:

| Path | Page | Source |
| --- | --- | --- |
| `/` | Home: API status, sign-in state, shared service links, plugin settings and operations | `web/src/routes/+page.svelte` |
| `/apps/notes/` | Project notes: linked Markdown notes per project | `web/src/routes/apps/notes/+page.svelte` |

The gateway routes `/api/` to the Ember API and everything else to the `web`
service. From the Ember root, run
`docker compose -f deploy/compose.yaml up --build -d` and visit
`http://127.0.0.1:3000/`. To install the app, use the browser's install action
(Add to Home Screen on iOS); it opens on Project notes. Installation and
service workers require HTTPS or localhost/loopback.

Shared code lives in `web/src/lib/`: `api.ts` (every API call), `Account.svelte`
(sign-in and sign-out), `PluginCard.svelte`, and `SchemaFields.svelte` with
`schema.ts`, which build plugin forms from the JSON Schemas the API publishes.
The browser loads no plugin code or remote HTML. Sign-in and sign-out are
full-page navigations through the API; the page never sees a token.

**Devcontainer:** rebuild and reopen the `ember/` workspace, then run **Tasks:
Run Task → Ember web: start** in VS Code. It launches Vite and the Go API
in their own terminals. Open `http://localhost:5173/`. Use **Ember
web: type-check** to check the client and service worker. Container setup
installs the locked dependencies, generates the SvelteKit types and configures
the Svelte extension and workspace TypeScript SDK.

For web development, run `npm ci` and `npm run dev` inside `web/` with Node
24+. Open `/` or `/apps/notes/` on the Vite server; `/api/` proxies to a local
Ember API at `127.0.0.1:8080`. Run `npm run check` to type-check the Svelte
client and its service worker. `npm run build` uses the static adapter to write
`web/build/`. Compose builds these assets in a Node stage and serves them with
Nginx; Node is not required on the runtime host.

After the first online visit, the service worker caches the static app shell.
Project lists, note bodies, plugins and saves go directly to the API. While
disconnected, an already open project remains browsable in that window; keep
the editor open and retry Save after reconnecting. Reloading offline opens only
the shell.

`web/static/apps/notes/service-worker.js` is not part of the app. It replaces
the worker that Project notes registered at `/apps/notes/` while it was a
separate app, so that browsers stop serving its old cached shell. Its header
says when to delete it.

With the gateway and API already running,
`docker compose -f deploy/compose.yaml up -d --build web` updates only the
frontend. To add a page, add a route under `web/src/routes/`; the gateway needs
no change.
