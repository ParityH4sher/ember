---
sidebar_label: Documentation workflow
sidebar_position: 5
---

# Documentation workflow

Keep project documentation and planning in `docs/`. Docusaurus reads this
directory directly; new Markdown pages appear in the sidebar automatically.

## Podman devcontainer

Requires rootless Podman and the Dev Container CLI. From the repository root:

```sh
devcontainer up --docker-path podman --workspace-folder .
```

The first container creation installs dependencies from the lockfile. When the
workspace opens in VS Code, the **Ember docs: start** task launches Docusaurus
in its own visible terminal; stop it with `Ctrl+C` or the terminal stop button.
Open <http://localhost:3000> after the initial compilation. To start it manually,
run `npm --prefix website start` in the devcontainer terminal.

Use the container ID returned by the command:

```sh
podman logs -f <container-id>
podman stop <container-id>
podman start <container-id>
```

For VS Code, install the Dev Containers extension, set
`"dev.containers.dockerPath": "podman"` in **User Settings (JSON)**, then run
**Dev Containers: Reopen in Container**.

The container requests Node.js 24 and Go 1.27.4 and includes distribution
Ansible, Python, Make, GitHub CLI, and Claude Code. Use the
[qualified Ansible controller](operations/README.md#ansible-host-provisioning)
for provisioning; the devcontainer's distribution Ansible may be older.
It maps the host user to the container's `node` user with Podman's `keep-id`
namespace. The workspace mount supports shared SELinux labeling. Port 3000 is
published on host loopback and configured for editor forwarding. The notes
client uses its own loopback port, 5173. Run the VS Code task **Ember web:
start** to start Vite and its Go API, then open `http://localhost:5173/`.
Container creation installs the web toolchain from `apps/frontends/web/package-lock.json`
and runs `svelte-check` and the worker type check. The web and docs dependencies
live in container volumes separate from host `node_modules`.

VS Code installs extensions for Claude Code, Svelte, GitHub Actions and pull
requests, Go, Docker file editing, Ansible, YAML, OpenAPI, Markdown
linting/preview, and EditorConfig.
The Docker extension provides Compose editing support; the container does not
mount a Docker/Podman socket or run privileged containers. Run Compose against
your host runtime separately when needed.

Claude Code (the `claude` CLI) is a development aid in this container only.
Ember does not depend on it, and you can use another agent instead. Run `claude`
in a container terminal and sign in with your own account or API key. The
`ember-claude-config-<id>` volume holds its settings and sign-in
(`CLAUDE_CONFIG_DIR=/home/node/.claude`), so they survive a rebuild. Remove
that volume to discard them. `CLAUDE_CODE_VERSION` in
`.devcontainer/Containerfile` pins the version a build installs. Claude Code
then updates itself. To build without it, set that build arg to an empty
string, for example with `"build": {"args": {"CLAUDE_CODE_VERSION": ""}}` in
`devcontainer.json`.

Edit files under `docs/`; the browser updates automatically. Polling handles
bind-mounted file changes. Rebuild the container after changing its configuration
or Containerfile. To reinstall after dependency changes, run
`npm --prefix website ci`.

### Troubleshooting stale Docusaurus paths

If compilation reports missing modules under a different workspace path—for
example, generated files refer to `/home/.../website/node_modules` while the
container is running under `/workspaces/ember`—Docusaurus has stale generated
`.docusaurus` state from another environment. Stop the docs server, clear the
generated cache, reinstall dependencies in the environment where the server
runs, and start it again:

```sh
cd /workspaces/ember
npm --prefix website run clear
npm --prefix website ci
npm --prefix website start
```

Run these commands inside the devcontainer. For local development outside the
container, use the equivalent commands from the repository root in that local
environment. Do not share `node_modules` between host and container; native
modules and generated absolute paths can be environment-specific.

## Local development

With Node.js 24 or newer, run these commands from the repository root:

```sh
npm --prefix website ci
npm --prefix website start
```

Open <http://localhost:3000>. Stop the server with `Ctrl+C`.

## Writing

- Use `.md` for Markdown and `.mdx` only when React components are needed.
- Keep statements concise; distinguish proposals, decisions, and implemented behavior.
- Link to other pages using relative `.md` paths so links work on GitHub and the site.
- Use fenced `mermaid` blocks for diagrams, including the C4 architecture view.
- Set `sidebar_label` and `sidebar_position` in front matter when needed.
- Group topics in directories; use `_category_.json` to set category labels and order.
- Record technology decisions under `docs/decisions/`.

## Build and preview

```sh
npm --prefix website run build
npm --prefix website run serve
```

Stop the development server before serving the build; both use port 3000.
The build validates internal links and writes a static site to `website/build/`.

For deployment, set `DOCS_URL` to the site's origin and `DOCS_BASE_URL` to its
path prefix when building. Defaults are `http://localhost:3000` and `/`.
Upload the contents of `website/build/` to a static web host.

## GitHub Pages

The workflow in `.github/workflows/docs.yml` builds and deploys the site.
The default URL after deployment is <https://parityh4sher.github.io/ember/>.

1. In the repository's **Settings → Pages → Build and deployment**, select
   **GitHub Actions** as the source.
2. Commit and push the documentation, website files, lockfile, and workflow to `main`.
3. Check **Actions → Deploy documentation** for the deployment result.

Changes to `docs/`, `contracts/`, `website/`, `.nvmrc`, or the workflow trigger deployment on
`main`. Pull requests touching those inputs build the site and check that the
published contracts match their sources, without uploading or deploying the
site. Use **Run workflow** on `main` to deploy manually.

The workflow reads the Pages origin and path, including the `/ember/` prefix,
and uses the built-in `GITHUB_TOKEN`. No deployment key is required.
After changing the Pages domain, rerun the workflow to rebuild links and assets.
