# Ember POC — Architecture and Implementation Handoff

- **Status:** Proposed architecture and proof-of-concept brief
- **Date:** 2026-11-20
- **Audience:** POC implementation agent and Ember contributors
- **Purpose:** Translate the architecture discussion into an actionable,
  reproducible POC without treating unresolved design choices as established
  requirements.

## 1. Context and vision

Ember aims to be one interface for many applications and workflows, including:

- Immich (photos)
- Paperless-ngx (documents)
- 3D printers and related tools
- AI tools and workflows
- CAD, slicers, Blender, and other creative or engineering tools

An exploratory system already demonstrates the potential of a flexible,
self-maintaining, AI-driven environment. This POC has a different engineering
goal: make Ember reproducible, secure, maintainable, and stable while preserving
extensibility.

The intended POC target is a Raspberry Pi provisioned with Ansible. The design
should also make sense on a homelab server or larger infrastructure. The UI
should be independent of the platform services behind it.

### Tension to resolve

- **Flexibility:** AI and integrations should let Ember connect to and work with
  many services.
- **Stability:** Infrastructure state should be predictable, reviewable,
  reproducible, and recoverable.
- **Proposed compromise (Option C):** Ember can connect to compatible services
  and may request lifecycle operations through a separately controlled
  infrastructure management layer.

**Guiding principle:** Ember may be autonomous in using its capabilities, but
must be constrained when changing the infrastructure that provides them.

## 2. Current repository baseline

The repository is an early scaffold, not an implementation of the full POC. At
the time of writing it contains:

- `apps/api`: a Go API with `GET /healthz`.
- `apps/frontend`: a static status page.
- `services/`: documented future boundaries for an AI worker, model adapter,
  and CAD worker; these are not running services.
- `contracts/openapi.yaml` and `contracts/project-schema/`: initial contracts.
- `deploy/compose.yaml`: Compose deployment for the API and frontend.
- `infra/ansible/`: initial Debian/Ubuntu provisioning and deployment roles.
- `docs/`: architecture, decisions, operations, and sandboxing documentation.

The GitHub Actions workflow tests the API and packages a portable Compose source
bundle. The target host builds container images for its own architecture.

The proposed [AI gateway architecture and evaluation](docs/architecture/ai-gateway.md)
is a separate design proposal. No gateway product is selected or deployed yet.
See [API and integration contracts](docs/architecture/api-contracts-and-integrations.md)
for where HTTP, MCP, plugin, and upstream API definitions belong.

Do not assume that the proposed POC workflow, integrations, controller, shared
service, authentication, or persistent data model already exist. Inspect the
code and update this status as implementation proceeds.

## 3. Architectural boundaries

Keep the following roles distinct, even if the POC co-locates them on one
device.

### 3.1 Ember Platform / Core

Responsibilities:

- User-facing UI and API
- Integration/plugin catalog and configuration
- Workflow orchestration and potential AI interfaces
- User identity and permissions
- Connections to service instances

It must not directly run arbitrary shell commands, mutate the container runtime
socket, or assume it owns every connected service.

### 3.2 Integration adapter

An integration definition describes how Ember communicates with a *type* of
service, independently of any deployed instance. It should declare:

- Stable integration identifier, e.g. `paperless` or `immich`
- Connection configuration schema and secret references
- Authentication method
- Supported capabilities/actions
- Health or connectivity probe
- API/client implementation

A service connection is one configured endpoint of that integration type. It
should have a connection ID, integration type, endpoint, credential reference,
permissions/visibility, and optional metadata.

One Ember instance may connect to multiple instances of one integration type.
Multiple Ember instances may connect to the same service instance using
independent credentials.

### 3.3 Infrastructure Controller (separate logical component)

Responsibilities:

- Apply approved, declared infrastructure changes
- Reconcile actual state toward desired state where supported
- Health monitoring and limited automated recovery
- Lifecycle operations allowed by policy
- Service ownership records and audit logging

It must be operable independently of Ember's UI, AI, and workflow engine.
Ember requests operations through a restricted API; Ember does not receive
general infrastructure-administrator privileges.

For the POC, use a very small controller or tightly constrained equivalent. Do
not build a general autonomous operator.

