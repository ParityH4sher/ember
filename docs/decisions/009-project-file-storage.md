# ADR 009: Keep project files on the filesystem

Status: Accepted for project files; prototype database and queue selected in DEC-003
Date: 2026-11-21

An operator should be able to copy a project to another disk and open its sources
without a running Ember instance. Give each project its own directory on the
instance filesystem. Keep its versioned manifest, editable sources, and published
artifacts together. Manifest paths such as `source/bracket.py` are relative to
that directory and stay valid after the copy.

| Data | Decision |
| --- | --- |
| Project manifest and file contents | Ordinary files are the source of truth. |
| Ownership, approvals, task status and attempts | Instance records in PostgreSQL, outside the portable project. |
| Pending work | NATS JetStream delivers job IDs. Redelivery must not publish a completed result twice. |

**First implementation.** `apps/api/internal/projectfiles` reads and writes files
under an existing, caller-selected project directory. It rejects symlink components
and stages writes beside the destination before renaming, so a reader never sees
a partial replacement. The instance must control the directory and its parents;
workers edit separate workspaces. `.ember-stage-` is reserved for temporary files.
The store does not validate manifest contents or make multiple file writes one
transaction. Project creation now seeds a `notes/` vault beside the local
`record.json` registration; the frontend reads and edits these Markdown files via the API.

**Next integration check.** With a fixture project, copy a quiescent directory
to a fresh root, validate its manifest, and compare file digests. Before deploying
for private data, define snapshot consistency, power-loss recovery, retention
and restore objectives in [DEC-003](../planning/human-decisions.md#dec-003---persistence-and-restore-objectives).
