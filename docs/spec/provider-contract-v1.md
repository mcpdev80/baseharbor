# Provider Contract v1

## Scope

This specification defines the common rules for providers.

## Provider axes

Runtime, Capability and Delivery Provider are independent axes.

```text
runtime != capability != delivery
```

Implementations MUST NOT make one axis a hidden requirement of another.

## Standards-first interoperability

Providers MUST use an established open standard when it already defines the required interoperable semantics. De-facto standards and established ecosystem conventions SHOULD be used where no suitable formal standard exists.

Provider-specific contracts MUST stay behind the provider boundary. A provider MUST NOT require BaseHarbor Core or portable application intent to adopt product-specific configuration merely because that provider needs it.

Before a new service kind or provider is implemented, its design records:

- Existing Standards
- Adopted Standards
- Deviations
- BaseHarbor Extensions
- Compatibility Impact

## Provider identity and versioning

A provider implementation MUST expose a stable provider ID and implementation version independently from BaseHarbor and independently from capability specification versions.

For example:

```text
provider: baseharbor/postgresql
provider version: 0.1.0
protocol: baseharbor.provider/v1
capability: database.sql/v1
```

A concrete product version or image digest is realization metadata. It MUST NOT replace the provider implementation version or the capability specification version.

## Capabilities

A provider MUST declare the semantics it supports.

BaseHarbor MUST reject a provider before mutation when a required semantic is unsupported.

Nominal capability-name equality alone MUST NOT be treated as compatibility.

## Service connection outputs

Provider connection outputs align with Service Binding Specification 1.1 well-known entry names whenever the semantics match:

```text
type
provider
host
port
uri
username
password
certificates
private-key
```

Alternative aliases such as `hostname`, `connectionHost`, `user`, `pass` or `connectionString` MUST NOT be introduced when the standard name applies.

Secret-bearing entries may be represented internally through protected references and resolved only at the trusted workload projection boundary. Standard naming does not permit plaintext secrets in diagnostics, registry state or portable intent.

BaseHarbor-specific binding metadata uses an explicit versioned extension namespace and remains separate from Service Binding fields.

## Placement and ownership

Capability providers use the common placement vocabulary where the underlying product and provider implementation can satisfy the required semantics:

```text
shared
application
external
```

A provider descriptor MUST declare every placement scope it actually implements. BaseHarbor MUST reject an unsupported requested scope before mutation and MUST NOT silently downgrade isolation or ownership.

For `shared` placement:

- provider infrastructure lifecycle MUST belong to the Target/provider boundary rather than to one application;
- application data/logical resources, credentials, identities and Service Bindings MUST remain application-scoped;
- one application MUST NOT be able to read, mutate or destroy another application's logical resources through the standard application binding;
- application destroy MUST remove only application-owned resources and MUST preserve the shared provider and sibling applications;
- provider/Target destroy MAY remove the shared provider after application ownership has been released;
- a provider MUST NOT claim `shared` support if these isolation and lifecycle guarantees cannot be met.

Sharing provider infrastructure is explicitly a resource-efficiency mechanism. It MUST NOT be implemented by merely reusing one global application credential or one unpartitioned application data namespace.

`application` placement provides a dedicated provider lifecycle for one application/environment and remains valid when stronger physical isolation or product limitations require it.

`external` placement keeps provider lifecycle ownership outside BaseHarbor. BaseHarbor MUST NOT destructively mutate foreign/external resources it does not own.

A named sharing boundary MAY subdivide `shared` placement without introducing a fourth scope.

## Credential ownership taxonomy

Provider implementations MUST use the normative [Credential and access ownership v1](credential-access-v1.md) taxonomy.

Providers MUST preserve the distinction between Human/management identity, Application-service credentials and BaseHarbor-internal machine identity.

A provider MUST NOT replace internal machine credentials with shared human/developer credentials, and shared provider infrastructure MUST NOT imply shared application-service credentials.

Application business users, groups, roles and permissions remain application/IdP-owned and outside the provider credential model.

## Verification

A provider reporting process health is not sufficient when the capability requires protocol/data-flow verification.

## Container filesystem handoff

When a provider generates configuration, certificates, target definitions or
other files for a container, its implementation MUST declare the intended
consumer and make the required paths readable by that container's effective
UID/GID regardless of the host process umask. A successful host-side write is
not proof that the container can read the file or traverse its mounted
directories.

Non-secret configuration projections SHOULD use the narrowest modes that let
the declared consumer read them (commonly files `0644` and directories `0755`)
and MUST be mounted read-only unless the container must write them. Secret
source files MUST remain owner-protected on the host. A provider that needs to
make secret material readable to a non-root container MUST create a
consumer-scoped projection under a protected host-side parent and mount it only
into that authorized container. It MUST NOT solve access failures with
world-writable modes such as `0777`, by running the container as root, or by
loosening the original secret's permissions.

