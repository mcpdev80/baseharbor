# BaseHarbor architecture

BaseHarbor is a secure, modular, self-hosted application platform foundation. It manages backend infrastructure and application lifecycle for independent applications without forcing those applications into a BaseHarbor-specific SDK.

The long-term lifecycle target is continuous growth from local development and homelab deployments to stricter production environments and, where needed, Kubernetes/OpenShift enterprise deployments without redefining the logical application contract.

## Product boundary

BaseHarbor is developer-first, but it is not a source-code build service or proprietary application framework. Applications keep their own source, business logic and native ecosystem interfaces. BaseHarbor owns the operational realization around them: managed dependencies, lifecycle, isolation, security boundaries, recovery and provider-specific deployment mechanics.

```text
Application contract
        |
        v
BaseHarbor lifecycle / policy / provisioning
        |
        +-- Compose (current complete provider)
        +-- Kubernetes (future provider)
        +-- OpenShift (future enterprise provider)
```

The application should continue to consume standard interfaces such as PostgreSQL, Redis/Valkey, S3, OIDC/OAuth2, OpenBao/Vault-compatible secrets and OpenTelemetry/OpenMetrics rather than BaseHarbor-specific data protocols.

## Capability versus product

The portable application contract describes capabilities, not infrastructure product choices. Provider selection belongs to the environment/platform side.

```text
Application Contract
        ↓
Capabilities
        ↓
Environment / Policy
        ↓
Provider selection
        ↓
Concrete product/runtime
```

For example, an application may require a SQL database, S3-compatible object storage and secrets without depending on whether an environment realizes them through PostgreSQL, SeaweedFS and OpenBao or through customer-managed PostgreSQL, Ceph RGW and Vault.

This is a hard architecture rule. Every bundled/default component must have a provider boundary and a documented replacement path. See [Capability and provider model](capability-provider-model.md) and ADR [0005](decisions/0005-capabilities-not-products.md).

## Shared core and control surfaces

BaseHarbor is designed as **one shared application/lifecycle core with multiple control surfaces**.

```text
                         BaseHarbor Core
              +--------------------------------+
              | PortableContract                |
              | input resolution                |
              | plan / preflight / apply        |
              | verify / status / diagnostics   |
              | recovery / update semantics     |
              | provider selection/capabilities |
              +---------------+----------------+
                              |
          +-------------------+-------------------+
          |                   |                   |
          v                   v                   v
       baha CLI            HTTP API        Operator controllers
                              |
                              v
                       lightweight Web UI
```

`baha` remains the primary local developer/operator interface. Future HTTP API, Web UI and Kubernetes/OpenShift Operator surfaces must reuse the same domain models and lifecycle semantics rather than reimplementing them.

The Web UI is intentionally a thin client over the BaseHarbor API. It must not shell out to `baha`, bypass lifecycle validation or implement its own readiness/security rules.

A future Kubernetes/OpenShift Operator reconciles BaseHarbor desired state through the same application/provider model. It must use idempotent reconciliation and provider-native observation rather than wrapping imperative CLI commands.

Lifecycle results should be machine-readable before presentation. CLI output, API responses, Web UI views and Operator status/conditions are different renderings of the same underlying state and verification results.

v0.4.1 introduces this foundation for application capabilities as typed capability/provider/resource/binding models plus structured lifecycle steps and diagnostics. The reusable capability lifecycle is non-interactive and fail-closed; prompting remains a control-surface concern.

v0.4.2 adds provider registry, placement and lifecycle ownership as protected operator state. Shared, application-scoped and external provider placement therefore remains outside portable application intent.

v0.4.3 moves repository understanding into a shared read-only core. CLI inspection and guided application initialization consume the same deterministic detector engine, with explicit Detected/Suggested/Possible confidence and machine-readable results for future control surfaces.

Provider Integration Contract v1 establishes the mandatory boundary for all subsequent capability providers. BaseHarbor owns versioned capability semantics; built-in/reference providers implement them through the shared driver/registry model, and future external providers will adapt to the same semantics through a language-neutral gRPC/Protocol Buffers boundary. OCI is the distribution direction for future external provider packages. See [Provider Integration Contract v1](provider-integration-contract.md).

See ADR [0009](decisions/0009-shared-core-multiple-control-surfaces.md).

## Control plane and application stacks

The BaseHarbor control plane is shared. Application data-plane resources are isolated by default.

```text
BaseHarbor control plane
├── baha CLI
├── platform metadata
├── platform secrets / trust bootstrap
├── lifecycle and convergence engine
└── shared observability infrastructure

Application: mailflow
├── isolated network
├── dedicated PostgreSQL
├── dedicated Redis/Valkey
├── isolated secret scope or optional dedicated secret service
└── application-owned data
```

