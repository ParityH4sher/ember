# Human decision register

These are approval requests, not a list of technologies already chosen. Record the approver, date, scope and linked ADR when resolving an entry. The default assumptions allow work on the health scaffold; none grants authority to run a privileged operation. First answer DEC-002 (application identity) and DEC-005 (available target). They determine whether the next proof can use any private service data.

## DEC-001 - Control plane boundary

Status: Decided  
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
2026-11-21 — Project operator selected **A**: keep identity, project and task
coordination in one modular Go API. Run execution and infrastructure management
outside the API's privilege boundary, even when colocated on the Pi.
See [ADR 007](../decisions/007-core-and-execution-boundaries.md).

## DEC-002 - Identity and secret custody

Status: Decided for the disposable prototype
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
2026-11-21 — Project operator selected **A**: use OIDC for human sign-in,
with Authentik as the default identity service and the issuer configurable
by the operator. Workloads use separate, revocable credentials and scoped
service secrets. This is a design decision; sign-in is not deployed.
If Authentik is unavailable, block new sign-ins and pause authenticated access
for existing human sessions. The operator restores Authentik using host access;
there is no backup Ember login. For the prototype, the operator keeps service
credentials in permission-restricted files on the Pi, outside repositories and
project workspaces. Mount each secret read-only into only the service that
needs it; keep each instance's database password and Immich key separate.
Exclude these prototype service credential files from backups. After storage
loss, the operator revokes the old credentials and issues replacements during
the rebuild.

## DEC-003 - Persistence and restore objectives

Status: PostgreSQL, NATS JetStream and disposable-prototype recovery objectives selected
Priority: High

### Context
The filesystem is selected for portable project files ([ADR 009](../decisions/009-project-file-storage.md)). The manifest does not provide transactional task admission, authorization records or backup consistency.

### Options
A. Add per-instance SQLite alongside project files. B. Add a per-instance database/role on PostgreSQL, potentially on a shared host. C. Defer a database until a workflow needs durable control records.

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
2026-11-21 — Project operator selected **B**: store task and approval records
in PostgreSQL, with a separate database and role for each Ember instance.
Project files remain on the instance-owned filesystem. For disposable prototype
fixtures, losing all Ember test records and project files is acceptable; they
can be recreated, with no restore deadline. Revisit these targets before
storing real user work. The operator selected **NATS JetStream** over a custom
Go/KV broker for thumbnail job delivery. Queue job IDs; keep photos on the
filesystem and authoritative task and approval records in PostgreSQL. Before
deployment, measure broker resource use on the Pi and verify that redelivery
after a worker restart cannot publish a thumbnail twice. Both Ember instances
will share one operator-managed NATS server on the Pi. Give each instance a
separate NATS account, credentials and JetStream streams. Verify that each
instance can publish and consume its own jobs but cannot publish or consume
jobs in the other account.

## DEC-004 - Execution threat class and placement

Status: Reviewed tools and first job selected; tool owner pending  
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
2026-11-21 — Project operator selected **A** for the prototype: only vetted,
fixed operations in hardened rootless containers. Projects may supply inputs,
not commands or executable plugins. The first job to qualify on the Pi is
thumbnail generation: resize one staged, read-only test photo to a fixed size
without network access or Immich credentials. No job is enabled by this
decision; the project operator explicitly deferred assigning a tool owner.

## DEC-005 - First integration and target hardware

Status: First integration and placement selected; hardware qualification and test setup pending  
Priority: High

### Context
The POC brief needs two instances to use one service; the product vision eventually needs validated designs and approvals. Qualification depends on the capacity left after the OS and existing services are accounted for.

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
2026-11-21 — Project operator selected **Immich** for the first integration.
The proof is read-only: two Ember instances use separate disposable Immich
accounts, each able to read its own test photos but not the other's. Both Ember
instances will run on the operator's existing Raspberry Pi 5 with 8 GB RAM
and a 32 GB SSD, running Ubuntu Server. Immich already runs on a separate
server and is the shared upstream for the proof. The operator can create two
disposable Immich accounts there, one per Ember instance, with a distinct
test photo in each. Account creation and the isolation test are pending.
2026-11-23 — The read-only Immich adapter exists, and one dedicated non-admin
account (`ember-poc`) is configured for the homelab instance. The second
account, the second Ember instance and the isolation test are still pending.
The SSD figure is total Pi capacity; the Ubuntu release, free disk space and
other Pi services are not yet confirmed.

## DEC-006 - Lifecycle authority and sharing

Status: Decided  
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
2026-11-21 — Project operator selected **A**: the operator manages service
restarts, upgrades and removal outside Ember. Neither Ember instance nor its
agents receive lifecycle credentials for shared services such as Immich and
Authentik.

## DEC-007 - Model routing, privacy and spending

Status: LiteLLM selected for the first pilot; production adoption pending
Priority: High

### Context
Different users may bring local or remote models. Fallback or shared caching could send private project data to a new processor; concurrency can exceed a budget charged only after a response.

### Options
A. No gateway product; monitored narrow adapter per configured provider. B. Qualify an existing LLM gateway with independent tool authorization. C. Build a custom routing and tool gateway.

