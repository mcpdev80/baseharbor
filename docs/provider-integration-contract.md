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
- **OCI artifacts and OCI Distribution**: registry-neutral packaging and distribution.
- **Open Service Broker API concepts**: useful lifecycle concepts around catalog/provision/update/bind/unbind/deprovision.
- **Service Binding concepts**: useful representation patterns for application-facing bindings.

These building blocks do not replace BaseHarbor's own capability, lifecycle, ownership, security or verification rules.

The public compatibility contract must not depend on HashiCorp go-plugin, Kubernetes, Docker, GitHub, a specific cloud provider or a BaseHarbor-operated registry.

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

## OCI packaging and distribution

Future external providers should be publishable through normal OCI registries.

Examples include GHCR, Quay, Harbor, Artifactory and private OCI-compatible registries.

BaseHarbor must not require a proprietary account or central BaseHarbor registry.

Provider packages should eventually carry:

- provider protocol version;
- implemented capability specifications;
- immutable digest;
- provider metadata;
- platform/runtime metadata where needed;
- standard signature/provenance references where practical.

The exact OCI artifact manifest/media type is intentionally deferred until the external loader is implemented.

## Bindings and secrets

Bindings expose standard application-facing values.

Secret material is not normal provider metadata.

Provider responses, diagnostics, registry state and portable application intent must never contain plaintext passwords, tokens, private keys or credential-bearing URLs.

When a binding needs secret material, the provider contract should return a stable credential/secret reference for BaseHarbor to resolve at a trusted boundary.

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

- Manifest v1 is unchanged.
- Compose remains the complete current runtime.
- Existing PostgreSQL, Valkey and OpenBao provisioning remains authoritative.
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