A dedicated application service does not imply a dedicated physical host. On Compose it normally means separate containers, networks and volumes. Later deployment engines can map the same logical desired state to other runtimes.

## Identity versus deployment context

`app.name` is the stable logical application identity. `app.environment` is deployment context. The same application can exist simultaneously in development, test, staging, production and customer-specific environments.

Provider/runtime identities may include the environment to preserve isolation, but applications must not depend on those generated names.

## Portable contract versus deployment/runtime state

The repository-owned `baseharbor.yaml` Manifest v1 remains the desired-state compatibility source. v0.4 translates its portable application intent into `PortableContract`, while provider-specific compatibility fields stay outside that provider-neutral view.

Compose-specific deployment realization is stored separately in protected BaseHarbor runtime state. In v0.4 this protected deployment state includes details such as:

- selected/public FQDN used for local HTTP Host and TLS ServerName verification;
- TLS mode for the current repository deployment;
- normalized existing/BYOC certificate and key material under owner-only BaseHarbor state;
- automatically selected workload host-port fallbacks;
- generated Compose overrides and runtime identity material.

These values are operational realization, not portable application requirements. They must not be promoted into `PortableContract` merely because Compose currently needs them.

## Principles

1. One shared application/lifecycle core with multiple control surfaces; `baha` is the primary local interface, while future API/Web UI and Operator surfaces reuse the same domain behavior.
2. Applications remain independent and keep all business/domain logic.
3. Application contracts describe capabilities, not concrete infrastructure products.
4. Provider selection is environment/platform-owned and replaceable.
5. Every default BaseHarbor component has a defined provider boundary and documented replacement path.
6. Native protocols are preferred where they already provide a stable ecosystem boundary.
7. Secure defaults, least privilege, deny by default and fail closed.
8. Mature open-source components are composed instead of reimplemented.
9. Desired state is validated before mutation and verified after mutation.
10. Container-running is not equivalent to service-ready.
11. Secrets never belong in application manifests.
12. Compose is the complete current provider and remains first-class; future Kubernetes/OpenShift providers must preserve logical application requirements rather than redefine them.
13. Environment/risk policy and deployment topology are separate concepts.
14. Observability is integrated through open standards rather than a proprietary telemetry stack.
15. CLI, HTTP API, Web UI and Operator are adapters over shared domain/lifecycle services; business logic must not be duplicated in presentation layers.
16. Every capability provider implements a versioned BaseHarbor Capability Specification and shared provider lifecycle; product-specific one-off lifecycle paths are not allowed.

## Application lifecycle model

BaseHarbor follows an explicit convergence flow:

```text
Manifest
   ↓
Validate
   ↓
Resolve dependencies
   ↓
Preflight
   ↓
Build desired-state plan
   ↓
Mutate through current provider
   ↓
Verify actual state
   ↓
Ready / Failed truthfully
```

Manifest v1 remains intentionally small and Compose-oriented as a compatibility surface, while v0.4 translates its portable intent through `PortableContract`:

```yaml
version: 1
app:
  name: demo
  environment: dev
services:
  postgres:
    enabled: true
  redis:
    enabled: false
  secrets:
    enabled: false
```

An explicit repository Compose workload may also be a valid application shape without inventing an unused managed PostgreSQL or Valkey dependency. Empty manifests still fail validation, and managed-secrets-only applications remain unsupported where the current runtime broker requires a materialized managed backend.

Provider-specific details such as Compose project names, networks, host ports, generated overrides, Kubernetes resource names or OpenShift Routes are implementation details, not portable application requirements.

The product-oriented field names that exist in Manifest v1 are the current pre-v1 Compose contract; they do not override the capability/provider rule for future contract evolution.

## Runtime truth and verification

BaseHarbor v0.4 continues to treat runtime truth as more than container state:

- PostgreSQL and Valkey use real protocol verification;
- selected Compose workload services distinguish running/healthy, starting, unhealthy, exited and missing states;
- conventional application-owned HTTP/HTTPS publishers are actively probed on their locally published host ports;
- redirects count as reachable exposure; 5xx or unreachable endpoints are NOT READY;
- hostname-bound local HTTPS probes use the configured public FQDN for HTTP Host/TLS ServerName while still dialing the local published socket;
- restore and update paths report success only after the relevant post-mutation readiness boundary succeeds.

`baha app show`, `status` and `doctor` reuse this truth rather than inventing separate optimistic status models.

