# Ember frontends

`web/` is the installable SvelteKit + Vite Project Notes client, written in TypeScript. `desktop/`
is reserved for a native client. `home/` is the standalone static status page.
The shared gateway routes `/` to `home`, `/apps/notes/` to `notes`, and `/api/` to Ember. From the Ember root,
run `docker compose -f deploy/compose.yaml up --build -d`, visit
`http://127.0.0.1:3000/apps/notes/`, and use the browser's install action
(Add to Home Screen on iOS). Installation and service workers require HTTPS
or localhost/loopback.

**Devcontainer:** rebuild and reopen the `ember/` workspace, then run **Tasks:
Run Task → Ember web: start** in VS Code. It launches Vite and the Go API
in their own terminals. Open `http://localhost:5173/apps/notes/`. Use **Ember
web: type-check** to check the client and service worker. Container setup
installs the locked dependencies, generates the SvelteKit types and configures
the Svelte extension and workspace TypeScript SDK.

For web development, run `npm ci` and `npm run dev` inside `web/` with Node
24+. Open `/apps/notes/` on the Vite server; `/api/` proxies to a local Ember
API at `127.0.0.1:8080`. Run `npm run check` to type-check the Svelte client
and its service worker. `npm run build` uses the static adapter to write
`web/build/`. Compose builds these assets in a Node stage and serves them with
Nginx; Node is not required on the runtime host.

In Chrome and Edge, the installed app can open a blank workspace window
(`/apps/notes/display/`) on every other display, and **Pin to top** floats a
note in a Picture-in-Picture window. See `docs/multi-display.md` and
`docs/always-on-top.md`.

After the first online visit, the worker caches the static app shell. Project
lists, note bodies and saves go directly to the API. While disconnected, an
already open project remains browsable in that window; keep the editor open
and retry Save after reconnecting. Reloading offline opens only the shell.

Each web frontend builds from its own directory and runs in its own container.
To run just the status page and API through the private gateway, from the Ember root:

```sh
docker compose -f deploy/compose.yaml up -d --build frontend api home
```

Use `notes` instead of `home` for a Notes-only setup. With the gateway and API
already running, `docker compose -f deploy/compose.yaml up -d --build notes`
updates only Notes. To add another frontend, give it a build context, a Compose
service, and a path in `deploy/gateway/nginx.conf`.
