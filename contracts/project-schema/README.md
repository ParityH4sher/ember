# Ember project format, version 1

`v1.schema.json` defines the portable project manifest. File paths are
project-relative; implementations must additionally reject symlinks and verify
resolved paths remain inside the project root. A schema check alone is not a
filesystem security boundary.

Example:

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
    }
  ]
}
```

Incompatible format changes require a new schema version and an explicit
migration strategy. See the
[agent project contract](../../docs/agent-project-contract.md).