### 3.4 Infrastructure implementation

- Ansible provisions the Raspberry Pi baseline.
- Docker Compose is the proposed POC application runtime.
- Kubernetes is a possible later deployment adapter; do not add it to the POC
  without a concrete requirement.
- Ember application services must run as containers orchestrated by Compose or
  Kubernetes. **Do not create or use systemd units to deploy Ember services.**
- Integration APIs and core business logic must not depend on Compose or
  Kubernetes.

Conceptual view:

```text
 Ember A                         Ember B
 UI + API + integrations         UI + API + integrations
        |                                |
        +---------------+----------------+
                        |
              authenticated API access
                        |
           shared / dedicated services
          (e.g. Paperless, Immich, printer)
                        ^
                        | lifecycle requests only
                        |
             Infrastructure Controller
                        |
            declared / approved state
                        |
                 Ansible + Compose
                        |
                Pi / homelab host
```

**Service use** (application API calls) and **service management** (deploy,
restart, update, delete) are separate permission domains.

## 4. Service deployment and ownership models

Do not assume every service lives inside an Ember deployment or is shared.

| Model | Example | Lifecycle authority |
| --- | --- | --- |
| Shared, infrastructure-owned | Ember A and B connect to one Immich or Paperless | Infrastructure owner/controller; neither Ember can unilaterally delete or upgrade it |
| Dedicated, instance-assigned | Paperless assigned to Ember A | Explicit owner and policy; controller enforces lifecycle rules |
| Externally managed | Existing printer API or cloud service | External operator; Ember uses permitted APIs but does not administer the host |

Distinguish an owner from a consumer. A connection does not imply ownership.
Removing a consumer's connection must never delete the underlying service.

For multiple Ember instances, shared ownership and lifecycle metadata must not
live only in one Ember instance's local configuration. A POC may use a small
controller-owned registry or explicit infrastructure configuration. Avoid
building a distributed control plane prematurely.

Sharing a service does not automatically provide data isolation. User and tenant
separation must be enforced by the downstream service's actual authorization
model. Prefer independent service credentials; do not assume every service
supports true multi-tenancy.

## 5. Autonomy policy

| Level | Example | Proposed POC policy |
| --- | --- | --- |
| Observe | Check whether Paperless is reachable or healthy | Safe to automate |
| Recover | Restart a failed, explicitly managed container within limits | May be automatic only if explicitly allowed by policy; demonstrate a bounded operation |
| Change | Upgrade Paperless, create storage, install a service, delete data | Explicit approval and a declared change; autonomous execution is out of scope |

An AI assistant may propose a deployment change, but it must not execute
arbitrary privileged commands. A future flow could be:

1. User asks Ember to install Immich.
2. Ember/AI creates a versioned proposal and reports its effects.
3. Validation checks inputs, security, resources, and ownership.
4. An authorized human or policy approves the change.
5. The controller applies the explicit, allowlisted change.
6. Ember separately registers an integration connection to the resulting
   service.

Proposal generation is a future design target, not a requirement for the first
POC.

## 6. Recommended POC scope

### Demonstrate

- Reproducible machine setup: Ansible provisions a supported Raspberry Pi Linux
  installation; document prerequisites and idempotent runs.
- Independent Ember instances: Compose runs Ember A and Ember B with separate
  configuration and identity.
- One real integration: both instances connect to one shared service through
  the same reusable adapter. Select a service based on hardware and resources;
  Paperless is a candidate, not a requirement.
- Connection model: endpoint and credentials are per-instance; adapter code
  does not hardcode a hostname or assume one service instance.
- Permission boundary: using a service does not grant lifecycle authority.
- Constrained lifecycle action: an authenticated, authorized request performs
  one allowlisted operation (for example, restart an explicitly managed shared
  container) and records an audit entry. Disconnecting must not delete it.
- Failure behavior: Ember reports service unavailability without crashing;
  recovery does not erase data.
- Documentation: explain how to move services to another host by changing
  connection configuration, not rewriting the adapter.

### Explicitly defer

- Kubernetes deployment
- General-purpose plugin marketplace or execution of untrusted plugins
- Automatic service discovery across a network
- AI-generated arbitrary Docker/Ansible commands or self-rewriting deployment
  logic
