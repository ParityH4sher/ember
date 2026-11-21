# Task lifecycle and runner contract

**Status: Proposed.** `services/ai-worker/README.md` describes a role; there is no executor, job endpoint or queue. A person chooses their agent and provider. The API owns the task's identity, scope, durable state and outcome; an agent-specific session ID is metadata, never the primary project identifier.

| Event | Proposed state transition | Guard and owner |
| --- | --- | --- |
| Submit | new → queued | API authenticates, authorizes project ownership, bounds requested inputs/outputs and captures a policy revision. Commit task and enqueue intent together before returning an ID. |
| Claim | queued → running | Dispatcher atomically assigns an expiring lease and increasing attempt number. One active writer per attempt; start with one active job per instance on a Pi. |
| Request consequential action | running → awaiting approval | Broker records canonical action and target; do not hold a worker or database transaction indefinitely while waiting. A grant to generate output is not a grant to send it to a device. |
| Approve or deny | awaiting approval → queued or terminal | Recheck identity, expiry and exact parameters; unauthorized or stale approvals have no effect. Resume with a new fenced attempt when needed. |
| Complete | running → succeeded or failed | Accept result only for current live attempt; validate bounded exports and commit references before publishing completion. Distinguish tool failure from invalid result. |
| Cancel/expire | queued → cancelled; running → stopping → terminal | Revoke new calls, signal execution supervisor, wait for confirmed process-tree termination and clean up scratch. A dropped HTTP stream is not cancellation. |
| Lose worker | running/stopping → interrupted | Reconcile runtime evidence after lease expiry. A stale worker cannot commit results after reassignment. Record unknown upstream outcome; do not silently mark success or retry. |

**Scheduling and retry.** Start with a durable task table and transactional claim *if a store is approved*; no distributed broker is required to prove one runner. Enforce per-instance and per-user admission quotas, bounded pending work, per-attempt timeout and overall deadline. Retry transient, idempotent operations with capped backoff/jitter and an attempt limit. For side effects, use an upstream idempotency key or reconcile first. Never replay a device command, approval consumption or paid request just because the caller timed out. A scheduler can enqueue through the same admission rules later; scheduled jobs do not get extra authority.

**Task envelope.** Instance/user/project/task IDs, input revision, runner contract version, requested capabilities, granted scope, deadline, resource limits and expected artifact manifest. Mount a curated snapshot, never an unrestricted project parent. Separate scratch per attempt and export only regular, authorized outputs. Record image/tool versions, attempt, policy, validation and unresolved errors outside the worker. Purge scratch after export or on failure; report cleanup failures and quarantine the runtime rather than reusing it.

**Client behavior.** Poll authorized durable status first. Add event streaming only when resumable IDs, retention and disconnect behavior are defined. A reconnect reads committed task state; a client must not resubmit a non-idempotent task to recover its view.

**First qualification:** use a deterministic fixture runner with no external access. Cover duplicate submission, racing claims, stale completion, cancellation during export, restart with an expired lease, unknown external outcome, and disk quota exhaustion. A fixture validates orchestration, not containment of hostile code. Live agents, credential issuance and runner technology depend on the [human decisions](../planning/human-decisions.md).