### Comparison
A minimizes dependencies but may lack multi-provider governance. B can reuse streaming, quota and compatibility work but entails licensing, credential custody and Pi overhead. C maximizes control and adds a large compatibility, security and maintenance obligation.

### Recommendation
Qualify the shared router against mock providers first (see [ADR 003](../decisions/003-ai-gateway-shortlist.md)). Require explicit eligible destinations, pre-request budget reservation, accounting for interrupted streams and no unauthorized fallback. Tool calls remain requests, not grants.

### Consequences
Determines data movement, accounting, rate limits, usage attribution, support for structured output, and who can inspect request traces.

### Dependencies
Paid inference, remote provider traffic, shared model gateway, model-assisted tool use.

### Default Assumption
The current scaffold makes no model calls. Use mock providers for the routing proof.

### Human Decision
2026-11-21 — Project operator requires a shared AI gateway/router as a default
service in the target Ember deployment. All model calls go through it. Model
configuration is scoped to the user: the router automatically chooses an
eligible model for the task, without requiring a model choice at submission.
If no eligible model is available, report that outcome rather than using
another user's configuration or an unauthorized destination. No gateway is
deployed yet. Use existing gateway software rather than building
an Ember gateway. Pilot **LiteLLM Proxy** as the single gateway for both model
routing and MCP tool access. Test automatic per-user model selection separately
from read-only Immich tool access across the two instances. Production adoption
depends on those isolation tests and qualification on the target Pi.

Hosted-model approval is per user and applies across that user's projects by
default. A local-only project excludes all hosted models, including on retry
or fallback. If no eligible local model can perform the task, report that no
route is available. Configuring a provider for one user does not approve it
for another.

Each user sets their own monthly limit for metered model calls. For now, use
subscriptions rather than pay-as-you-go billing. A subscription does not
authorize metered overages or fallback to a billed API. Before enabling a
live route, verify the specific plan supports access through the gateway.

Pilot targets are ChatGPT/Codex and Claude subscriptions, plus a local Ollama
endpoint. Verify each subscription's supported access method and plan limits
separately. For the pilot, run LiteLLM on the Ember Pi and Ollama on a
separate LAN machine. Qualify the Pi-to-Ollama path as the local inference
route; the specific inference machine and model remain open.
These are evaluation targets, not mandatory providers for every user.

## DEC-008 - Visibility of Ember container images

Status: Decided
Priority: Medium

### Context
CI publishes Ember's four images to the GitHub Container Registry
([ADR 011](../decisions/011-prebuilt-registry-images.md)). A new GHCR package
can start out private, and its visibility is set per package in GitHub.

### Options
A. Public packages. B. Private packages, with a `read:packages` token on each
deploying host.

### Comparison
A needs no credential on the host, and anyone can pull the images, which contain
only what the repository already holds. B keeps the images private, but each
host stores a long-lived token unencrypted in root's Docker configuration, and
the token must be rotated (DEC-002).

### Recommendation
A if the repository is public. B only if the source is private.

### Consequences
B needs `ember_registry_username` and `ember_registry_token` from a Vault file.

### Dependencies
Repository visibility, DEC-002.

### Default Assumption
The role logs in only when a token is supplied.

### Human Decision
2026-11-22 — Project operator selected **A**: the four `ghcr.io/parityh4sher/ember-*`
packages are public, so hosts pull them without credentials.

2026-11-23 — [ADR 013](../decisions/013-single-web-frontend.md) replaces
`ember-home` and `ember-notes` with a new `ember-web` package. Under this
decision it must be made public after its first publish.

## DEC-009 - Agent chat from the web API

Status: Requested by the project operator on 2026-11-23; design proposed in the
[project chat plan](project-chat.md); deployment scope pending  
Priority: High

### Context
The operator asked for a chat in a project that sends each message to an agent
running in the sandbox router and shows the answer. Until now the router was
reachable only by the operator, and DEC-004 enabled no job. Connecting it to the
web API lets any caller of the chat route start a container on the operator's
host.

### Options
A. Keep the router operator-only; agents run only from the command line.
B. Let the API call the router for operator-registered agents, off by default,
with sign-in required unless the operator explicitly allows loopback
development. Agents stay offline until a model route exists.
C. As B, and also give agents a network route to the model gateway now.

### Comparison
A keeps today's boundary and gives no chat. B exposes container launches to
signed-in users but keeps the router's sandbox, limits and offline network.
C is needed for real model answers, but it puts message text and credentials
on the path to a provider before DEC-007's gateway is qualified.

### Recommendation
B now, C as a separate decision once the LiteLLM pilot provides per-agent keys
and egress limited to the gateway.

### Consequences
Runs are tied to the API's single router slot and are not durable (DEC-003).
Roles are not scoped to projects, so any signed-in user can chat in any project.
Transcripts become project files under `chats/`.

### Dependencies
DEC-002 (identity), DEC-003 (durable task records), DEC-004 (execution
placement), DEC-007 (model routing).

### Default Assumption
No agents configuration file is shipped. Chat runs stay off, and the Compose
deployment does not mount the router socket.

### Human Decision
Pending. Approve B (or another option), name who may register agent images, and
decide whether chat may run on an instance reachable beyond loopback.
