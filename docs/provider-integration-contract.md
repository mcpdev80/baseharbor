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

- shared provider instances;
- application-scoped provider instances;
- external/BYO providers;
- BaseHarbor lifecycle ownership versus external lifecycle ownership.

External provider compatibility does not grant BaseHarbor permission to mutate an externally owned provider.

Logical resources remain application-owned even when the provider instance is shared.

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

Observation never grants BaseHarbor lifecycle ownership. Only explicit managed exposure intent is registered as an application-scoped provider resource. The current Compose reference provider is Caddy; its host ports, network, TLS files and generated configuration remain protected provider/deployment state.