Provider lifecycle and conformance checks MUST exercise generated file and
directory access under a restrictive host umask and verify the consuming
container's actual read or protocol path before reporting readiness.

### Shipped provider filesystem requirements

The following requirements apply to every shipped provider realization and
management consumer. They are part of this contract, not implementation
guidance. A provider descriptor or implementation MUST refine these entries
with concrete image/version, effective UID/GID, mount destination, and access
mode for its selected topology. A provider MUST NOT pin a numeric UID or
require UID `0`. With Docker/Compose, omitting `user` preserves the image's
declared default identity; platforms such as OpenShift may assign an arbitrary
non-root UID. Provider images and filesystem paths MUST support those
platform-assigned UIDs. UIDs observed in a particular Docker run are diagnostic
metadata only and MUST NOT become provider requirements.

| Provider / consumer | Runtime identity requirement | Filesystem contract |
|---|---|---|
| PostgreSQL member | Image default locally; arbitrary platform UID supported, no BaseHarbor override | Database data directory is writable through the runtime's volume ownership/group mechanism. On rootless Docker, the upstream initialization entrypoint needs `CHOWN`, `DAC_READ_SEARCH`, `FOWNER`, `SETGID`, and `SETUID` to prepare a fresh volume and drop from root to the image's postgres account. Compose drops every other capability and keeps `no-new-privileges`. Server key, certificate, CA, and `pg_hba.conf` are read-only mounts. The key's host source remains owner-only; its tmpfs copy is mode 0600, owned by `postgres` before the entrypoint drops privileges, and scoped to PostgreSQL. |
| PostgreSQL HAProxy | Image default locally; arbitrary platform UID supported, no BaseHarbor override | `haproxy.cfg` and server certificate are non-secret/read-only projections. The proxy has no database data mount. |
| pgAdmin | Image default locally; arbitrary platform UID supported, no BaseHarbor override | `servers.json`, `pgpass`, connection CA certificates, UI password, and TLS certificate/key are consumer-scoped read-only projections. Credential/private-key projections remain below a host directory inaccessible to unrelated users and are mounted only into pgAdmin. |
| Valkey | BaseHarbor builds a pinned derivative with the upstream `valkey` image account as its local default and no Compose UID override; arbitrary platform UID supported | `/data` is group-owned by GID `0` and group writable/executable in the image so a platform-assigned non-root UID can initialize and write a mounted volume. AOF files are created with a restrictive group-compatible umask. On startup, existing Valkey-owned AOF files are migrated to GID `0` with group permissions; an assigned UID that cannot safely migrate old data MUST fail with a clear error. ACL configuration and TLS material are read-only mounts; ACL source is host-protected and projected only to Valkey. |
| Valkey HAProxy | Image default locally; arbitrary platform UID supported, no BaseHarbor override | `haproxy.cfg` and server certificate are readable, non-writable projections; no provider data or application credential files are mounted. |
| Redis Commander | Image default locally; arbitrary platform UID supported, no BaseHarbor override | `local.json`, `local-production.json`, HTTP password, and TLS material are read-only, consumer-scoped projections. `local.json` contains credentials and is mounted only into this authorized UI. |
| MongoDB | Image default locally; arbitrary platform UID supported, no BaseHarbor override | Database data is writable through the runtime's volume ownership/group mechanism. Initialization scripts, server certificate, and CA are read-only. Private key source remains protected and any consumer projection is scoped to MongoDB. |
| MongoDB UI | Image default locally; arbitrary platform UID supported, no BaseHarbor override | MongoDB CA/trust input is a read-only consumer projection. The UI access proxy is separately isolated and does not receive database credentials except through its declared authenticated upstream. |
| MongoDB UI access proxy | Image default locally; arbitrary platform UID supported, no BaseHarbor override | Caddyfile, server certificate, and private key are read-only mounts. The key is delivered only as a consumer-scoped projection beneath a protected host parent. |
| RabbitMQ | Image default locally; arbitrary platform UID supported, no BaseHarbor override | Broker data is writable through the runtime's volume ownership/group mechanism; generated configuration and TLS material are read-only projections. Credentials and private keys remain protected at source and are delivered only through scoped mounts. Docker health checks and broker-admin commands drop from root to the broker account before reading its owner-only Erlang cookie; a platform-assigned non-root UID runs these commands directly. |
| RabbitMQ UI access proxy | Image default locally; arbitrary platform UID supported, no BaseHarbor override | Caddyfile, certificate, and key are read-only consumer projections. A Caddyfile containing an authentication verifier is mounted only into this proxy. |
| SeaweedFS nodes/admin | Image default locally; arbitrary platform UID supported, no BaseHarbor override | The Docker image entrypoint uses `CHOWN`, `SETGID` and `SETUID` only to repair `/data` ownership and drop to its service account; these are the only added capabilities, with all others dropped and `no-new-privileges` retained. On arbitrary-UID platforms, the assigned UID must be able to write `/data` through the platform volume ownership/group mechanism. S3/admin credentials and TLS material are protected at source; each consumer receives only its scoped read-only configuration/key projections. |
| SeaweedFS access proxy | Image default locally; arbitrary platform UID supported, no BaseHarbor override | Caddy configuration and TLS material are read-only mounts; private key is consumer-scoped. |
| OpenBao server | Image default locally; arbitrary platform UID supported, no BaseHarbor override | Configuration, CA, server certificate/key, and PostgreSQL CA are read-only mounts. OpenBao storage is writable through the runtime's volume ownership/group mechanism. Server key and machine credentials remain host-protected and mounted only into OpenBao. |
| OpenBao HAProxy | Image default locally; arbitrary platform UID supported, no BaseHarbor override | HAProxy configuration is a readable, non-writable projection; no OpenBao machine credential or storage mount is permitted. |
| OpenBao admin surface | Image default locally; arbitrary platform UID supported, no BaseHarbor override | Admin CA/trust input and provider configuration are read-only. Administrative credentials use protected secret channels and MUST NOT be copied into broadly readable configuration. |
| Keycloak | Image default locally; arbitrary platform UID supported, no BaseHarbor override | Keycloak data is writable through the runtime's volume ownership/group mechanism. TLS and database CA material are read-only mounts; private material is host-protected and scoped to Keycloak. |
| Keycloak access proxy | Image default locally; arbitrary platform UID supported, no BaseHarbor override | Caddy configuration, upstream trust, certificate, and private key are read-only mounts; key material is consumer-scoped. |
| Prometheus | Image default locally; arbitrary platform UID supported, no BaseHarbor override | Configuration and target directories are read-only and readable/traversable by the assigned UID. Its data directory alone is writable through the runtime's volume ownership/group mechanism. Scrape client keys/certificates are consumer-scoped read-only projections. |
| Prometheus access proxy | Image default locally; arbitrary platform UID supported, no BaseHarbor override | Caddy configuration, upstream trust, and TLS material are read-only consumer projections; private keys are scoped to this proxy. |
| OpenTelemetry Collector | Image default locally; arbitrary platform UID supported, no BaseHarbor override | Collector configuration, CA, client certificate, and key are read-only consumer projections. Only declared queue/storage paths may be writable. Secret key sources remain protected and are projected only for the collector. |
| Loki | Image default locally; arbitrary platform UID supported, no BaseHarbor override | Loki configuration and trust material are read-only consumer-readable projections. Only Loki's `/loki` data paths may be writable. Object-storage credentials stay in protected runtime environment and MUST NOT be copied into broadly readable config. |
| Grafana Alloy log collector | Image default locally; arbitrary platform UID supported, no BaseHarbor override | Alloy config is a read-only consumer-readable projection. Its agent state uses a mode-`1777` temporary filesystem so any assigned non-root UID can create it; retained log data belongs to Loki. Host journal access MUST use a platform-supported read-only group/mount mechanism and MUST NOT be implemented by pinning UID `0`. |
| Tempo | Image default locally; arbitrary platform UID supported, no BaseHarbor override | `tempo.yaml` and public object-storage CA are read-only mounts; `/var/tempo` is the declared writable data path. S3 credentials remain in protected runtime environment and MUST NOT be written into `tempo.yaml`. |
| Runtime broker/executor | Image default locally; arbitrary platform UID supported, no BaseHarbor override | Runtime permissions/configuration, service tokens, workload identity certificates and operation paths are read-only except explicitly declared operation/resource state. Private keys and tokens are individual secret mounts. |
| Application workload | Workload/platform default locally; arbitrary platform UID supported, no BaseHarbor override | Service bindings, CA bundles, and runtime identity are read-only mounts. Only declared application data paths may be writable. Provider credentials are projected only when required and remain host-protected. |
| Caddy exposure and provider access proxies | Image default locally; arbitrary platform UID supported, no BaseHarbor override | Caddy configuration and certificates are read-only mounts; only explicitly declared Caddy state/cache paths may be writable. Private keys are consumer-scoped projections under protected host parents. |

