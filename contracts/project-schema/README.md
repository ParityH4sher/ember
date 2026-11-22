# Ember project format, version 1

`v1.schema.json` defines the portable project manifest. File paths are
project-relative; implementations must additionally reject symlinks and verify
resolved paths remain inside the project root. A schema check alone is not a
filesystem security boundary.

## Linked Markdown notes

`v1.notes.schema.json` is an optional profile of `v1.schema.json`. Projects
without notes can continue using the base schema. In a portable project with
notes, list the vault's UTF-8 Markdown files in `files` using project-relative
paths, `mediaType: text/markdown`, and `role: source`. The two entry points are:

- `notes/index.md`: the starting page and links to topics.
- `notes/handoff.md`: the current goal, last verified result and next action.

Use ordinary `[label](relative/path.md)` links relative to the containing note;
targets must stay inside `notes/`. Ember also understands `[[title]]` and
`[[folder/title|label]]`. A bare title resolves to a sibling first, then a unique
title in the vault. Folder-qualified wiki links start at the vault root unless
prefixed with `./` or `../`. Backlinks are derived from note content.

The notes API uses **vault-relative** paths (`handoff.md`); manifests use
**project-relative** paths (`notes/handoff.md`). The running API seeds the files
on project creation. Its `record.json` is an instance registration, not a
portable manifest.

Example manifest conforming to both schemas:

```json
{
  "schemaVersion": "1.0",
  "id": "bracket-prototype",
  "name": "Mounting bracket",
  "createdAt": "2026-11-20T12:00:00Z",
  "files": [
    {
      "path": "source/bracket.py",
      "mediaType": "text/x-python",
      "role": "source"
    },
    { "path": "notes/index.md", "mediaType": "text/markdown", "role": "source" },
    { "path": "notes/handoff.md", "mediaType": "text/markdown", "role": "source" }
  ]
}
```

Incompatible format changes require a new schema version and an explicit
migration strategy. See the
[agent project contract](../../docs/agent-project-contract.md).
