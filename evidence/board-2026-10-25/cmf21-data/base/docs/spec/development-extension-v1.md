# Development Extension Contract v1

## Scope

This specification defines BaseHarbor authoring-time development extensions:

- Stack Profiles;
- Development Plans;
- Development Adapters.

Development extensions create or validate ecosystem-native application source. They do not provision provider infrastructure and do not become runtime provider instances.

```text
application requirement
!= development integration
!= capability provider
!= placement
!= runtime
!= delivery
```

## Existing standards

BaseHarbor reuses existing formats and ecosystem conventions instead of defining a framework:

| Concern | Standard / convention |
| --- | --- |
| Schema validation | JSON Schema 2020-12 |
| Extension artifact distribution | OCI Image / Distribution |
| Go dependencies | Go modules |
| JavaScript / Next.js dependencies | package.json / npm ecosystem |
| Python dependencies | pyproject.toml / Python packaging conventions |
| Quarkus dependencies | Maven / Quarkus extension conventions |
| Telemetry | OpenTelemetry / OTLP |
| Provider connection outputs | Service Binding Specification naming where applicable |
| Portal projection | Backstage Catalog `Component` metadata, optional and static |

Devfile concepts may inform interoperability, but Devfile is not the canonical BaseHarbor Development Plan and is not a Core dependency.

## Adopted BaseHarbor contracts

Machine-readable authority:

- `contracts/development/v1/stack-profile.schema.json`
- `contracts/development/v1/development-plan.schema.json`
- `contracts/extension/v1/extension-descriptor.schema.json`

### Stack Profile

A Stack Profile is authoring metadata.

It MUST NOT become portable application intent, deployment state, runtime state or Provider Runtime Registry state.

A profile contains stable development components and optional capability preferences. Multiple components are supported from v1.

Composition is deterministic:

1. referenced parent profiles are resolved in declared order;
2. identical parent values may merge;
3. conflicting parent values fail closed;
4. the child profile may explicitly override inherited values;
5. cycles fail closed;
6. the effective profile has deterministic component and capability ordering.

An implementation preference remains a preference. It MUST NOT silently become a provider/product requirement in the portable Application Contract.

### Development Plan

The Development Plan is an authoring-time plan and is distinct from the runtime/deployment plan.

It may contain ecosystem dependency requirements, application binding expectations, source/config actions, build conventions, health conventions and telemetry integration actions.

The plan MUST be deterministic for the same Application Contract, effective Stack Profile and adapter versions.

### Development Adapter

A Development Adapter MUST expose these semantics:

```text
Descriptor
Detect
Supports
Plan
Bootstrap
Validate
```

An adapter MUST NOT provision capability-provider infrastructure, hide provider resolution inside application-source generation, introduce a required BaseHarbor application SDK, or require a runtime product in portable application intent.

Generated applications continue to use normal ecosystem libraries.

## Reference adapters

v0.4.18 defines four reference adapters:

```text
development/go
development/nextjs
development/python
development/quarkus
```

The reference adapters are examples of the contract, not privileged Core semantics. A third-party adapter uses the same common extension metadata, resolution/trust rules and conformance expectations.

## Conformance

A conforming adapter proves: Descriptor, Detection, Contract mapping, Development planning, Bootstrap, Binding portability, Capability integration, Inspect round-trip, Evidence validation, Idempotency, Secret safety, No provider leakage and MCP parity.

The implementation MUST converge on:

```text
Contract
  -> Development Plan
  -> ecosystem-native implementation
  -> normal BaseHarbor inspection
  -> evidence
  -> SATISFIED
```

MCP parity is verified through the normal BaseHarbor machine tool `baseharbor.app.new`; no shell or runtime escape hatch is part of the adapter contract.

## Extension distribution and trust

Provider Extensions and Development Extensions share distribution mechanics, not lifecycle semantics.

Common metadata includes extension family, stable extension ID, implementation version, compatibility, platform compatibility, OCI reference, immutable digest, JSON Schema reference and signature/SBOM/attestation references.

Resolution is fail-closed when multiple candidates are ambiguous. Trust policy may require digest, signature, SBOM and/or attestation metadata before an extension is accepted.

Tags are discovery aliases. Immutable digest is authoritative identity when artifact distribution is used.

## Backstage boundary

Backstage support is an emitted-contract integration, not a Development Adapter family and not a Core dependency.

`catalog-info.yaml` emission is optional. Portal owner is explicit and portal lifecycle is never derived from BaseHarbor environment. Runtime, target, provider placement and credentials MUST NOT leak into emitted Catalog metadata.

See ADR 0016 and `docs/how-to/backstage.md`.

## Compatibility impact

These contracts add authoring metadata only.

They do not change existing portable Application Contract semantics, capability-provider placement semantics or runtime-provider semantics. Brownfield `baha app init` and greenfield `baha app new` converge on the same Application Contract and repository inspection/evidence model.
