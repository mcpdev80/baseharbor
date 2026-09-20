# Provider Integration Contract v1

BaseHarbor standardizes **application capability semantics and provider lifecycle**, not infrastructure products.

The long-term compatibility promise is:

> A provider can be implemented by BaseHarbor, the community or a product vendor without changing the portable application contract.

## Architecture

```text
Application intent
      |
      v
Versioned Capability Specification
      |
      v
Provider Registry / selection
      |
      v
Provider Integration Contract v1
      |
      +-- built-in/reference driver
      |
      +-- future external driver
              |
              +-- gRPC / Protocol Buffers
                      |
                      +-- vendor/community provider
```

BaseHarbor owns the hard semantics. A provider may declare that it implements a BaseHarbor capability specification; it may not redefine that specification.

## Public/open building blocks

BaseHarbor reuses open standards for infrastructure plumbing where they fit:

- **gRPC + Protocol Buffers**: language-neutral external provider API transport.
- **OCI Image/Distribution specifications**: registry-neutral provider packaging and distribution.
- **JSON Schema 2020-12**: provider configuration schema declaration and validation.
- **Open Service Broker API concepts**: useful lifecycle concepts around catalog/provision/update/bind/unbind/deprovision.
- **Service Binding concepts**: useful representation patterns for application-facing bindings.

These building blocks do not replace BaseHarbor's own capability, lifecycle, ownership, security or verification rules.

The public compatibility contract must not depend on HashiCorp go-plugin, Kubernetes, Docker, GitHub, a specific cloud provider or a BaseHarbor-operated registry.

## GraphQL boundary

GraphQL is **not** the BaseHarbor provider lifecycle protocol.

GraphQL is optimized for client-driven data querying and schema evolution. Provider lifecycle needs strongly defined commands, deadlines, cancellation, transport status, idempotency and long-running operation semantics. The external provider boundary therefore remains gRPC/Protocol Buffers.

GraphQL may be evaluated later for a user-facing/control-plane query API or Web UI backend, but it must remain an adapter over the same BaseHarbor core and must not become a second provider lifecycle implementation.

## Version axes

Provider protocol and capability specifications are versioned independently.

Example:

```text
provider protocol:         baseharbor.provider/v1
capability specification: database.sql/v1
```

A protocol version defines how BaseHarbor and a provider exchange lifecycle requests/results.

A capability specification defines what application-facing behavior a provider must satisfy.

Breaking protocol semantics require a new provider protocol major version.

Breaking application-facing capability semantics require a new capability specification major version.

## Required lifecycle

The existing BaseHarbor lifecycle remains authoritative:

```text
resolve
  -> preflight all resources
  -> provision/apply
  -> bind
  -> verify
```

The Provider Integration Contract adds the complete vocabulary required for future provider implementations:

```text
Describe
Preflight
Provision
Bind
Verify
Status
Update
Backup
Restore
Destroy
```

`Describe`, `Preflight`, `Provision`, `Bind` and `Verify` form the required v1 integration foundation.

Status/update/backup/restore/destroy support is declared explicitly. Unsupported optional operations must be reported as unsupported, never silently treated as success.

## Built-in providers

Current BaseHarbor providers remain compiled into BaseHarbor and keep their existing proven provisioning paths.

They are nevertheless described through the same semantic contract:

| Provider | Capability specification |
| --- | --- |
| PostgreSQL | `database.sql/v1` |
| Valkey | `cache.key-value/v1` |
| OpenBao | `secrets/v1` |
| Caddy | `exposure.http/v1` |

New reference providers such as S3, OTLP, Prometheus, Loki, Tempo, Grafana, messaging, AgentGateway, MCP and vector search must follow the same boundary.

Do not add product-specific lifecycle semantics directly to the portable application contract or CLI orchestration.

## Driver boundary

The current Go `Driver` remains the internal lifecycle interface.

`IntegrationDriver` extends that interface with a versioned `IntegrationDescriptor`.

A `DriverAdapter` allows existing built-in drivers to participate without creating a second workflow engine.

Future external-provider transport must adapt into this same semantic interface rather than introduce a parallel lifecycle.

## External protocol

The repository contains the versioned schema:

```text
spec/provider/v1/provider.proto
```

It defines the future language-neutral gRPC boundary.

The schema is part of the contract foundation; v1 of this architecture prerequisite does **not** yet:

- start external provider processes;
- download OCI modules;
- implement a gRPC server/client in the BaseHarbor runtime;
- execute untrusted provider code;
- provide vendor SDKs.

Those are deliberately deferred until real provider implementations have further proven the contract.

## RPC reliability and asynchronous operations

