# ADR 0001: Stable service instances and availability intent

- Status: Accepted
- Date: 2026-09-08

## Context

BaseHarbor is a backend runtime for independent applications. Applications should declare the backend capabilities they need while BaseHarbor owns provisioning, security, topology and lifecycle.

A single application may legitimately need more than one service of the same type, for example:

- a transactional PostgreSQL database and a separate analytics PostgreSQL database
- one Valkey instance for cache and another for sessions

BaseHarbor also needs a path to multi-node and highly available deployments without turning the application manifest into Docker Compose, Kubernetes, Patroni or Sentinel configuration.

## Decision

### 1. Service instances have stable logical names

Repeated services are modeled as named logical instances, not as an integer replica count.

```yaml
services:
  postgres:
    instances:
      primary: {}
      analytics: {}

  redis:
    instances:
      cache: {}
      sessions: {}
```

Names such as `primary`, `analytics`, `cache` and `sessions` are application-facing logical identities. They remain stable even if the underlying runtime topology changes.

The compact single-instance form remains supported:

```yaml
services:
  postgres:
    enabled: true
  redis:
    enabled: true
```

This resolves internally to one instance named `default` and preserves the standard `DATABASE_URL`, `REDIS_URL` and `VALKEY_URL` contract.

### 2. Instance count and HA replica count are different concepts

Two PostgreSQL instances mean two independent logical databases with independent credentials, storage and lifecycle.

High availability does not mean creating additional logical instances. HA is an implementation topology behind one logical instance.

For example, future intent may look like:

```yaml
services:
  postgres:
    instances:
      primary:
        availability: high
```

The application still receives one stable `primary` endpoint. BaseHarbor may implement that endpoint with multiple nodes, a proxy, leader election, replication and failover.

### 3. The manifest expresses intent, not topology

Application manifests must not require infrastructure-specific fields such as:

```text
patroni
sentinel_count
etcd_nodes
quorum_size
pod_anti_affinity
synchronous_standby_names
```

Those are runtime-provider decisions.

The intended future availability vocabulary is small and semantic, for example:

- `standard`
- `high`
- `critical`

The exact supported values and guarantees will be introduced only when HA is implemented and tested. This ADR reserves the architectural direction; it does not enable HA behavior today.

### 4. Application-facing endpoints stay stable across topology changes

Applications consume native protocols and normal environment variables or binding files.

A future HA migration must not require application-code changes. For example:

```text
DATABASE_PRIMARY_URL=postgresql://stable-endpoint/...
REDIS_CACHE_URL=redis://stable-endpoint/...
```

The endpoint may later terminate at a local proxy, virtual service, managed database endpoint or Kubernetes Service. That is transparent to application code.

### 5. Runtime providers map intent to implementation

The same logical application manifest should remain portable across:

- single-node Docker/Podman
- remote or multi-node hosts
- Kubernetes
- external managed PostgreSQL/Redis providers

Each runtime provider is responsible for translating the logical service instance and availability intent into a supported topology.

A provider must fail closed when it cannot satisfy the requested availability contract. It must not silently downgrade `high` to a single instance.

## Consequences

### Positive

- applications can request multiple independent PostgreSQL or Valkey instances today
- instance identities remain stable across upgrades and future HA implementations
- simple applications keep a very small manifest
- BaseHarbor avoids exposing implementation-specific orchestration details
- application code remains independent of the `baha` CLI and BaseHarbor SDKs
- future HA can be added without redefining what a logical service instance means

### Trade-offs

- runtime reconciliation must track resources per logical instance
- generic environment variables become ambiguous when multiple instances exist
- removal or renaming of a service instance is a stateful lifecycle operation and must be handled safely
- HA requires separate work for placement, failover, quorum, backup/recovery and provider capability validation

## Runtime contract rules

For a single default instance, compatibility variables remain:

```text
DATABASE_URL
REDIS_URL
VALKEY_URL
```

Named instances receive explicit variables such as:

```text
DATABASE_PRIMARY_URL
DATABASE_ANALYTICS_URL
REDIS_CACHE_URL
REDIS_SESSIONS_URL
VALKEY_CACHE_URL
VALKEY_SESSIONS_URL
```

When several PostgreSQL instances exist, `default` is the preferred generic `DATABASE_URL` target. If no `default` exists but a `primary` instance exists, `primary` may receive the generic alias. Otherwise BaseHarbor does not invent an ambiguous generic database URL.

The same preference rule applies to cache bindings. If no unambiguous preferred instance exists, only named variables are emitted.

## Non-goals of this decision

This ADR does not choose:

- Patroni versus another PostgreSQL HA implementation
- Redis Sentinel versus Redis Cluster or another Valkey topology
- a Kubernetes operator
- a specific proxy/load balancer
- synchronous versus asynchronous replication defaults
- production HA SLOs

Those decisions require their own implementation and operational evidence.