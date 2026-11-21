# Ember

Self-hosted AI design and fabrication workspace: connect design tools, workshop
machines, and physical interfaces to help turn ideas into real artifacts. This
repository is the monorepo for Ember's applications, deployment, and documentation.
See the [project proposal](docs/IDEA.md), [inspiration list](docs/inspiration.md),
and [documentation index](docs/README.md).

## Build and install

The API is an initial platform scaffold. Build it locally with Go 1.24 or download
the portable Compose bundle from the **Actions** tab. On a server with Docker
Compose v2 or Podman Compose, unpack the bundle and run from its root:

```sh
./install.sh
```

The installer builds the containers for the server's architecture and starts the
stack. See the [deployment guide](docs/deployment.md) for configuration and
limitations.

The GitHub Actions artifact is a portable deployment bundle, not a complete
Ember product or a claim of production security. Application capabilities will
be added behind the documented platform boundaries.

## Repository map

```text
apps/                 User-facing applications and the core API
services/             Proposed worker and adapter contracts (no running workers)
contracts/            API and portable project format contracts
deploy/               Compose deployment and host install tooling
infra/ansible/        Host provisioning and baseline configuration
docs/                 Architecture, decisions, and operations documentation
website/              Docusaurus documentation site
```

See [architecture](docs/architecture/README.md) for boundaries and
[operations](docs/operations/README.md) for deployment and recovery procedures.
The detailed POC handoff for an implementation agent is
[EMBER_POC_IMPLEMENTATION_BRIEF.md](EMBER_POC_IMPLEMENTATION_BRIEF.md).

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