External provider RPCs must follow normal distributed-systems safety rules:

- every call has an explicit deadline; BaseHarbor must never wait indefinitely;
- cancellation is propagated and providers must stop work when safely possible;
- read-only calls may be retried according to gRPC status/retry policy;
- mutating calls are not blindly retried and require an idempotency key;
- repeating an identical mutation with the same idempotency key must not create duplicate provider resources;
- transport/protocol failures use standard gRPC status codes;
- BaseHarbor domain state and operator-facing diagnostics remain structured response data;
- external providers expose the standard gRPC Health Checking service for transport/service health;
- local external providers should prefer a Unix domain socket rather than an unauthenticated TCP listener;
- remote provider endpoints require TLS and should use mTLS or equivalent workload identity where practical.

Provision, bind, unbind, update, backup, restore and destroy may be long-running. They therefore return a stable operation identity which BaseHarbor can poll through `GetOperation`; cancellation is best-effort through `CancelOperation`.

A provider may complete an operation immediately, but the API must not require lifecycle work to fit inside one synchronous RPC.

## Protocol Buffer evolution

`baseharbor.provider.v1` follows additive protobuf evolution rules:

- existing field numbers are never changed or reused;
- removed fields and enum values reserve their old numbers and names;
- new v1 fields are additive and optional/forward-compatible;
- enum zero values remain explicit `UNSPECIFIED` states;
- breaking wire or semantic changes require a new provider protocol major version;
- implementation code must tolerate unknown fields and enum values where the language runtime permits it.

## OCI packaging and distribution

Future external providers must be publishable through normal OCI registries such as GHCR, Quay, Harbor, Artifactory and private OCI-compatible registries. BaseHarbor must not require a proprietary account or central BaseHarbor registry.

OCI handling is **digest-first**:

- tags are convenient mutable discovery aliases;
- installation/lock state records and verifies the resolved manifest digest;
- updates resolve a new digest explicitly rather than trusting that a tag is immutable.

For a runnable provider distributed as a container, prefer a normal OCI Image. Multi-platform providers use an OCI Image Index with explicit platform descriptors.

Generic non-container provider packages may use OCI artifact guidance with a BaseHarbor-specific RFC 6838 media type/`artifactType`; large metadata belongs in referenced blobs/config rather than oversized annotations.

OCI `subject` + Referrers are the preferred association mechanism for signatures, SBOMs and provenance. Clients must honor the OCI Distribution compatibility fallback when the Referrers API is unavailable.

Supply-chain policy must support standard verification mechanisms rather than inventing BaseHarbor signatures. Sigstore/cosign or Notation-style signatures and in-toto/SLSA provenance are the preferred open directions. Verification policy decides which identities/issuers/provenance are trusted before a provider is executed.

Provider packages carry or reference:

- provider protocol version;
- implemented capability specifications;
- immutable digest;
- provider metadata/configuration schema;
- platform/runtime metadata where needed;
- signature, SBOM and provenance referrers where available.

## Bindings and secrets

Bindings expose standard application-facing values.

Secret material is not normal provider metadata.

Provider responses, diagnostics, registry state and portable application intent must never contain plaintext passwords, tokens, private keys or credential-bearing URLs.

When a binding needs secret material, the provider contract returns a stable credential/secret reference for BaseHarbor to resolve at a trusted boundary. The protobuf schema uses a `oneof` so a binding is either a non-secret public value or a credential reference, never both.

## Provider configuration schema

Provider-specific operator configuration uses **JSON Schema 2020-12**. Each external provider advertises its schema through `Describe`.

The schema describes operator/deployment configuration only. It must not smuggle provider-specific fields into portable application intent and must not contain secret values. Secret inputs use BaseHarbor credential references.

## Ownership and placement

The v0.4.2 provider registry remains authoritative for:

- `shared` provider instances in the BaseHarbor Platform/Core Runtime;
- `application` provider instances dedicated to exactly one application/environment;
- `external`/BYO providers;
- BaseHarbor lifecycle ownership versus external lifecycle ownership.

These scopes have strict semantics. In the Compose runtime, `application` means a dedicated provider container/project and provider state for that application; it is never reused by another application. `shared` means one BaseHarbor-owned platform provider instance that is created lazily and may serve one or multiple explicitly authorized applications. Consumer count does not change the scope. `external` means the provider lifecycle remains outside BaseHarbor.

External provider compatibility does not grant BaseHarbor permission to mutate an externally owned provider.

Logical resources remain application-owned even when the provider instance is shared. Sharing a provider instance never implies shared credentials, data access or cross-application network connectivity.

## Conformance

A provider compatibility claim requires conformance, not only successful startup.