- Fully autonomous install, upgrade, or delete operations
- Cross-instance identity federation and guaranteed tenant isolation
- Distributed controller consensus, scheduling, or high availability
- Complete integrations for every planned application, printer, CAD tool, or
  workflow

If a selected service is too resource-intensive for the Pi, use a lighter real
HTTP service while preserving the architecture, and record the deviation.

## 7. Suggested minimal contracts

These are suggestions, not settled API decisions. Adapt names to the repository.

### Integration definition

```yaml
id: paperless
version: 0.1.0
configSchema:
  required: [baseUrl, credentialsRef]
capabilities:
  - health.read
  - documents.list
```

### Ember-owned connection

```yaml
id: documents-main
integrationId: paperless
baseUrl: http://shared-paperless:8000
credentialsRef: secret://ember-a/paperless
```

Ember B should have a separate connection and secret reference even if it uses
the same endpoint.

### Controller-owned service registry record

```yaml
serviceId: paperless-shared
managementMode: managed
ownership: infrastructure
runtime: compose
consumers:
  - ember-a
  - ember-b
allowedOperations:
  - health.read
  - restart.request
```

Consumer IDs alone are not credentials or authorization.

### Illustrative controller API

```text
GET  /managed-services/{serviceId}/health
POST /managed-services/{serviceId}/operations/restart
GET  /operations/{operationId}
```

Security requirements:

- Authenticate client identity and user/service principal as appropriate.
- Authorize against the controller's service registry and policy.
- Allowlist service IDs and operations; never accept arbitrary shell strings or
  filesystem paths.
- Audit requester, target, action, time, result, and correlation ID.
- Rate-limit retries/restarts and prevent conflicting concurrent operations.
- Never expose the container-runtime socket, host shell, or Ansible privilege
  escalation directly to Ember's general plugin API.

A small local implementation is acceptable for a POC only if the trust
boundary is real and testable.

## 8. Repository organization

Use the current repository as the source of truth; adapt rather than creating a
parallel structure:

```text
ember/
├── apps/
│   ├── frontend/
│   └── api/
├── services/
│   ├── ai-worker/
│   ├── model-adapter/
│   └── cad-worker/
├── integrations/             # Add when the first adapter is implemented
├── contracts/
│   ├── openapi.yaml
│   └── project-schema/
├── deploy/
│   ├── compose.yaml
│   └── kubernetes/           # Later, only if justified
├── infra/
│   └── ansible/
├── docs/
│   ├── architecture/
│   ├── decisions/
│   └── operations/
└── EMBER_POC_IMPLEMENTATION_BRIEF.md
```

Do not create empty speculative services just to match a diagram. Add runnable
code and tests when a component is selected.

## 9. Implementation sequence

### Phase 0 — Inspect and decide

Inspect the existing code and deployment conventions. Identify what can be
reused, record assumptions and incompatibilities, and agree on a small plan
before major edits.

### Phase 1 — Host provisioning

Ansible provisions a clean Pi with a supported container runtime, required
directories, network policy, and service accounts. Verify a second Ansible run
is idempotent for intended managed resources.

### Phase 2 — Two Ember instances and one service

Compose starts Ember A, Ember B, and the selected shared service. Give each
Ember instance independent configuration and credentials. Add persistent
volumes where required by the shared service.

### Phase 3 — One reusable integration

Implement one service-type adapter and two concrete connection instances.
Verify the same adapter implementation connects both Ember instances to the
shared service.

### Phase 4 — Controller boundary

Implement read-only health plus one allowlisted lifecycle operation with
authorization, policy, auditing, and safe failure behavior. Verify Ember cannot
bypass the controller.

### Phase 5 — Verification and handoff

Run acceptance tests. Document architecture, operation, recovery, known
limitations, and any deviations from this brief.

## 10. Acceptance criteria

- A documented Ansible command provisions the target from a known baseline.
- Re-running provisioning is idempotent for intended managed resources.
- Ember A and B start independently and retain distinct settings/credentials.
- Both use one shared service through one reusable integration implementation.
- Changing or removing Ember A's connection does not alter Ember B or delete the
  shared service.
