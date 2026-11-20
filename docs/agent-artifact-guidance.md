---
sidebar_label: Agent artifact guidance
sidebar_position: 4
---

# Agent defaults: saving and publishing artifacts

This page defines recommended defaults for agent instructions and task
contracts. It does not imply that artifact storage, agent execution, or
frontend viewers are implemented in the current scaffold.

## Workspace layout

Unless a task contract overrides these paths, agents should treat the project
workspace as the durable source of truth and use:

```text
source/       Editable/native project files
artifacts/    Deliverables intended for preview, download, or fabrication
reports/      Validation results and task completion report
jobs/         Explicitly requested machine/job files (never auto-submit)
```

Keep source and exports separate. Use project-relative paths, preserve existing
work, and do not write outside the task's granted workspace. Do not depend on
chat history or temporary worker files as the only copy of a result. Ember must
enforce path scope; agent instructions alone are not a security control.

## Deliverables and frontend visibility

For every deliverable, the task should name its artifact ID, relative path,
format/media type, units where applicable, and validation requirements. Agents
should save the actual bytes at the declared path and list that path in the
completion report. They must not claim an artifact is published merely because
it exists in a temporary directory or was attached to a model response.

For a 3D model, a useful default is:

- Preserve the editable native source under `source/` (for example, a
  FreeCAD document or Blender project).
- Export a preview-friendly format under `artifacts/`. Use **GLB/glTF** for
  browser scene previews where supported; use **3MF** or **STL** for print
  workflows, and **STEP** for mechanical CAD exchange as appropriate.
- Include units, coordinate/orientation assumptions, and tool/version in
  artifact metadata or the task report. Do not silently convert units.
- Report validation performed and flag checks that need human review.

The frontend can only display an artifact when the platform has stored it in
the project artifact area and has a viewer for its media type. The task contract
should identify the intended viewer/use; agents must not assume every format
has an in-browser preview. When preview is unavailable, make the file
downloadable if the product supports it and retain the native source.

## Recommended task defaults

Tasks should provide agents with:

- Project manifest and relevant project instructions.
- Explicit objective, allowed input paths, writable output paths, and limits.
- Required outputs, formats, IDs, naming, units, and expected frontend use.
- Available tools and permissions, network policy, and approval gates.
- A required completion-report path, by default
  `reports/agent-result.json`.

For each artifact, report at least its ID, path, media type/format, role
(`source` or `deliverable`), and units when relevant. Also report tools and
versions, validation status, assumptions, and unresolved issues. A suggested
report shape is:

```json
{
  "schema": "ember.task-result/v1",
  "task_id": "task-123",
  "status": "completed",
  "created": [
    {
      "id": "assembly-preview",
      "path": "artifacts/assembly.glb",
      "media_type": "model/gltf-binary",
      "role": "deliverable",
      "units": "mm"
    }
  ],
  "validation": [
    {"check": "export-opened", "status": "passed"},
    {"check": "dimensions-reviewed", "status": "needs-human-review"}
  ],
  "tools": [{"name": "Blender", "version": "4.5"}],
  "assumptions": [],
  "unresolved": []
}
```

This is guidance, not a finalized result schema. The
[project schema](pathname:///project-schema/README.md) and
[agent project contract](agent-project-contract.md) describe the current
portable project-manifest direction. Model-generated reports and project
metadata must never grant permissions or bypass Ember authorization.