The initial static conformance layer validates:

- provider protocol version;
- provider identity;
- exact versioned capability declarations;
- consistency between declared capabilities and provider metadata.

Runtime conformance grows with each capability and must test real semantics, for example:

- authenticated SQL query;
- Redis-compatible protocol probe;
- application-isolated secret access;
- S3 put/get;
- OTLP export;
- metrics ingestion/scrape;
- messaging publish/consume;
- provider lifecycle ownership;
- cross-application isolation;
- secret-safe diagnostics.

A future vendor-facing conformance tool can build on the same reports.

## Compatibility

- Manifest v1 remains version 1 and is additively extended with provider-neutral managed HTTP exposure intent.
- Compose remains the complete current runtime.
- Existing PostgreSQL, Valkey and OpenBao provisioning remains authoritative; Caddy is the first managed Compose exposure reference provider.
- No external provider module is required.
- No Kubernetes/OpenShift/cloud implementation is implied.
- The v0.4.1 capability lifecycle and v0.4.2 provider registry remain the only provider domain model.

## Rule for all subsequent providers

Every new capability/provider integration must:

1. define or extend a versioned BaseHarbor Capability Specification;
2. declare that exact specification in its integration descriptor;
3. reuse the shared provider lifecycle/registry model;
4. keep provider-specific details outside portable application intent;
5. add capability-specific conformance tests;
6. preserve the future ability to replace the built-in implementation with a conforming external provider.


## HTTP exposure provider boundary

`exposure.http/v1` is the first traffic capability implemented through this provider contract.

BaseHarbor deliberately keeps two paths distinct:

```text
application-owned publisher -> discover -> observe -> verify
managed exposure intent      -> resolve -> preflight -> provision -> bind -> verify
```

Observation never grants BaseHarbor lifecycle ownership. Only explicit managed exposure intent is registered as an application-scoped provider resource. The current Compose reference provider is Caddy; its host ports, TLS files and generated proxy configuration remain protected provider/deployment state. The stable exposure integration network is workload integration state owned by the generated BaseHarbor workload override and is consumed by the exposure provider as an external network.


### Capability-owned binding parameters

Provider protocol v1 carries capability-owned, provider-neutral binding semantics through preflight/provision/bind. For `exposure.http/v1`, the typed binding contains the logical workload service, target port, HTTP/HTTPS transport and public/internal visibility.

These fields are defined by the capability specification, not by Caddy or another provider. Provider-specific configuration remains separate operator configuration. This allows a future conforming provider to receive the same application intent without reading `baseharbor.yaml` or depending on BaseHarbor's Go implementation.

## Secure binding semantics

Starting with v0.4.5, provider lifecycle bindings may include the shared `secure-binding/v1` model. This is the single provider-neutral representation for workload identity, credential references, trust material references, authorization metadata, secret references and declared renewal/rotation/revocation support.

Rules:

- plaintext credentials, tokens, private keys and secret values are forbidden in the provider protocol;
- provider-specific security internals such as OpenBao paths/AppRoles/policies, Kubernetes Secret names or cloud-secret object identifiers stay provider/deployment state;
- secure-binding metadata is validated before provider preflight and therefore before mutation;
- shared provider infrastructure never implies shared workload authorization;
- providers translate least-privilege authorization metadata into their native ACL/policy model;
- security diagnostics are machine-readable and must remain secret-safe;
- the same semantics are represented by `WorkloadBinding.security` in `baseharbor.provider/v1`.

The existing OpenBao/runtime-broker/mTLS implementation is the first reference realization of this contract. v0.4.5 extracts its stable semantics; it does not replace that implementation.


## S3 object-storage provider boundary in v0.4.6

`object-storage.s3/v1` is the portable capability boundary. The application owns logical bucket identity; provider placement, physical bucket names, IAM objects, endpoint placement, storage topology and implementation-specific state are not application intent.

The current Compose reference implementation uses one lazy shared SeaweedFS provider. It is created only when an application explicitly requests S3 object storage. Logical buckets remain application-owned resources even though the provider process/storage plane is shared.

Every logical bucket receives independent bucket-scoped credentials represented through `secure-binding/v1`. The provider uses SeaweedFS IAM internally, while plaintext credentials are resolved only at the trusted binding/runtime boundary and never enter provider-registry metadata or portable capability diagnostics.

Conformance for this capability must exercise authenticated S3 behavior, including Put/Get, rather than treating provider process liveness as readiness. A future Ceph RGW, AWS S3 or other conforming implementation must satisfy the same application-facing semantics without requiring a manifest rewrite.

The current public CLI does not yet expose general external-provider selection/loading. That remains operator/provider-platform work; the capability and provider protocol are already shaped so the application contract does not need to change when that layer arrives.