- The integration supports multiple connection records without cloning the
  adapter.
- An unauthorized lifecycle request is rejected and logged.
- An authorized health request works; the one allowlisted restart is audited.
- Unknown service IDs or operations cannot trigger arbitrary container actions.
- Shared service data persists across a controlled restart/redeployment.
- Service unavailability is shown as degraded state and does not crash Ember.
- An Ember instance can restart without redeploying the shared service.
- Documentation explains how to relocate services by changing connection
  configuration rather than rewriting adapters.
- Ember application workloads are run with Compose in this POC. No systemd
  service units are installed or used.

## 11. Engineering and safety constraints

- Pin container image versions/digests where practical; avoid mutable `latest`.
- Treat repository code, Compose files, and Ansible roles as reviewable source
  of truth.
- Keep secrets out of Git, logs, and API responses. Use examples with no real
  values.
- Grant containers minimum permissions and network access; never mount the
  container-runtime socket into Ember Core.
- Avoid default public exposure of service APIs. Document ingress/TLS before
  remote access.
- Document backup and restore for every persistent service introduced.
- Include smoke tests and negative security tests, not only happy-path tests.
- Prefer small composable components; avoid enterprise infrastructure without
  a concrete POC need.

## 12. Open decisions

Do not silently treat these as settled:

- Which existing code from the Ubuntu/Claude dashboard should be reused?
- Exact Pi model, RAM, OS, architecture, network, disk, and attached hardware?
- Which shared service should be used for the first real integration?
- Should the controller be a separate container on the Pi or hosted elsewhere?
  Keep it a separate trust boundary either way.
- What authentication, identity, and secret-storage mechanism is appropriate?
- Is frontend delivery a browser UI, PWA, native wrapper, or combination?
- Is desired state Git-tracked, controller-local, or both, and how are changes
  approved?
- Are dedicated, instance-assigned managed services in the first POC or later?
- What language/framework should the POC use for new components?

When information is missing, choose the simplest defensible POC option, document
it, and avoid irreversible coupling.

## 13. Instructions to the implementation agent

**Goal:** Implement the smallest working system that proves independently
configured Ember instances can safely use a shared integration while
infrastructure lifecycle operations remain separately controlled and
reproducible.

1. Inspect the repository and existing deployment/code conventions first.
2. Write down selected scope, assumptions, and a small implementation plan.
3. Prefer working code, tests, runnable Compose/Ansible configuration, and
   operator instructions over speculative abstractions.
4. Preserve the distinction between integration definition, concrete
   connection, service ownership, and lifecycle controller.
5. Enforce restrictions in code and process permissions; UI-only restrictions
   are insufficient.
6. Do not give an AI agent uncontrolled infrastructure execution permissions.
7. Do not add systemd deployment; use Compose for the POC. Kubernetes can be
   evaluated later if a real orchestration need arises.
8. Run available tests and report exactly what was verified and what requires
   Pi hardware or external infrastructure.
9. At completion, report changed files, setup/run instructions, verification
   results, limitations, and next steps.

## 14. Discussion summary: goals versus recommendations

### User-stated goals and concerns

- Ember should be one interface for many applications and workflows.
- Where integrations live and whether services are shared is not yet known.
- A self-maintaining AI vision is attractive for flexibility, but stability and
  reproducibility matter.
- The POC should run on a Raspberry Pi and generalize to a larger homelab.
- Ember application services should be deployed with Compose or Kubernetes, not
  systemd.

### Proposed architecture, still requiring validation

- Option C: flexible integrations with optional, independently governed service
  management.
- Separate Ember Platform and Infrastructure Controller logical boundaries.
- Shared, dedicated, and external service ownership models.
- Ansible and Compose for the POC; Kubernetes only when justified.
- Bounded recovery operations and approval for infrastructure-changing actions.
- Two Ember instances, one shared service, one reusable adapter, and one
  constrained controller operation as a strong POC demonstration.

No final integration target, authentication solution, framework, controller
hosting model, or service ownership policy has been selected.

## Definition of success

The POC succeeds when two independently configured Ember instances can use one
shared service through a reusable integration, while an independently
authorized infrastructure layer owns lifecycle decisions and Ansible/Compose
can reproduce the setup.
