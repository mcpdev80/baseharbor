# ADR 0001: Stable service instances and availability intent

- Status: Accepted
- Date: 2026-09-08

## Context

BaseHarbor is a backend runtime for independent applications. Applications should declare the backend capabilities they need while BaseHarbor owns provisioning, security, topology and lifecycle.

A single application may legitimately need more than one service of the same type, for example:

- a transactional PostgreSQL database and a separate analytics PostgreSQL database
- one Valkey instance for cache and another for sessions

BaseHarbor also needs a path to multi-node and highly available deployments without turning the application manifest into Docker Compose, Kubernetes, Patroni or Sentinel configuration.

The same requirement applies to BaseHarbor itself. The control plane must be able to run in a simple standard topology for development and small installations, while also having a clear path to a highly available topology for production environments. Operators should express the desired availability level without having to hand-author the control-plane topology.

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

### 4. Availability intent also applies to the BaseHarbor control plane

BaseHarbor itself follows the same intent-over-topology rule as application services.

A future operator-facing configuration may express only the desired control-plane availability, for example:

```yaml
baseharbor:
  availability: standard
```

or:

```yaml
baseharbor:
  availability: high
```

`standard` is the simple default topology intended for development, evaluation and smaller installations. It should minimize moving parts while preserving the same security and runtime contracts.

`high` means that the BaseHarbor control plane must be deployed with no avoidable single point of failure for the components required to operate the platform. The exact topology remains a BaseHarbor/provider responsibility.

The HA control-plane design is expected to cover at least:

- the BaseHarbor control-plane API
- control-plane PostgreSQL
- OpenBao
- stable control-plane network endpoints
- certificate/PKI availability and rotation paths
- any coordination, routing or leader-election component introduced by the chosen implementation

A provider must fail closed if it cannot satisfy the requested control-plane availability level. It must never silently start a standard single-node topology when `high` was requested.

### 5. The `baha` CLI surface stays stable across standard and HA modes

Operators should continue to use the same primary workflow regardless of topology:

```text
baha up
baha status
baha doctor
```

The CLI may expose explicit setup or migration options for choosing an availability mode, but day-to-day commands must not require operators to understand the underlying HA implementation.

`baha status` and `baha doctor` must become topology-aware: in `high` mode they should validate the effective HA contract rather than only reporting that individual processes are running.

Examples of future HA-aware diagnostics include:

- required control-plane members are reachable
- quorum is healthy where applicable
- the stable write endpoint is available
- replicas are sufficiently current according to the selected HA contract
- OpenBao is available through the supported HA topology
- certificate/PKI paths remain usable
- no requested `high` component has silently degraded to an unsupported single-node state

The `baha` executable itself is an operator tool, not a runtime dependency. A running BaseHarbor installation, including an HA installation, must continue operating when no `baha` process is running.

### 6. Application-facing endpoints stay stable across topology changes

Applications consume native protocols and normal environment variables or binding files.

A future HA migration must not require application-code changes. For example:

```text
DATABASE_PRIMARY_URL=postgresql://stable-endpoint/...
REDIS_CACHE_URL=redis://stable-endpoint/...
```

The endpoint may later terminate at a local proxy, virtual service, managed database endpoint or Kubernetes Service. That is transparent to application code.

The same principle applies to BaseHarbor's own operator-facing endpoints. Moving a control plane from `standard` to `high` must not force application code to adopt a BaseHarbor SDK or runtime login flow.

### 7. Runtime providers map intent to implementation

The same logical application manifest should remain portable across:

- single-node Docker/Podman
- remote or multi-node hosts
- Kubernetes
- external managed PostgreSQL/Redis providers

Each runtime provider is responsible for translating the logical service instance and availability intent into a supported topology.

The same rule applies to the control plane: a provider may implement `high` differently on a multi-host Docker environment, Kubernetes or a future managed environment, while preserving the BaseHarbor operator contract.

A provider must fail closed when it cannot satisfy the requested availability contract. It must not silently downgrade `high` to a single instance.

## Consequences

### Positive

- applications can request multiple independent PostgreSQL or Valkey instances today
- instance identities remain stable across upgrades and future HA implementations
- simple applications keep a very small manifest
- BaseHarbor avoids exposing implementation-specific orchestration details
- application code remains independent of the `baha` CLI and BaseHarbor SDKs
- future application-service HA can be added without redefining what a logical service instance means
- future BaseHarbor control-plane HA follows the same small semantic availability model
- operators keep the same core `baha` workflow in standard and HA modes
- `baha` remains an operator tool rather than a required runtime process

### Trade-offs

- runtime reconciliation must track resources per logical instance
- generic environment variables become ambiguous when multiple instances exist
- removal or renaming of a service instance is a stateful lifecycle operation and must be handled safely
- HA requires separate work for placement, failover, quorum, backup/recovery and provider capability validation
- control-plane HA requires explicit health semantics beyond process liveness
- migrations between `standard` and `high` must preserve identities, secrets and stable endpoints without unsafe implicit downgrade paths

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
- OpenBao HA storage/topology details
- a Kubernetes operator
- a specific proxy/load balancer
- synchronous versus asynchronous replication defaults
- production HA SLOs
- exact quorum sizes or node counts
- the concrete CLI syntax for switching an existing installation from `standard` to `high`

Those decisions require their own implementation and operational evidence.