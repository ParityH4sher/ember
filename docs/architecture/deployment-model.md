# Deployment and data ownership

**Status: Proposed** beyond the two-container scaffold. An Ember instance is an independently configured control plane and its data, not a user account or a Compose service.

| Unit | Lifecycle/configuration owner | Isolation requirement |
| --- | --- | --- |
| Ember instance | Its operator owns ports, desired version, policies, private state and backup keys | Separate project name, network, writable roots and credential namespace. A header or instance ID is not proof of identity. |
| Shared default service | Deployment operator configures required shared dependencies (e.g. IdP, Git host) | Not deactivated through the plugin API. Authenticate each consumer and isolate data downstream. |
| Shared configurable service | Service operator owns deployment; Ember operator/user configures an included adapter (e.g. Immich) | Activation only controls this Ember instance's use. Disconnecting it never stops or changes the shared service. |
| Self-maintained service | Trusted runner provisions a task-scoped service only when explicitly permitted | Isolated, resource-bounded, and disposable by default; no host socket or shared-service credentials. |
| External integration | Upstream owner, including pre-existing applications and devices | Ember can request explicitly authorized API operations; no provision/update authority follows from connectivity. |
| Agent execution environment | Trusted runner supervisor | One task grant and bounded workspace. No ambient identity from the instance or host. |

### POC placement

Use operator-configured service origins, not LAN discovery. Compose DNS identifies peers within one project. `name: ember` and the default port collide for two installations unless both the project name and host port differ. Verify distinct networks with `compose -p ... config`; even successfully separated networks do not protect two consumers using the same upstream administrator account. There is no need for direct Ember-to-Ember communication. Health and quota errors from a shared service should affect only the dependent operation.

Ansible owns host prerequisites and can enable the container engine through its native service manager. Ember workloads remain Compose services; no Ember systemd unit is needed. Keep each shared application in an independently owned deployment with its own backup and rollback procedure. A deployment script or agent must not delete a shared volume as a consequence of deleting a consumer.

Measure a Pi with the selected OS, RAM, storage medium and thermal limits. Record free disk during image builds, peak memory during restart, request latency under one representative job, I/O during backup, and behavior when capacity runs out. Cross-compilation alone says nothing about those limits. Prefer moving expensive jobs off the Pi over provisioning Kubernetes. Consider an orchestrator only with an identified multi-node scheduling or availability requirement and an operator able to maintain networking, storage, upgrades and recovery.

### Data model and recovery gates

Keep project sources and validated artifacts portable and project-relative; use the manifest under `contracts/project-schema/`. Control records (ownership, tasks, approvals, idempotency keys, run attempts and audit references) belong to the instance, not to a model transcript. Search indexes and previews should be rebuildable. Secrets are references into an operator-controlled store, never portable project metadata.

Use an [instance-owned filesystem for project files](../decisions/009-project-file-storage.md). Before adopting a database for control records, decide the write workload and restore objectives. For a single writer on one host, compare SQLite with an instance-owned PostgreSQL database. A shared PostgreSQL server still needs distinct databases/roles, restore permissions, quotas and backup tests. Shared tables keyed only by an instance ID concentrate policy risk. Do not let workers connect to any control database.

A future artifact publish should stage bytes privately, bound and validate regular files, compute digests, then commit visible references together. Readers must not see a completed task referencing a partial file. Reconcile orphaned staged bytes after interruption. Filesystem and SQL operations do not form one transaction; define crash recovery explicitly. Changes to project format require version checks, a tested export/restore path and an approved migration.

There are no durable app records to back up today. Before introducing any: name the operator, set recovery point/time targets and retention, snapshot matching metadata and artifact revisions, encrypt off-host copies, keep recovery keys independently, and rehearse restore to a fresh isolated instance. Verify ownership and hashes before restoring traffic. A service restart or successful archive upload is not a restore test.
