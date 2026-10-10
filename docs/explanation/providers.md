# Providers

Providers realize BaseHarbor semantics without changing portable Application Intent.

## Runtime providers

Runtime Providers run application workloads.

Current reference runtimes:

- Docker;
- rootless Podman.

Compose remains a workload-source/input model for the local runtime path. It is not the portable Runtime Provider identity.

Future Runtime Providers include Kubernetes and OpenShift.

The v0.4.19 Runtime boundary deliberately does not require those future providers to imitate Compose, Docker or Podman mechanics.

## Capability providers

Capability Providers realize logical application dependencies.

Examples include:

- `database.sql`;
- `cache.key-value`;
- `database.key-value`;
- `database.document`;
- `messaging.queue`;
- `messaging.pubsub`;
- `messaging.stream`;
- `object-storage.s3`;
- secrets;
- OIDC identity;
- metrics, logs, traces and OTLP;
- HTTP exposure.

Reference products prove these contracts. Product names do not become portable capability names.

## Delivery providers

Delivery Providers control who owns reconciliation of desired runtime state.

The portable distinction is:

- `direct` — BaseHarbor owns reconciliation;
- `delegated` — an external reconciler owns reconciliation.

Runtime, capability and delivery remain separate axes:

```text
runtime != capability != delivery
```

## Bundled and external providers

Bundled providers ship with BaseHarbor but retain their own provider identity/version.

External/BYO providers let BaseHarbor consume infrastructure it does not own.

Removing an external provider registration removes the BaseHarbor reference/binding and must not destroy the foreign service.

## Placement

Capability providers use the common placement model:

- `shared` — Target/provider-owned infrastructure serving isolated application resources;
- `application` — dedicated provider lifecycle for one application/environment;
- `external` — infrastructure lifecycle remains outside BaseHarbor.

Shared infrastructure never means shared application credentials, ownership or unpartitioned data.

For example:

```text
one shared PostgreSQL provider
├── app-a database + least-privilege role
├── app-b database + least-privilege role
└── app-c database + least-privilege role
```

The same ownership rule applies to other provider families where the product can safely implement shared placement.

## Stable identity

Provider ownership does not depend on container names, repository paths or future Kubernetes object names.

```text
provider_instance_id != application_id != deployment_id
```

Application-scoped provider ownership follows the stable `application_id`.

## Related documentation

- [Provider CLI](../cli/providers.md)
- [External / BYO providers](../how-to/external-providers.md)
- [Provider Contract v1](../spec/provider-contract-v1.md)
- [Runtime Provider Contract v1](../spec/runtime-provider-contract-v1.md)
- [Delivery Provider Contract v1](../spec/delivery-provider-contract-v1.md)


## Explicit HA and native provider topology

On a fresh installation, omitted `ha` and `ha: false` select the standard topology. `ha: true` selects the supported native topology; a component override takes precedence over the global request. Fixed native cardinalities are validated before mutation. Independent logical instances remain independent datasets, even when their names end in a number.

The following audit covers Core and bundled application providers. Counts describe data or service roles, not total containers. Runtime status and Doctor compare protected retained Compose definitions with the selected runtime's running services. They report requested/active HA, data members, service members, configured replication, auxiliary roles and the actual runtime-host failure domain. Uncollected replication or failover proof is stated explicitly; multiple containers alone are never replication proof.

