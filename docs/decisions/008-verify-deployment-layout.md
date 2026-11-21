# ADR 008: Test the source bundle's effective Compose paths

Status: Accepted  
Date: 2026-11-21  
Scope: reversible verification practice for the existing deployment contract.

**Problem and evidence.** Both apps build locally on the target host. The compose file lives in `deploy/` and the app directories are siblings of `deploy/`. Compose parsing succeeds even when the resolved contexts contain no Dockerfiles. At the baseline both contexts were absent under the documented command.

**Alternatives.** A YAML syntax test is cheap but cannot detect wrong effective paths. Building/running images on every platform also tests the proxy and images, but depends on registries, an engine and representative targets. Rendering the effective model in a fresh extracted bundle and asserting both Dockerfiles exist is cheap, offline and tests the actual directory contract. A source-tree-only test misses packaging errors.

**Decision.** Use the extracted-bundle path check as a fast regression gate; supplement it with an isolated runtime smoke test on qualified runners. Keep the operator-facing invocation consistent between bundle, source and Ansible. Check exposure and version metadata independently; the path assertion does not establish isolation, ARM performance or provenance.

**Cost and consequences.** No new runtime dependency; the test requires the already named Compose provider. Verification scales per packaging layout rather than per application endpoint. Preserve the ability to run Podman Compose; document which provider and version actually passed. Revisit if Ember starts shipping prebuilt images. This is a decision about verification, not a claim that CI implements it yet.

## Implementation choices to review at M1b

| Path convention | CLI / installer | Ansible | Trade-off |
| --- | --- | --- | --- |
| Paths relative to **manifest directory** | `-f deploy/compose.yaml` works without an extra flag when the contexts are `../apps/...` | Use the deployment's `deploy/` directory as `project_src` and its `compose.yaml` as the file | Preserves the documented simple CLI. Also review `.env` discovery and implicit project naming; test explicit port overrides. |
| Paths relative to **bundle root** | Keep `./apps/...`; every build invocation needs `--project-directory` pointing at the bundle root | Existing `project_src` already selects that root | Small installer fix, but ordinary `-f` builds still fail silently at config time and break later at build time. |
| Move the manifest to the bundle root | Simple default discovery | Change role copy destination and file argument | Good for a new layout, but breaks scripts using the existing published `deploy/compose.yaml` path. |

Prefer the first convention for this repository. Test both entry points; do not combine `../apps/...` with the old explicit bundle-root override. The project name is already explicit in the manifest; still verify overrides for two instances. Whichever convention is chosen, generate the checksum with the **archive basename** and verify it before extracting from a fresh directory.