For every row, the provider implementation MUST enforce these operational
rules:

- Requested creation modes are not sufficient: after writing or atomically
  replacing a consumer projection, the provider MUST explicitly set and verify
  its effective mode so a restrictive umask cannot make it unreadable.
- Providers MUST support arbitrary non-root UIDs. They MUST NOT set `runAsUser`,
  Compose `user`, or equivalent to a fixed numeric UID, and MUST NOT require UID
  `0`. Filesystem access MUST use read-only mounts, runtime-supported volume
  ownership, or group permissions compatible with platform-assigned UIDs.
- Non-secret configuration/public certificate projections MUST be readable by
  an arbitrary assigned UID without granting write access. Secret projections
  MUST use a consumer-scoped read-only mount plus a runtime-supported ownership
  or group mechanism; they MUST NOT rely on world-readable host source files.
  On OpenShift-compatible images, writable image paths MUST be group-owned by
  GID `0` and group-writable/executable as required (`g=u`); this is a group
  compatibility rule, not a UID `0` requirement.
- Authoritative private keys, credentials, tokens, and secret stores MUST stay
  owner-protected (normally `0600` files and `0700` parent directories) on the
  host. They MUST NOT be made readable by broadening their source permissions.
- Every bind mount MUST be read-only unless the provider requires container
  writes to that exact path. Writable data directories MUST be distinct from
  configuration/secret projections and owned or mapped to the declared
  runtime-assigned identity or declared group mechanism.
