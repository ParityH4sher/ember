---
sidebar_label: Gaps and improvements
sidebar_position: 5
---

# Gaps and improvements

This is a prioritized gap analysis supplied with the feature wishlist. Its
evidence describes the **reference personal-Ember system**, checked against
that system's code on 20 November 2026. It is not an audit of this repository:
the current Ember scaffold is much smaller. Treat proposed fixes as design
input, not as verified findings about this codebase or committed
implementation decisions.

Effort estimates are from the source analysis: S = under a day, M = a few days,
L = weeks. The ordering is the source author's priority.

## Fix now in the reference system

| # | Gap | Evidence in reference system | Why it matters | Suggested change | Effort |
|---|---|---|---|---|---|
| 1 | Backups sit in the same box | Nightly restic backup goes to a second disk in the same server; off-site copy awaits new hardware. | A physical incident or ransomware could take both copies. | Add encrypted off-site restic storage and test restores from that copy monthly. | S |
| 2 | No CI | No `.forgejo/workflows`; 2,407 tests are run manually or in sessions. | Broken changes can reach the main branch without a test failure blocking them. | Run tests on every push in Forgejo Actions and require a passing run before lane merges. | S |
| 3 | Untrusted text reaches tools with side effects | Chat and voice workers have all 19 MCP tools while reading web pages, research, and transcripts; approval cards use policy. | Prompt injection in outside content could influence a process able to control apps or files. | Separate reading from acting; require renewed approval for actions after untrusted input, and log tool calls with input provenance. | M |
| 4 | Tailnet presence is the only login | None of 153 routes checks the requester; the system assumes one user. | Any device on the tailnet may reach sensitive capabilities. | Authenticate every request using Tailscale identity or per-device credentials, with scoped permissions. | M |
| 5 | Code is not shareable as-is | Hostname appears in 26 tracked files; personal details in 37; no README or LICENSE; private repository. | Sharing code could expose personal information, including in history. | Move personal values to configuration/secrets; start clean or carefully scrub history; add README and license. | S–M |

## Structure: build into the new core

These are architectural lessons for the new core and gradual module migration,
not recommendations to retrofit the current small scaffold wholesale.

| # | Gap | Evidence in reference system | Why it matters | Suggested change | Effort |
|---|---|---|---|---|---|
| 6 | One 10,900-line server | `hud/server.py` handles 153 paths in 132 branches, with global state and hand-written SSE; voice, chat, and workshops share a process. | A crash affects unrelated functions; testing and parallel development are difficult. | Use clear service/router boundaries, typed models, and generated OpenAPI; isolate voice. Framework choice remains open. | L |
| 7 | No consistent API contract | Routes mix resource areas and naming; transport combines four SSE channels, polling, and long polls. | Each new client must learn many one-off interfaces. | Define versioned resources and typed event streams for clients. | M |
| 8 | Workshop runs are scripts, not jobs | About 80 scripts have separate command lines; progress uses feed files. | Start, progress, cancellation, retries, and history differ by tool. | Provide a shared job lifecycle and runner interface for workshop modules. | M |
| 9 | State lacks a schema | State is stored in JSON folders; chats exist only as Claude transcripts. | Changes are hard to migrate and query consistently. | Use a migrated database for structured operational state; keep the user-owned vault in Markdown. | M |
| 10 | Sessions steered through a terminal | Relay types `/clear` into tmux; this has erased visible output and retained old model/effort settings. | Terminal/UI automation is fragile and hard to test. | Drive sessions through a supported structured API, explicitly setting model and effort on each call. | M |

## Quality and operations: improve along the way

| # | Gap | Evidence in reference system | Why it matters | Suggested change | Effort |
|---|---|---|---|---|---|
| 11 | No reproducible setup | No Python project/lock files; numbered setup scripts are run manually. | Rebuilds depend on undocumented knowledge. | Add locked dependencies and infrastructure-as-code or a tested bootstrap; validate in a sandbox. | M |
| 12 | Code explanations live elsewhere | Docstrings rely on private vault plans and project codenames; one-off setup scripts sit beside live scripts. | New contributors cannot understand or safely operate the code from the repository alone. | Add README and architecture docs, self-contained docstrings, repository ADRs, and separate setup scripts. | S |
| 13 | Many screenshot harnesses | 21 screenshot scripts each have their own stand-ins and checks. | Duplicated harness logic increases maintenance. | Use one visual-test harness configured per page. | S–M |
| 14 | Monitoring checks liveness, not correctness | Uptime Kuma and heartbeat check availability; errors are spread across logs. | Problems may only surface when a user notices. | Use structured logs with request/job IDs, centralized access, and dashboard error counts. | M |
| 15 | Rules live in a large prompt | `CLAUDE.md` is 16.5 KB and accumulates dated rules; some recurring errors persist. | Every session pays the context cost, while prose rules can be missed. | Put enforceable rules in hooks/tests, keep the entry prompt short, and version prompts with evaluation sets. | M |
| 16 | One budget is a point of failure | At the weekly usage limit, voice, chats, and nightly review stop; embeddings are the only local-model use. | Important functions can go unavailable for an extended period. | Evaluate a model layer with local fallbacks and API overflow, with privacy and cost controls. | M |
| 17 | Human review is a bottleneck | Planned PC checks total about 180 minutes in one sitting, with more queued. | Completed work waits for manual validation. | Automate repeatable checks in a Windows test VM; reserve human review for judgment and bundle releases. | M |

## Preserve these design choices

The source analysis recommends keeping:

- Markdown and Git as the memory store: readable, diffable, synchronized, and
  re-indexable.
- Claude Code as the current agent runtime rather than building an agent loop.
  For Ember, runtime integrations must remain replaceable and user-specific.
- Recipes as code in sealed Docker environments, checked before users see
  outputs.
- Hooks and tests for hard rules rather than relying on prompt text alone.
- A thin PC shell that serves pages from the server, supports signed updates,
  and has a kill switch.

These are source-system recommendations, not proof that the current Ember
repository already provides these capabilities. See the
[feature catalogue](feature-catalogue.md) for the broader wishlist and
[architecture status](architecture/README.md) for this repository's actual
implementation.
