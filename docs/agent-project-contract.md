---
sidebar_label: Agent project contract
sidebar_position: 4
---

# Agent project definitions and artifact contract

Ember should allow Claude, OpenAI models, and local models to work on the same
project without making any one provider's prompt or session format the canonical
project format. The project definition and saved artifacts are the durable
source of truth; provider-specific instructions are generated adapters.

There is not one universal standard that makes all AI agents interoperable.
Interoperability therefore needs explicit layers:

1. **Project contract:** Ember-owned, versioned project definition and task
   instructions.
2. **Tool interoperability:** protocols such as MCP can expose tools, but do
   not define project files, artifact formats, or completion criteria.
3. **Artifact interoperability:** established formats selected for each type
   of design output, plus native editable sources where needed.
4. **Agent execution:** provider or runner adapters that translate the same
   contract into each agent's supported instructions and tools.

## What standards do—and do not—solve

MCP can standardize how a model client discovers and calls tools. It does not
standardize what a project means, where an agent should save a CAD source, how a
printer job is validated, or how one agent hands work to another. Agent
frameworks and session formats are also not a substitute for durable,
provider-neutral project data.

Prefer stable, documented, tool-supported artifact formats. For example:

| Artifact | Candidate interchange / delivery formats | Important qualification |
| --- | --- | --- |
| Mechanical 3D design | STEP (`.step`, `.stp`); STL; 3MF | STEP is for model exchange; STL/3MF are commonly used for printing. Preserve the editable CAD source and its tool/version. |
| 2D geometry | SVG; DXF | Confirm units, scale, layers, and the target machine's supported dialect. |
| PCB fabrication | Gerber and Excellon drill files | Preserve the KiCad or other native project, design-rule results, and fabrication settings. |
| Mesh / scene assets | glTF/GLB; USD where supported | Keep the native Blender or other scene source and note dependencies. |
| Documents and instructions | Markdown, plain text, JSON, CSV | Define encoding, schema/version, units, and expected columns/fields as appropriate. |

These are examples, not universal requirements. Machine-specific formats may be
needed, and a syntactically valid export does not by itself prove a design is
safe or manufacturable.

## Project definition

An Ember project definition should be machine-readable, versioned, and kept
with the project. It describes project-wide conventions, not secrets or
unbounded permissions. A task contract adds the particular objective, allowed
scope, and requested deliverables.

The following is an illustrative YAML shape, not yet a finalized schema:

```yaml
schema: ember.project/v1
name: desk-organizer
purpose: Design and fabricate a modular desk organizer

structure:
  source: source/
  exports: artifacts/
  jobs: jobs/
  reports: reports/

artifacts:
  - id: enclosure
    kind: mechanical-part
    source:
      format: freecad
      path: source/enclosure.FCStd
    deliverables:
      - format: step
        path: artifacts/enclosure.step
      - format: 3mf
        path: artifacts/enclosure.3mf
    units: mm
    validation:
      - id: model-check
        command: freecad-check
      - id: print-preview
        requires-human-review: true

tools:
  - name: FreeCAD
    version: ">= 1.0"

fabrication:
  machine: printer-main
  start-job-requires-approval: true

agent-guidance:
  preserve-sources: true
  report-path: reports/agent-result.json
```

Command examples above are placeholders. Projects should name real, installed
validators or define validation through a trusted Ember adapter. Project files
must not be able to grant themselves permissions or bypass host authorization.

## Task work contract

### Linked project notes and resuming a task

When exchanging a project with notes, use the
[v1 notes profile](pathname:///project-schema/v1.notes.schema.json) of the
[portable manifest contract](pathname:///project-schema/README.md). List notes as
Markdown source files with project-relative paths; backlinks can be rebuilt.

Open **Project notes** from Ember's sidebar (`/apps/notes/`). **Create project**
creates `notes/index.md` and `notes/handoff.md` and opens the index automatically.
Browse or filter the note list, edit Markdown and save, follow a link, or use
**Backlinks** to see which notes refer to the open one. A link to a missing note
opens a new draft. Both `[[note title]]` and `[label](relative/path.md)` work.
The files are ordinary UTF-8 Markdown. For portable links use paths relative to the
containing note, e.g. `[material choice](decisions/material.md)`. No Obsidian
plugin, database or provider session is required. Start at `index.md`, then
read `handoff.md` for the current goal, last verified result and next step.
Link longer evidence and decisions from those notes instead of loading a whole
transcript on every run.

**Available now:** an operator-side CLI for local handoffs. From `apps/api`,
with an existing project directory and separate trusted parent for task copies:

```sh
go run ./cmd/ember-notes stage /data/project-a /tasks/run-1
# Give the sandbox only run-1 as its writable notes workspace.
# It writes decisions/material.md and links it from handoff.md.
# Stop the sandbox and review the changed files before publishing.
go run ./cmd/ember-notes publish /data/project-a /tasks/run-1 decisions/material.md handoff.md
go run ./cmd/ember-notes stage /data/project-a /tasks/run-2 decisions/material.md
```

`stage` fills in missing starter notes and creates a new workspace with the index, handoff and any explicitly
selected topic notes. Links do not automatically bring other files into scope.
`publish` copies only named Markdown files (up to 256 KiB each); the durable
vault stays outside the sandbox. Keep workspace parents operator-controlled
and publish only from a stopped task with no writers. Publish linked topics
before the handoff: replacement is atomic per file, not per batch. Serialize
CLI publishes with browser edits; the browser detects stale browser saves, but
the CLI does not merge changes. Automatic staging and publication by a sandbox
runner are future integration work.

### Run instructions

For each run, Ember should provide the agent:

- The project definition and relevant project instructions.
- A bounded objective and explicit in-scope/out-of-scope paths.
- Available tools and permissions, including whether external network access
  is allowed.
- Required deliverables, formats, paths, naming, units, and validation.
- Any approval gates, especially for machine operations or destructive actions.
- A structured completion-report location and schema.

The agent should save work to the project workspace, not rely on provider chat
history as the only copy. It should preserve editable source, keep derived
exports separate, avoid unrelated changes, and report exactly what it changed.

An illustrative result report:

```json
{
  "schema": "ember.task-result/v1",
  "task_id": "task-123",
  "status": "completed",
  "changed": ["source/enclosure.FCStd"],
  "created": ["artifacts/enclosure.step"],
  "validation": [
    {"check": "model-check", "status": "passed"},
    {"check": "print-preview", "status": "needs-human-review"}
  ],
  "tools": [{"name": "FreeCAD", "version": "1.0.0"}],
  "assumptions": [],
  "unresolved": []
}
```

Ember—not the model—must enforce filesystem scope, network policy, tool
permissions, approval gates, and machine-control authorization. Model
instructions are helpful guidance, not a security boundary.

## Provider portability

Provider adapters may express a task as system/developer instructions, tool
schemas, MCP connections, or runner-specific metadata. The adapter must not
silently change the project requirements. Log the provider/model and adapter
version for each run, while keeping project definitions and outputs readable
without that provider.

Before finalizing a schema or adopting an interoperability dependency, evaluate
candidate standards against actual target tools and record the decision in an
ADR. See the [implementation policy](decisions/004-go-and-library-policy.md).
