# ADR 007: Keep instance control state together and isolate execution

Status: Proposed  
Date: 2026-11-21  
Approval: pending [DEC-001](../planning/human-decisions.md#dec-001---control-plane-boundary).

**Problem.** One health handler is easy to deploy. Adding identity, projects and jobs as independently deployed services now would distribute authorization and state before their contracts or load are known. Putting executable agents in that handler's process would instead give them control-plane authority. A Raspberry Pi, the existing Go policy and replaceable, user-selected agents constrain both choices.

| Option | Cost and maintenance | Security and failure isolation | Scaling |
| --- | --- | --- | --- |
| One process including executable extensions | Low initial integration cost; difficult upgrades across plugin versions | A faulty plugin can read credentials or stop the API | CPU-intensive work competes with HTTP |
| Modular Go API, separate execution/privileged edges | Small internal interfaces; one core build and migration owner. Separate supervision only where required | Central ownership check, no untrusted code in core. API remains one failure domain | Vertical core scaling first; scale expensive workers separately |
| Microservice per domain and broker now | More release coordination, tracing, identity propagation and recovery | Smaller individual failures but more network and credential boundaries | Independent scaling possible at a cost not justified by current load |

**Recommendation.** Choose the middle option for the POC. Keep HTTP, application decisions and storage/adapter details separable in code as they appear. Prefer synchronous calls for short, bounded reads; use persisted tasks for heavy operations. Do not introduce a mandatory provider service, generalized plugin loader or speculative internal framework.

**Revisit when** observed workload or independent team/release needs require a new boundary. Before extracting a service, name its authoritative data, grant scope, deadline, retry behavior and operator. Unknowns: concurrency, hardware, data retention and trust of the first adapter. Approval does not extend to a sandbox or a database selection.
