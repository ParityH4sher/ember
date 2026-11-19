# Ember

Self-hosted AI design and fabrication workspace: connect design tools, workshop
machines, and physical interfaces to help turn ideas into real artifacts. See the
[project proposal](docs/IDEA.md), [inspiration list](docs/inspiration.md), and
[documentation index](docs/README.md).

## Documentation preview

With rootless Podman and the Dev Container CLI:

```sh
devcontainer up --docker-path podman --workspace-folder .
```

Open <http://localhost:3000>. The container installs dependencies and starts
Docusaurus automatically. Edits under `docs/` reload in the browser.

For VS Code setup, local commands, and authoring rules, see the
[documentation workflow](docs/documentation.md).

## GitHub Pages

The deployment workflow targets <https://parityh4sher.github.io/ember/>.
Enable **GitHub Actions** under **Settings → Pages**, then push to `main`.
See [deployment instructions](docs/documentation.md#github-pages).