## OTLP provider boundary in v0.4.7

`telemetry.otlp/v1` uses the same Provider Integration Contract as every other capability. OTLP protocol semantics are owned by BaseHarbor; OpenTelemetry Collector is a reference provider implementation.

The current managed Compose provider is lazy/shared. An external OTLP endpoint is represented as external provider placement and remains externally lifecycle-owned. Provider-specific endpoints, authorization headers and Collector configuration remain deployment/provider state.

Conformance requires fail-closed preflight, idempotent provisioning/binding and a real OTLP HTTP/protobuf export accepted by the selected endpoint. Merely reporting a running Collector process is not sufficient.

The OTLP binding is capability-owned and typed in Provider Protocol v1. Requesting it does not authorize a provider to provision Prometheus, Loki, Tempo, Grafana or any other unrelated observability product.


## Metrics / Prometheus conformance in v0.4.8

Prometheus is the first reference provider for `metrics/v1`. The provider boundary remains the same boundary intended for later VictoriaMetrics, Mimir or community implementations.

Conformance requires at least:

- fail-closed preflight for direction, signal format and source endpoint;
- idempotent shared-provider provisioning;
- automatic target registration without manual Prometheus configuration;
- application/environment/service/source attribution;
- isolation when several applications use the same workload service name;
- real scrape/ingestion verification through a successful `up=1`;
- removal of only the affected application's target bindings;
- no credentials or secret values in target state or normal diagnostics;
- no implicit provisioning of Grafana, Loki or Tempo.

Collection policy is deployment/operator state. Declaring a `metrics/v1` source does not automatically authorize collection in every environment.

## Provider placement, sharing boundaries and runtime realization

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
Runtime realization of the selected placement
```

The canonical placement scopes remain exactly `application`, `shared` and `external`. A sharing boundary is an optional property of `shared`; it is not a fourth scope.

A shared provider is never automatically reachable by every application. Access is explicit, least-privilege and deny-by-default. A sharing boundary allows an operator to intentionally reuse one provider instance for a selected set of applications while keeping unrelated applications outside that trust boundary.

Provider implementations declare the placements they support. If policy resolves to a placement that the selected provider cannot satisfy, BaseHarbor fails closed before mutation instead of silently changing placement.

The portable application contract never contains provider placement, sharing-boundary, lifecycle-ownership or runtime-realization mechanics. The developer continues to state only application capabilities. BaseHarbor and deployment policy resolve the infrastructure details.

Placement semantics are fixed before runtime realization. The runtime may choose platform-native mechanisms to implement those semantics, but it may not reinterpret them: `application` remains one dedicated provider instance for exactly one application/environment, `shared` remains BaseHarbor Platform/Core Runtime infrastructure, and `external` remains externally lifecycle-owned. Compose currently realizes these guarantees through dedicated/shared projects, networks and volumes. Future Kubernetes/OpenShift runtimes may use namespaces/projects, Operators, NetworkPolicies or other platform-native mechanisms without changing the placement meaning or application intent.

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

## Managed container security and arbitrary-UID portability

A containerized BaseHarbor-managed provider is required to operate without UID 0.

The current Compose adapter may select a provider image's stable non-root service account or a known non-zero UID/GID. That is deployment realization only; it is never part of portable application intent or capability semantics.

Kubernetes and OpenShift runtimes must treat the runtime-assigned identity as authoritative. In particular, an OpenShift-compatible provider must tolerate an arbitrary platform-assigned non-zero UID instead of requiring the Compose UID. BaseHarbor-owned images prepare writable state paths for this model. Third-party images must either satisfy the active runtime's arbitrary-UID/security policy, be wrapped by a provider-owned compatible image without changing application semantics, or be rejected for that runtime before mutation.

Across runtimes, managed containers should use a read-only root filesystem, drop all Linux capabilities, disable privilege escalation, expose only the minimum writable state, avoid runtime sockets and privileged host access, and bind host ports no wider than the capability requires.

## Executable conformance since v0.4.9

The static descriptor checks remain the first gate, but Provider Integration Contract v1 now also has a reusable executable lifecycle conformance harness.

Reference providers can be exercised against side-effect-free preflight, unsupported-placement fail-closed behavior, deterministic provisioning, idempotent repeated convergence, binding, real verification, retry semantics, secret-safe diagnostics and ownership-safe destroy. The in-process fake provider supplies deterministic CREATE/NOOP/DRIFT/REPAIR/failure scenarios without introducing a new runtime or plugin system.

Loki is the first v0.4.9 reference provider required to consume this executable conformance machinery.