| Provider | Standard topology | Explicit native HA | Data replication | Auxiliary services | Resource implication / audit finding |
| --- | --- | --- | --- | --- | --- |
| Core PostgreSQL | One PostgreSQL data member | Three Patroni data members and three etcd voters | Streaming WAL to two standbys | Stable SQL proxy, administrative client, initialization/TLS jobs | One vs three data volumes, plus three HA DCS volumes; proxy/admin are not database replicas |
| Core OpenBao | One OpenBao process, using the Core SQL database | Three OpenBao processes, sharing the Core HA SQL database | SQL replication belongs to PostgreSQL; OpenBao processes are not separate copies of its dataset | Stable API proxy and administrative client | One vs three application processes; SQL storage is counted once |
| Core / Shared Keycloak | One identity process, dedicated database/user on Core SQL | Three identity processes on the same Core HA SQL | Core WAL replication; no additional SQL deployment | One identity access gateway | One physical Shared identity provider; app realms/clients are logical consumers |
| Shared PostgreSQL | Reuses the one Core SQL member | Reuses three Core Patroni members; incompatible consumer overrides are rejected | Same Core WAL replication | Existing Core helpers, optional UI | Isolated owned databases/users; no environment/boundary deployment or extra data volumes |
| Application PostgreSQL | One isolated SQL member per logical instance | Application placement rejects HA explicitly; use the native shared provider for HA | None in isolated Single mode | Optional administration/UI | Previously silent Single realization of requested HA now fails before mutation; this placement limitation remains explicit |
| SeaweedFS | One combined master/volume/filer/S3 member | Odd member count of at least three; default three | Native volume replication `100` and coordinated filer metadata; Single uses `000` | Stable authenticated TLS S3 gateway; optional management services | One vs N data volumes; the unconditional three-node default is removed |
| Prometheus | One scraper/TSDB | Two independent scrapers/TSDBs by default; explicit count at least two | Zero shared-dataset replicas: each TSDB stores its own scrape history | Stable authenticated access gateway | One vs N TSDB volumes; the unconditional two-instance default is removed |
| OpenTelemetry Collector | One receiver | Two receivers by default; explicit count at least two | No data replicas; receivers are stateless | Stable authenticated OTLP gateway | One vs N collector processes; the unconditional two-receiver default is removed |
| Loki | One local-storage data service | Three native read/write members with replication factor two | Two ingester copies; long-term objects use the explicitly requested object-storage topology | Alloy collection and authenticated access gateway | One vs three local WAL/data volumes; Loki HA does not implicitly enable S3 HA |
| Tempo | One local-storage process | Distributed native roles: two distributors, six live stores across three partitions/two logical zones, three block builders, two query frontends, two queriers, one backend scheduler and two backend workers | Two live-store owners per partition; three Redpanda brokers; object storage follows its own intent | Kafka initialization and authenticated query access | Eighteen Tempo service roles plus three Redpanda data brokers; `instances: 2` is a native topology input, not two total containers |
| Valkey cache / durable key-value | Shared: one provider per Core/Target; AppScoped: one member per requested instance | One primary, N−1 replicas and three Sentinels; default N=3 | Native primary/replica replication | Access gateway, HA Sentinels, optional operator UI | Shared ACL users and key/channel namespaces; no per-consumer data volume |
| RabbitMQ | One broker per logical instance | Odd member count of at least three; default three | Provider-native quorum queues / replicated streams | Stable access proxy and optional management access | One vs N broker data volumes; explicit member request controls replication topology |
| MongoDB | One standalone data member per logical instance | Odd member count of at least three; default three | Native replica set with primary/secondaries | Optional management UI and its access gateway | One vs N data volumes; standalone is not mislabeled as a replica set |
| Gateways, administrative clients, init jobs, Alloy and UI services | Started only for their requested technical/management role | No independent data-HA default | No replicas of the provider dataset | These services are themselves auxiliary roles | Their CPU/RAM and any own UI state still count toward total resources; they are not removed merely to reduce container counts |

Resource figures above describe processes and persistent storage domains. CPU/RAM consumption depends on workload, retention and image configuration; no fabricated measurements or universal minimum are claimed. Runtime acceptance records native member/helper names and checks authenticated readiness, repeated reconciliation and owned destruction.

Status and Doctor distinguish requested HA, native live data/service members, auxiliary roles, configured replication and authenticated replication proof. Core and identity SQL use native Patroni primary/replica health roles; Valkey uses replication links and Sentinel quorum, MongoDB primary/secondary roles, and RabbitMQ native cluster membership. RabbitMQ data replication remains queue-policy-dependent. Counts alone report `ha-active=unknown`, not verified HA; a live replication observation is not a destructive failover test.

### Existing installations and recovery

Existing Single or HA data topology is retained. A conflicting explicit or default manifest request fails before changing retained Compose, credentials or native members; maintenance without a new topology request uses retained membership. There is no automatic destructive cluster-to-Single migration. Native acceptance also attempts a conflicting request and verifies unchanged running container identities.

Provider SQL recovery uses the installation's protected operator identity only inside the provider-owned database restore. Version-added dependencies are removed only inside the bound provider database, and the complete schema/data restore runs in one transaction. Archive decoding fails before any database mutation; decoded SQL is temporary and owner-only. The original Compose is captured with the database backup before mutation. The verified archive preserves original ownership and ACLs; application credentials remain least-privileged. OpenBao is unsealed before readiness verification. Failed-provider recovery admits a removed or stopped container only when protected retained Compose and approved immutable original/target image digests prove the owned source; normal upgrade selection still requires live native inventory.

The tracking issue is [#851](https://github.com/mcpdev80/baseharbor/issues/851), integrated directly into [PR #837](https://github.com/mcpdev80/baseharbor/pull/837). Runtime qualifications and the final candidate are recorded there. This audit does not by itself declare release readiness or claim that pending runtime gates passed.

### Shared provider ownership

Shared providers are physical installation/Target resources. Environments and sharing boundaries are logical consumers; Prometheus, Loki and Tempo reuse a canonical Shared deployment. Explicit AppScoped providers retain independent deployments and lifecycle. AppScoped Keycloak keeps its own SQL database deployment, while Shared Keycloak uses the owned `baseharbor_identity` database on Core PostgreSQL.

Shared Valkey clients must prefix every key/channel using binding `baseharbor-key-prefix` or `REDIS_KEY_PREFIX` / `VALKEY_KEY_PREFIX` (including named-instance variants). Consumer ACLs deny administrative scanning, scripts, database selection and foreign namespaces. Clients requiring an unmodified global keyspace must explicitly select AppScoped. App destruction removes only its owned database/user or ACL user/namespace. Shared consumer credential/PKI rotation requires a Core-owned transaction and is currently rejected explicitly.

Retained environment/boundary deployments or a dedicated Shared Keycloak database fail before reconciliation/update: backup-verified migration is currently unsupported, with existing data and Compose retained. No silent conversion or deletion occurs. [#857](https://github.com/mcpdev80/baseharbor/issues/857) tracks implementation and native acceptance; pending gates are not claimed as successful.