## Recovery and update boundaries

Backup is supported together with restore, not as an isolated archive feature. Restore validates before destructive work, keeps workloads stopped during uncertain recovery and reports READY only after restored state, runtime identity, managed backends and application workload/exposure have been verified.

Application source update is strict fast-forward only. BaseHarbor does not reset, stash, rebase, merge divergent history or discard local work. Durable applications require an explicit recovery policy before mutation. BaseHarbor self-update verifies release artifacts, replaces the CLI atomically and retains a recovery binary for rollback if post-update verification fails.

## TLS boundary in v0.4

v0.4 retains the existing/BYOC certificate lifecycle for repository Compose deployments, including certificate/key/FQDN validation, downgrade prevention, protected installation, workload restart and readiness verification.

It does **not** introduce a provider-neutral `tls.certificate` manifest capability, BaseHarbor-managed ACME issuance, OpenBao PKI issuance, automatic certificate rotation, cert-manager integration or Kubernetes/OpenShift ingress realization. Those remain future provider/capability work.

## `baha` as the primary local interface

`baha` is not a thin wrapper around Compose. It is the stable primary local operator/developer interface for BaseHarbor lifecycle, diagnostics, recovery, updates and application resources. Compose is the current implementation target behind that interface.

This does not make the CLI the permanent home of BaseHarbor business logic. Shared lifecycle, status, diagnostics, input-resolution and policy behavior belongs below the CLI so future API/Web UI and Operator surfaces can expose the same semantics.

## Security and operations direction

Implemented in the current Compose line:

- OpenBao-backed managed application secrets;
- scoped runtime identity and mTLS broker isolation;
- verified encrypted backup/restore;
- health/readiness/doctor semantics including application-owned HTTP/TLS exposure;
- guarded Git-backed application updates and BaseHarbor self-update recovery;
- existing/BYOC certificate validation and lifecycle for repository deployment state.

Future capabilities include:

- environment-aware managed access policy;
- OIDC/OAuth2 identity and authorization for managed/enterprise deployments;
- provider-neutral ingress/exposure capability realization;
- BaseHarbor-managed ACME and/or OpenBao PKI issuance/rotation;
- lifecycle audit/retention policy beyond current protected operational metadata;
- broader metrics, logs and traces through OpenTelemetry/OpenMetrics-compatible pipelines;
- standard and HA deployment profiles without changing logical resource identity;
- Kubernetes and OpenShift runtime providers.

## Non-goals

BaseHarbor does not aim to implement its own database, cache protocol, secret store, OIDC protocol, S3 protocol, ACME server or monitoring database. It also does not own application schemas or business data models.


## Endpoint and exposure foundation in v0.4.4

v0.4.4 extracts HTTP/HTTPS endpoint identity and readiness into a shared endpoint core. Logical endpoint identity uses workload service names and protocol/address metadata; generated Compose container names are never the application-facing identity.

Endpoint discovery does not imply exposure ownership:

```text
application-owned endpoint
  -> discover
  -> observe
  -> verify

explicit exposure.http/v1
  -> resolve provider
  -> preflight all
  -> provision
  -> bind logical endpoint
  -> verify end-to-end
```

The first managed Compose exposure provider is Caddy. It owns a separate application-scoped proxy project, while the generated workload override owns the stable exposure integration network and attaches only explicitly exposed workload services. Caddy consumes that network externally. It does not rewrite the application's Compose source and does not own application volumes.

The same `exposure.http/v1` intent is designed to map later to other providers without changing the application contract.

## Secure binding foundation in v0.4.5

v0.4.5 adds a shared security-binding domain below CLI/runtime-specific code. Capability bindings can now carry provider-neutral workload identity, credential references, trust references, authorization metadata, secret references, lifecycle support and security diagnostics.

The existing Compose/OpenBao path remains authoritative. Its client certificate already uses the SPIFFE subject `spiffe://baseharbor/apps/<application>/<environment>`; v0.4.5 exposes that stable identity semantically without exposing certificate paths, AppRoles, SecretIDs, policies or secret values.

The lifecycle validates secure-binding metadata during plan construction before any provider preflight or mutation. This keeps later SQL, S3, messaging, vector, AI and MCP providers on one security boundary instead of creating provider-specific credential plumbing.

Human OIDC/RBAC/MFA/JIT/breakglass remains a separate v0.6 platform-access concern. Cross-provider rotation completion remains later lifecycle work.


