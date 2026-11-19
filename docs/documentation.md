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

Open <http://localhost:3000> after the initial compilation. Every container start
runs `npm ci` from the lockfile, then starts Docusaurus as its main process.
No manual `npm install` or `npm start` is needed.

Use the container ID returned by the command:

```sh
podman logs -f <container-id>
podman stop <container-id>
podman start <container-id>
```

For VS Code, install the Dev Containers extension, set
`"dev.containers.dockerPath": "podman"` in **User Settings (JSON)**, then run
**Dev Containers: Reopen in Container**.

The container provides Node.js 24 and maps the host user to the container's
`node` user with Podman's `keep-id` namespace. The workspace mount supports shared
SELinux labeling. Port 3000 is published on host loopback and configured for editor
forwarding.

Edit files under `docs/`; the browser updates automatically. Polling handles
bind-mounted file changes. Rebuild the container after changing its configuration
or Containerfile. Restart it after dependency changes to reinstall from the lockfile.

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

Changes to `docs/`, `website/`, `.nvmrc`, or the workflow trigger deployment on
`main`. Use **Run workflow** on `main` to deploy manually.

The workflow reads the Pages origin and path, including the `/ember/` prefix,
and uses the built-in `GITHUB_TOKEN`. No deployment key is required.
After changing the Pages domain, rerun the workflow to rebuild links and assets.