- A file replacement does not update an already-running container's bind
  mount. Providers MUST recreate/reload the consumer as required by that
  runtime and MUST verify the new inode/configuration is active before
  reporting readiness or rotation complete.
- Readiness MUST include a consumer-side read and the provider protocol/data
  path, not merely host-side `stat`, successful file creation, or a running
  container state.

Implementations MUST maintain a tested mapping from each row to the concrete
mount sources, file modes, directory traversal modes, arbitrary-UID behavior, and
reload/recreation action. Newly added providers and management surfaces MUST
add their requirements here before being considered supported.

## Registry boundaries

BaseHarbor distinguishes two registries:

- **Runtime Provider Registry** — deployed provider instances, placement, ownership and application/resource bindings.
- **Provider Catalog** — installable provider distributions, versions, supported service contracts/capabilities, product compatibility, platforms, OCI artifact identity, digest, schema and provenance.

The Runtime Provider Registry MUST NOT become a package/distribution catalog.

Provider artifacts SHOULD use OCI-compatible distribution. Artifact digest is immutable identity; tags are discovery aliases.

## Extensibility

Provider implementations MAY use mature OSS, standard APIs/SDKs, controllers/operators or managed-service APIs behind the BaseHarbor contract.


## Standards-first provider metadata

Provider metadata MUST keep these version axes independent:

```text
BaseHarbor service contract version
Provider protocol version
Provider implementation version
Product/engine version
Artifact digest
```

A provider descriptor SHOULD identify:

- stable provider ID and provider version;
- supported service kinds and BaseHarbor service/capability contract versions;
- protocol/semantic capabilities;
- supported product/engine versions where relevant;
- supported platforms;
- OCI artifact reference and immutable digest when distributed as an artifact;
- JSON Schema 2020-12 provider configuration schema;
- standard signature/SBOM/provenance references where available.

The canonical target schema is `contracts/provider/v1/provider-descriptor.schema.json`.

## Service bindings

Provider connection outputs MUST use Service Binding Specification 1.1 well-known entry names where their semantics apply.

BaseHarbor-specific binding extensions MUST remain in a distinct versioned extension namespace and MUST NOT redefine standard names.

Secret-bearing values may remain opaque references until the trusted workload projection boundary; this does not justify alternative field names.

## Service versus protocol

A provider implements a BaseHarbor service contract and MAY declare protocol compatibility.

Examples:

- `cache` is the service; RESP is a protocol compatibility property.
- `object-storage` is the service; S3 API compatibility is a protocol/API property.
- `observability` is the service family; OpenTelemetry/OTLP is the standard telemetry protocol/data path.

Provider product names never become portable application service kinds.


## Management-surface access

Human-facing provider management surfaces follow [Management surface access v1](management-access-v1.md). Providers declare the effective authentication class and role mapping; BaseHarbor MUST NOT infer stronger authorization than the provider can enforce.

The normative shipped-provider/surface classification is [Provider and management-surface acceptance v1](provider-management-acceptance-v1.md).

## Availability

Capability providers negotiate the same portable availability requirement independently from the Runtime Provider.

Providers declare SUPPORTED, PARTIALLY_SUPPORTED or UNSUPPORTED with explicit limits. A required unsupported guarantee fails before provider mutation. Provider-native clustering, quorum, replica roles and managed-service product modes remain realization state and never enter portable Application Intent.