v0.4.6 adds the first provider-neutral S3 object-storage implementation on the same shared lifecycle and secure-binding foundations. Logical buckets resolve to `object-storage.s3/v1`; SeaweedFS is a lazy shared Compose reference provider rather than application identity. Provider state owns physical bucket/IAM/topology details, while application-facing readiness is verified through an authenticated SigV4 Put/Get flow.

## OTLP telemetry foundation in v0.4.7

v0.4.7 adds `telemetry.otlp/v1` as a provider-neutral transport capability. OpenTelemetry is the ecosystem and instrumentation model; OTLP is the portable protocol boundary. The OpenTelemetry Collector is only the first managed Compose reference provider.

Applications declare the telemetry signals they export and keep using standard OpenTelemetry configuration:

```text
OTEL_EXPORTER_OTLP_ENDPOINT
OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf
OTEL_SERVICE_NAME
OTEL_RESOURCE_ATTRIBUTES
```

Managed Collector placement is lazy/shared. External OTLP destinations use the same capability and remain externally lifecycle-owned. BaseHarbor verifies a real OTLP HTTP/protobuf export rather than only checking that a collector process is running.

Common resource identity includes standard OpenTelemetry service/environment attributes plus BaseHarbor application, logical telemetry resource and provider attribution. OTLP transport does not imply Prometheus, Loki, Tempo, Grafana or another observability backend; those remain independent platform/provider concerns in later releases.

## Metrics collection and Prometheus provider in v0.4.8

Metrics are split into three independent concerns:

```text
application-provided signal source
        !=
deployment collection policy
        !=
metrics provider implementation
```

`metrics/v1` describes an application-provided OpenMetrics-compatible HTTP source. The manifest identifies the logical source, workload service, target port and path. It does not name Prometheus or another backend.

The current Compose policy enables collection by default only for development environments. Test/staging/production require explicit operator opt-in. Policy is resolved before provider mutation.

Prometheus 3.14.0 is the first shared BaseHarbor-owned Compose provider. It uses file-based service discovery generated from protected BaseHarbor state; operators do not edit scrape targets manually. Only declared source services join the internal `baseharbor-metrics` network. Every app/environment/service receives a deterministic collision-resistant DNS alias so identical Compose service names across applications do not collide.

Target labels include BaseHarbor application, environment, workload service and logical source identity. Readiness requires a real successful scrape visible in Prometheus as `up=1`, not merely a running Prometheus process.

The provider has no Docker/Podman socket. Its API is loopback-published only for local lifecycle verification/querying. Grafana, Loki and Tempo remain separate provider tracks and are never provisioned as side effects of metrics collection.

## Continuous application evolution foundation

BaseHarbor treats application intent as a continuously reconcilable desired state.

```text
source repository
      |
      v
read-only inspection
      |
      v
typed capability evidence
      |
      v
compare with explicit contract
      |
      +-- satisfied
      +-- new
      +-- ambiguous
      +-- stale (informational only)
      |
      v
minimal explicit contract delta
```

The model supports four lifecycle moments with one capability architecture:

1. **bootstrap** — understand a new repository and establish initial intent;
2. **evolution** — detect capability additions as the codebase changes;
3. **runtime** — allow explicitly authorized application-time resource operations where a capability supports them;
4. **retirement** — remove capability intent only through an explicit developer/operator decision.

Capability evidence includes direction (`consume`, `provide`, `export`, `receive`, `provision`) and may include runtime-operation hints such as `runtime.create`. Detection is never authorization.

Deployment-time and application-time resources use the same logical capability/provider boundary. An authorized application-time request to create an S3 resource resolves through `object-storage.s3/v1`; it does not introduce a SeaweedFS-, AWS- or Ceph-specific lifecycle.

The first real runtime mutation path uses a shared Runtime Provider Executor:

```text
authorized workload
    -> Application Runtime Broker
    -> mTLS/SPIFFE Runtime Provider Executor
    -> selected S3 provider / IAM API
```

The application and per-app broker have no Docker/Podman socket and no provider-global administrator credential. The shared executor also has no container-runtime socket or host-published port. Provider mutation stays behind the executor boundary, while the application receives only its resource-scoped binding.

Repository inspection already provides the first concrete evidence for this model:

- PostgreSQL and Redis/Valkey consumption;
- S3-compatible object-storage consumption and likely runtime bucket creation;
- application-provided OpenMetrics `/metrics`;
- OTLP export.

Repository-first `baha up` reuses this reconciliation path and reports newly detected/ambiguous capability changes and runtime-operation hints before normal convergence, without mutating the contract.

This foundation intentionally does not add the public runtime resource API yet. It defines the semantics that such an API must reuse. See ADR 0010.

