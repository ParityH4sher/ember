# Ember frontends

Ember has one browser frontend: `web/`, an installable SvelteKit + Vite app
written in TypeScript. It owns the whole site behind the gateway:

| Path | Page | Source |
| --- | --- | --- |
| `/login/` | Sign-in: one **Sign in with Authentik** button | `web/src/routes/login/+page.svelte` |
| `/` | Dashboard: API status, counts, recent projects, shared services | `web/src/routes/+page.svelte` |
| `/desktop/` | Desktop surface with the launcher circle; where the installed app starts | `web/src/routes/desktop/+page.svelte` |
| `/apps/notes/` | Project notes: linked Markdown notes and chats per project | `web/src/routes/apps/notes/+page.svelte` |
| `/plugins/` | Plugins: run operations; admins configure and activate | `web/src/routes/plugins/+page.svelte` |
| `/settings/` | Display preferences (theme, density) for this browser | `web/src/routes/settings/+page.svelte` |
| `/account/` | Profile and account: identity from the ID token, session, sign out | `web/src/routes/account/+page.svelte` |
| `/admin/` | Admin only: instance status, plugin status, agents, access rules | `web/src/routes/admin/+page.svelte` |

Every page except `/login/` renders inside the app shell (sidebar, top bar,
user menu). When sign-in is enabled and nobody is signed in, the root layout
sends the visitor to `/login/?returnTo=<page>`. The button names the provider
the API reports (`provider.name` in `GET /v1/auth/session`, "Authentik" in the
Ansible deployment). If the session ends while a page is open (any `401`), the
page stays as it is and the shell shows **Sign in again**, so an unsaved note
can still be copied. Unknown paths get the app's 404 page: the web container's
`nginx.conf` serves the `404.html` fallback with status 404. The design and its limits are
in the [web app shell plan](../../docs/planning/web-app-shell.md).

The gateway routes `/api/` to the Ember API and everything else to the `web`
service. From the Ember root, run
`docker compose -f deploy/compose.yaml up --build -d` and visit
`http://127.0.0.1:3000/`. To install the app, use the browser's install action
(Add to Home Screen on iOS); it opens on the desktop surface (`/desktop/`, the
manifest's `start_url`), and the Dashboard is an app in the launcher. Installation and
service workers require HTTPS or localhost/loopback.

Shared code lives in `web/src/lib/`:

- `api.ts`: every API call.
- `session.svelte.ts`: the sign-in state, sign-in and sign-out, and the leave
  guard that pages with unsaved work register.
- `prefs.svelte.ts`: display preferences in `localStorage`; `app.html` applies
  them before first paint.
- `load.ts`: the loading/value/error state pages use for API data.
- `shell/`: `AppShell.svelte` and `UserMenu.svelte`.
- `ui/`: `Card`, `PageHeader`, `Stat`, `Badge` and `Icon`.
- `PluginCard.svelte`, `ActionForm.svelte` and `SchemaFields.svelte` with
  `schema.ts`, which build plugin forms from the JSON Schemas the API
  publishes, and `ServiceLinks.svelte`.
- `widgets/`: the widget contract, `widgetRegistry` and the `WidgetBoard`
  the dashboard hosts. See [widgets](../../docs/widgets.md).
- `apps/`: the app registry (`registerApp`), launch targets and the static
  app list. `launcher/` is the app launcher widget. See
  [app launcher](../../docs/app-launcher.md).
- `setup.ts`: registers the built-in widgets and apps (the composition root).

Colours are tokens in `app.css` with a dark and a light set; use the tokens,
not literal colours. The browser loads no plugin code or remote HTML. Sign-in
and sign-out are full-page navigations through the API; the page never sees a
token. Hiding admin controls is a convenience; the API enforces roles.

## Tests

| Suite | Command | Needs | Covers |
| --- | --- | --- | --- |
| Unit (`web/tests/unit`) | `npm run test:unit` | Node | Plain TypeScript modules without a browser: the widget and app registries and their validation, launcher search and ordering, pinned/recent state, keyboard navigation and shortcuts. Runs in CI (`web` job). |
| UI (`web/tests/ui`) | `npm run test:ui` | Node, Playwright's Chromium | The built app on `vite preview` with a mocked API: sign-in page and `returnTo`, roles, user menu, every page, session expiry, theme, 404, phone layout, the app launcher widget and circle (search, keyboard, the launcher shortcut, pinning, cross-window sync, runtime apps, configure/remove/add), the manifest's `start_url`, and axe WCAG 2.1 AA checks on every page in both themes. Runs in CI (`web` job). |
| Real infrastructure (`web/tests/real`) | `npm run test:real` or `web/scripts/test-real.sh` | A deployed Ember with Authentik and test accounts | Gateway, API, web container, Redis session store and sign-in through the real Authentik. See [its README](web/tests/real/README.md). |

The devcontainer image includes Playwright's Chromium and its system libraries
(and Google Chrome on amd64, for the Playwright and Chrome DevTools MCP
tools), so both suites run there. Keep `PLAYWRIGHT_VERSION` in
`.devcontainer/Containerfile` and the image in `web/scripts/test-real.sh`
equal to `@playwright/test` in `web/package.json`.

**Devcontainer:** rebuild and reopen the `ember/` workspace, then run **Tasks:
Run Task → Ember web: start** in VS Code. It launches Vite and the Go API
in their own terminals. Open `http://localhost:5173/`. Use **Ember
web: type-check** to check the client and service worker. Container setup
installs the locked dependencies, generates the SvelteKit types and configures
the Svelte extension and workspace TypeScript SDK.

For web development, run `npm ci` and `npm run dev` inside `web/` with Node
24+. Open any page on the Vite server; `/api/` proxies to a local
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
frontend. To add a page, add a route under `web/src/routes/` and an entry in
`lib/shell/AppShell.svelte`. Add its path to the slash redirect in
`deploy/gateway/nginx.conf` so that the link without a trailing slash works.
