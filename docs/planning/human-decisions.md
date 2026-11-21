# Human decision register

These are approval requests, not a list of technologies already chosen. Record the approver, date, scope and linked ADR when resolving an entry. The default assumptions allow work on the health scaffold; none grants authority to run a privileged operation. First answer DEC-002 (application identity) and DEC-005 (available target). They determine whether the next proof can use any private service data.

## DEC-001 - Control plane boundary

Status: Requires Human Input  
Priority: High

### Context
The API will need to coordinate identity and durable task state. A process split changes release, recovery and on-call responsibilities.

### Options
A. Modular Go API with separate execution/management boundaries. B. Independent network service per domain. C. Embed worker and plugin code in core.

### Comparison
A keeps transactions and policy local but shares core failures. B scales independently at the cost of distributed contracts and machine credentials. C is initially cheap but gives third-party code control-plane privileges. [ADR 007](../decisions/007-core-and-execution-boundaries.md) compares the options.

### Recommendation
A. Keep the existing Go/standard-library policy. Split according to trust or measured resource pressure.

### Consequences
Sets packaging and data ownership; does not select worker or database technology.

### Dependencies
First persistent domain module and separately deployed executor.

### Default Assumption
Harden the existing API in place. No framework replacement or new services.

### Human Decision
Pending

## DEC-002 - Identity and secret custody

Status: Requires Human Input  
Priority: Critical

### Context
Neither loopback nor a private LAN establishes user, workload, or instance identity.

### Options
A. Operator-selected OIDC issuer for people; separate revocable workloads and scoped service secrets. B. Local single-operator credentials with an explicit enrollment and recovery flow. C. Trust access to the network alone.

### Comparison
A can reuse an existing identity service, but issuer availability, audience validation and recovery need an owner. B reduces infrastructure but makes Ember responsible for enrollment, revocation and secure storage. C cannot express resource ownership and is unsuitable for private data or writes.

### Recommendation
Use A where an issuer already exists. Decide offline/access recovery, secret source and retention before relying on it.

### Consequences
Changes HTTP contracts, machine grants, browser session handling and backup exclusions.

### Dependencies
Private data, remote ingress, tool invocation, multiple human users.

### Default Assumption
No private or mutating endpoint. No live login attempt, credential rotation or ingress deployment.

### Human Decision
Pending

## DEC-003 - Persistence and restore objectives

Status: Requires Human Input  
Priority: High

### Context
The manifest is portable metadata. It does not provide transactional task admission, authorization records or backup consistency.

### Options
A. Per-instance SQLite and project files. B. Per-instance database/role on PostgreSQL, potentially on a shared host. C. Project files alone.

### Comparison
A minimizes Pi operation but constrains concurrent writers and introduces a driver choice. B supports more writers at the cost of database operations, network failures and restore coordination. C suits portable sources but leaves locking and recovery of multi-record state to Ember.

### Recommendation
Measure A for a single writer first. Get explicit approval for the driver, license and maintenance plan; require tested restore before user data.

### Consequences
Migration semantics, backup boundary, retention, bytes-at-rest protection and horizontal scaling depend on the choice.

### Dependencies
Durable tasks, idempotency, approvals, artifact publication.

### Default Assumption
No persistent service, schema migration or credentials in the deployment.

### Human Decision
Pending

## DEC-004 - Execution threat class and placement

Status: Requires Human Input  
Priority: Critical

### Context
Reviewed converters, arbitrary project code and personal-service adapters do not require or tolerate the same privileges.

### Options
A. Allow vetted bounded processing in hardened rootless containers only. B. Authorize hostile-code execution on a qualified disposable VM or separate host. C. Offer no executable workloads.

### Comparison
A uses fewer resources but shares the host kernel. B adds kernel/image maintenance and hardware qualification but better isolates untrusted code. C preserves the current boundary while leaving fabrication tasks external.

### Recommendation
C until a workload and its owner are named; evaluate A for reviewed tools, B for hostile code. Never downgrade automatically from B to A.

### Consequences
Determines grant issuance, egress enforcement, filesystem staging, cleanup and audit collection.

### Dependencies
Real agent runner, executable extensions and processing uploaded files.

### Default Assumption
No executable extensions, host mounts, shell endpoint or worker supervisor.

### Human Decision
Pending

## DEC-005 - First integration and target hardware

Status: Requires Human Input  
Priority: High

### Context
The POC brief needs two instances to use one service; the product vision eventually needs validated designs and approvals. The target Pi's model, memory, disk, OS and existing services are not specified.

### Options
A. Read-only Paperless or Immich connection using disposable scoped accounts. B. A lighter real service on the Pi or an external host. C. Start with a CAD/device integration instead.

### Comparison
A exercises realistic private data boundaries but needs service-specific authorization and a larger footprint. B is easier to qualify if it still demonstrates upstream separation. C addresses the making workflow sooner but adds toolchain compatibility and potentially physical side effects.

### Recommendation
A if a representative fixture and resources exist; otherwise B. Record actual Pi capacity, operator, two account scopes and success criteria before selecting the target. Avoid physical actions for the first proof.

### Consequences
Determines adapter contract, discovery, resource limits and device acceptance tests.

### Dependencies
First integration, shared service recovery test and Pi qualification.

### Default Assumption
Design against local HTTP fixtures. Compose for one host; no mandatory Kubernetes or local inference.

### Human Decision
Pending

## DEC-006 - Lifecycle authority and sharing

Status: Requires Human Input  
Priority: Critical

### Context
Using a shared application does not imply permission to restart, upgrade or destroy it. Sharing credentials can collapse instance boundaries even with separate networks.

### Options
A. Operator manages all lifecycle changes outside Ember. B. Separate controller performs approved, allowlisted operations on registered resources. C. Grant the core or agents direct runtime access.

### Comparison
A has lowest operational and security cost but no self-recovery beyond existing restart policy. B can bound recovery but needs an independent identity, registry, audit trail, locking and operator. C exposes the host or all consumers to a compromised application and is unsuitable.

### Recommendation
A for now; consider B only for a named, bounded operation with explicit service owner consent. Shared services must be separable from each consumer's deploy/restore lifecycle.

### Consequences
Defines who may change desired state, approve recovery, handle maintenance windows and resolve cross-instance conflicts.

### Dependencies
Self-maintenance, shared service update automation, controller credentials.

### Default Assumption
No lifecycle API, no automatic discovery and no credentials granted across instances.

### Human Decision
Pending

## DEC-007 - Model routing, privacy and spending

Status: Requires Human Input  
Priority: High

### Context
Different users may bring local or remote models. Fallback or shared caching could send private project data to a new processor; concurrency can exceed a budget charged only after a response.

### Options
A. No gateway product; monitored narrow adapter per configured provider. B. Qualify an existing LLM gateway with independent tool authorization. C. Build a custom routing and tool gateway.

### Comparison
A minimizes dependencies but may lack multi-provider governance. B can reuse streaming, quota and compatibility work but entails licensing, credential custody and Pi overhead. C maximizes control and adds a large compatibility, security and maintenance obligation.

### Recommendation
Follow ADR 003: evaluate A with a mock first. Consider B only against measured gaps. Require explicit eligible destinations, pre-request budget reservation, accounting for interrupted streams and no unauthorized fallback. Tool calls remain requests, not grants.

### Consequences
Determines data movement, accounting, rate limits, usage attribution, support for structured output, and who can inspect request traces.

### Dependencies
Paid inference, remote provider traffic, shared model gateway, model-assisted tool use.

### Default Assumption
No credentials, paid calls, prompt logging, shared cache or model selection by Ember.

### Human Decision
Pending