## Provider placement, sharing boundaries and runtime isolation

Provider placement is a BaseHarbor-wide deployment/operator concern. It is independent from application intent, runtime topology and product choice.

```text
Application intent
        |
        v
Capability
        |
        v
Provider resolution
        |
        v
Provider placement
   +----+------------------+
   |                       |
application             shared ---------------- external
                           |
                           +-- optional sharing boundary
        |
        v
Isolation / deployment boundary
        |
        v
Runtime/provider implementation
```

The canonical placement scopes remain exactly `application`, `shared` and `external`. A sharing boundary is an optional property of `shared`; it is not a fourth scope.

A shared provider is never automatically reachable by every application. Access is explicit, least-privilege and deny-by-default. A sharing boundary allows an operator to intentionally reuse one provider instance for a selected set of applications while keeping unrelated applications outside that trust boundary.

Provider implementations declare the placements they support. If policy resolves to a placement that the selected provider cannot satisfy, BaseHarbor fails closed before mutation instead of silently changing placement.

The portable application contract never contains provider placement, sharing-boundary, lifecycle-ownership or runtime-isolation mechanics. The developer continues to state only application capabilities. BaseHarbor and deployment policy resolve the infrastructure details.

Placement must also remain independent from runtime-specific isolation. Today Compose may realize boundaries through projects, networks and volumes. Future Kubernetes/OpenShift runtimes may map them to namespaces/projects, cluster-scoped infrastructure, Helm releases, Operators, NetworkPolicies or other platform-native mechanisms without changing application intent.

A future Operator's installation scope is not the same thing as provider placement or resource scope. A cluster-scoped Operator may legitimately manage application-scoped or sharing-boundary-scoped resources.

Multiple BaseHarbor installations are therefore not required merely to isolate groups of applications that share selected providers. Separate BaseHarbor control planes are reserved for genuine administrative, trust-domain, infrastructure or compliance boundaries.

Current implementation scope remains Docker/Podman Compose. Kubernetes/OpenShift mappings described here are architectural compatibility requirements only, not implemented runtime behavior.

## Progressive disclosure and explicit operator control

BaseHarbor must be simple by default without becoming restrictive.

The normal developer path should require only application intent and should use safe, explainable defaults:

```text
developer declares capability
        |
        v
BaseHarbor detects/resolves sensible defaults
        |
        v
plan -> preflight -> apply -> verify
```

Advanced users and operators must still be able to override deployment decisions explicitly where the platform supports them, including provider selection, provider placement, optional sharing boundary, lifecycle ownership where applicable, external provider references, isolation/deployment policy and supported provider/runtime options.

The control model is therefore progressive disclosure:

```text
simple path
  -> automatic safe defaults

advanced path
  -> explicit deployment/operator policy

expert path
  -> fully specified supported provider/runtime realization
```

Explicit control must not require polluting the portable application contract with infrastructure details. Portable application intent remains product-neutral; concrete infrastructure choices belong to deployment/operator configuration and control surfaces.

BaseHarbor must show the resolved plan before mutation so users can see what defaults were selected and can override supported decisions deliberately. Explicit user/operator configuration wins over defaults, but never bypasses capability conformance, security boundaries, validation or fail-closed behavior.

The goal is: easy when the user does not care about infrastructure details, precise when the user does.

## Convention by default, configuration by choice

BaseHarbor follows one UX and architecture principle across all capabilities and runtimes:

> **Convention by default, configuration by choice.**

The default path minimizes decisions. BaseHarbor detects what it can, chooses safe and explainable defaults, shows the resolved plan and proceeds through the normal validation lifecycle.

Users who want more control may progressively override supported deployment decisions without changing portable application intent.

```text
default
  -> capabilities only
  -> safe automatic provider/placement/runtime defaults

advanced
  -> explicit provider / placement / sharing / external references

expert
  -> supported naming, topology, runtime and provider realization hints
```

Examples of optional expert control may include stable resource prefixes, logical hostnames, Compose project/network/volume names, DNS aliases and later Kubernetes/OpenShift namespace/project naming. Ephemeral runtime-generated identities such as replica or Pod instance names remain runtime-owned unless the runtime explicitly supports a safe stable override.

Every configurable field must have explicit semantics:

- stable and safely overridable;
- hint/template only;
- generated/runtime-owned and not overridable.

Overrides are accepted only when the active runtime/provider can honor them safely and deterministically. They must never bypass security, ownership, reconciliation, conformance or fail-closed validation.

The simple path and expert path must use the same core model. Advanced flexibility must not create a second application contract or parallel lifecycle implementation.
