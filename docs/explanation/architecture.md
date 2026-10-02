# Architecture

BaseHarbor turns portable application intent into verified runtime infrastructure.

```text
Application
    ↓
Portable Intent
    ↓
Environment + Policy
    ↓
BaseHarbor Core
    ↓
Runtime + Capability + Delivery Providers
    ↓
Verified Result
```

## Portable intent

The application describes what it needs, not which infrastructure product must provide it.

Examples are SQL, cache, object storage, secrets, identity, HTTP exposure and telemetry.

## Repository workload sources

Repository syntax is interpreted before portable Application Intent and is not itself a provider axis.

```text
Repository
    ↓
Workload Source Adapter
    ↓
Normalized Workload Evidence
    ↓
Inspection / adoption
    ↓
Portable Application Intent
```

v0.4.20 proves this boundary with Compose, repository-authored Podman Quadlet and raw Kubernetes YAML. Source-native identity remains provenance. Logical workload components are the portable identity.

Helm and Kustomize are intentionally later source adapters. Kubernetes and OpenShift remain later Runtime Providers.

See [Workload sources](workload-sources.md).

## Three provider axes

BaseHarbor keeps three concerns separate:

```text
runtime != capability != delivery
```

- Runtime Provider: where workloads run.
- Capability Provider: how a logical dependency is realized.
- Delivery Provider: how desired runtime state reaches and reconciles with the runtime.

The current local runtime path supports Docker through Docker Compose and Podman through native Quadlets generated from the same Compose-based workload/runtime model. Kubernetes and OpenShift are later runtime providers.

## Environment and policy

Environment describes deployment risk and policy context. It does not identify a runtime or provider product.

## Ownership and placement

Where applicable, providers use the same placement model:

```text
application
shared
external
```

BaseHarbor mutates only resources it owns.

Shared placement is a first-class lifecycle boundary, not a shortcut for shared application data. The Target/provider owns the infrastructure; each application owns isolated logical resources, credentials and bindings. This makes resource-efficient topologies such as one PostgreSQL provider serving many isolated application databases possible without coupling portable application intent to provider topology.

## Lifecycle

Mutating operations follow:

```text
plan -> preflight -> apply -> verify
```

Reconciliation compares desired state with observed provider/runtime state and fails closed on ambiguity, ownership conflicts or unsupported requirements.

## Canonical local development routing

Development browser surfaces use one target-wide HTTPS gateway. The canonical URL is the developer-facing source of truth; runtime loopback ports and provider-specific internal endpoints remain implementation detail.

```text
https://<app>.baha.localhost
                    |
                    v
          Target dev gateway
                    |
                    v
      owned provider/runtime network
                    |
                    v
        verified internal upstream
```

The default domain is `baha.localhost` and remains configurable per Target. The normal application workload uses `<app>.<domain>`. Application-scoped management surfaces use `<app>-<service>.<domain>`; shared provider surfaces use short semantic hosts such as `pgadmin.<domain>`, `cache.<domain>`, `storage.<domain>`, `auth.<domain>`, `secrets.<domain>` and `metrics.<domain>`. Route ownership follows provider placement: application routes are removed with the application, shared routes belong to the shared provider boundary, and external providers keep their own URLs. Gateway TLS is issued from the existing BaseHarbor service-PKI boundary and HTTPS upstreams are verified against their projected trust material rather than disabling TLS verification.

## Interfaces

CLI, JSON and MCP are adapters over the same semantic core. No interface may bypass policy, ownership, verification or secret safety.

For normative behavior, use [Specs](../spec/README.md). For design rationale, use [ADRs](../decisions/).


## Documentation boundaries

Documentation follows the same responsibility discipline as code:

```text
Human docs      -> explain use and concepts
Reference       -> exact public behavior
Specs           -> normative contracts
Schemas/code    -> machine-readable authority
ADRs            -> decisions and rationale
GitHub Issues   -> future planning
Releases        -> delivered history
```

A detailed fact should have one authoritative home. Human documentation links to deeper reference/spec material instead of duplicating it.
