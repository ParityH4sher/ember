# ADR 011: Deploy prebuilt multi-arch images from the GitHub Container Registry

Status: Accepted (published from `main` since 755b647; a pull on the Pi is not yet verified)  
Date: 2026-11-22  
Scope: how the Ansible `ember` role obtains Ember's four images. The source
bundle and `deploy/install.sh` still build on the host.

**Problem.** The Ansible role copied the source to the Pi and built all four
images there. Every deploy ran `npm ci`, the Svelte check and the Go build on
the target, needed npm, the Go proxy and Docker Hub at deploy time, and could
produce different images from the same commit on different hosts. ADR 008 asked
to revisit its verification practice if Ember started shipping prebuilt images.

**Decision.**

- The `images` job in `.github/workflows/build.yml` builds each Compose service
  (`api`, `home`, `notes`, `gateway`) for `linux/amd64` and `linux/arm64`. It
  runs only for `main`, after the tests and the bundle smoke test pass, and
  pushes `ghcr.io/<owner>/ember-<service>:sha-<commit>`. The API image reports
  that commit as its version.
- The `images-latest` job moves `latest` to that commit once all four images
  exist, and only while the commit is still the head of `main`.
- Build stages run on the build platform (`FROM --platform=$BUILDPLATFORM`):
  Go cross-compiles, and the static notes build is architecture-independent.
  No step runs under emulation.
- `deploy/compose.registry.yaml` replaces each `build:` with an `image:`. All
  runtime settings still come from `deploy/compose.yaml`.
- The role's `ember_image_source: registry` installs only the Compose
  manifests and pulls `ember_image_tag` on every run. `build` stays the role
  default. The homelab inventory uses `registry` with `latest`.

**Consequences.**

- Each `site.yml` run deploys whatever `latest` names at that moment, so every
  commit on `main` that passes CI reaches the Pi on its next run. Pin
  `ember_image_tag: sha-<commit>` to hold or roll back a version. `/healthz`
  reports the running commit.
- Images are pulled by tag, not digest, and nothing verifies a signature or the
  attached BuildKit provenance at pull time.
- GHCR becomes a deploy-time dependency. npm, the Go proxy and Docker Hub no
  longer are, for this host.
- `!reset` in the overlay needs Docker Compose 2.24.4 or newer; the role checks
  this.
- The packages are public
  ([DEC-008](../planning/human-decisions.md#dec-008---visibility-of-ember-container-images)),
  so hosts need no credential. If they become private, each host needs a
  `read:packages` token, which Docker stores unencrypted in root's configuration
  ([DEC-002](../planning/human-decisions.md#dec-002---identity-and-secret-custody)).
- ADR 008's bundle checks still apply to the bundle. They say nothing about the
  registry images, which share only the Dockerfiles with it.
