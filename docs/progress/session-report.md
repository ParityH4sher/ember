# Ember - Autonomous Engineering Session Report

**Interim checkpoint.** The five-hour window is not complete. [Current engineering status](current-status.md) is authoritative for resumption.

## 1. Executive summary
Restored the API test gate, fixed source bundle layout and checksum portability, and added bounded API shutdown and request correlation. The original proxy lost its upstream after a container address change. The revised proxy recovered without a reload in native Docker and rootless Podman tests. Longer-running qualification is in progress.

## 2. Time usage
Started 2026-11-21 00:34:06 UTC. Phase 1 completed its 120-minute window. Implementation started after 02:34:06 UTC. Feature freeze 05:19:06 UTC; deadline 05:34:06 UTC. No final end time yet.

## 3. Initial repository assessment
The [dated evidence matrix](../architecture/current-state.md) separates a runnable health process and static proxy from project-format documentation and proposed workers. No persistence, authentication, agent runtime, MCP integration or artifact display.

## 4. Architecture work
Proposed one instance-owned Go control plane and isolated execution/privileged edges (ADR 007); accepted fast packaging layout verification as a reversible practice (ADR 008). Documented deployment, data ownership, task lifecycle and threat boundaries; selected no product dependency.

## 5. Documentation improvements
Linked the [engineering overview](../architecture/overview.md) from the docs index; added decisions, risk register, prioritized milestones, testing and troubleshooting guidance. Kept historical product/POC sources and existing ADR numbering. Corrected build path and inventory guidance.

## 6. Backend implementations
- Go API: server deadlines, signal-aware shutdown, drain/force-close and bind failure reporting; tests on real loopback listeners.
- Access telemetry: application-generated request IDs, structured JSON access/lifecycle logs and tests for response correlation and absence of raw request data.
- Delivery: effective manifest-relative build paths, relocatable checksum, source revision embedded in CI bundles, installer verification and health probe through Nginx.
- Runtime: refresh DNS after backend replacement; role defaults; supported, digest-pinned build inputs. No new dependencies in `go.mod`.

## 7. Quality improvements
Established reproducible evidence for test compilation, packaging and incomplete default provisioning. The corrected Compose invocation was validated on a disposable stack.

## 8. Test results
See the [experiment log](implementation-log.md) for commands, platform and actual outcomes. Baseline Go tests failed; revised tests pass under race detection. Bundle install/proxy tests passed on both engine paths. Docusaurus builds passed with Node 24 and 26 in isolated copies. Ansible qualification is in progress; physical Pi untested.

## 9. Human decisions required
[DEC-001–007](../planning/human-decisions.md) remain pending. Prioritize the application identity model and first POC target. Controller authority, sandbox and model gateway are not prerequisites to repair the baseline.

## 10. Remaining technical debt
Physical Pi qualification, signed releases, persistent recovery, full OpenAPI validation tooling and website dependency advisory triage remain open.

## 11. Recommended next steps
Finish instance-recovery and provisioning qualification, then prioritize human decisions needed for a private read-only integration. See the [roadmap](../planning/roadmap.md).

## 12. Repository status
Baseline `76baa4c`, initially clean `main`. Backend, tests, deployment, workflow and documentation changes remain uncommitted. No production deployment, remote push or commit.

## 13. Continuation instructions
Read [current status](current-status.md), check UTC and working tree, and distinguish outstanding experiments from completed evidence before proceeding.
