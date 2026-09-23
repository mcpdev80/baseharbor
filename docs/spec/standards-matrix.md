# Standards matrix

This matrix is the pre-v0.5 standards-first baseline. It records what BaseHarbor adopts, what remains a BaseHarbor extension and where existing v0.4 contracts need alignment rather than blind replacement.

| Area | Standard / pattern | Current version / status | Class | Governance / license | Maturity | Direct BaseHarbor use | Gap / BaseHarbor extension | Current action |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| Schema | JSON Schema | 2020-12 | Open standard | JSON Schema community; open specification | Very high | Portable service schemas and provider configuration validation | Lifecycle, ownership and provider execution are outside schema scope | **ADOPT** |
| Service connection | Service Binding Specification | 1.1.0 | Open specification | servicebinding.io community; Apache-2.0 | Mature in cloud-native ecosystems | Well-known binding names: `type`, `provider`, `host`, `port`, `uri`, `username`, `password`, `certificates`, `private-key` | Identity refs, authorization, rotation/revocation, ownership and verification metadata | **ALIGN** |
| Provider/resource architecture | Crossplane resource/provider/reconciliation model | v2.4 docs line | Established architecture pattern | CNCF ecosystem; Apache-2.0 project | High | desired/observed state, provider abstraction, external realization and reconciliation concepts | Do not import Kubernetes CRDs/finalizers/namespaces into portable intent | **KEEP / ALIGN** |
| Service lifecycle | Open Service Broker API | v2.17 family | Open API / architecture pattern | Open Service Broker community; Apache-2.0 | Mature but narrower than BaseHarbor | catalog, plan, provision, update, bind, unbind, deprovision and async-operation concepts | BaseHarbor adds reconciliation, placement, ownership, verification, backup/restore and runtime separation | **KEEP / ALIGN** |
| Provider distribution | OCI Image / Distribution / Runtime | Image 1.1.1, Distribution 1.1.1, Runtime 1.3.0 | Open standard | Open Container Initiative / Linux Foundation; Apache-2.0 | Very high | provider artifacts, digest identity, indexes/platforms, registry-neutral distribution | BaseHarbor catalog metadata and compatibility rules | **KEEP** |
| Observability | OpenTelemetry / OTLP | OTel 1.61.0, OTLP 1.11.0 | Open standard | CNCF; Apache-2.0 | Very high | traces, metrics, logs transport and semantic conventions | BaseHarbor intent/policy/provider resolution only | **KEEP / ALIGN** |
| Identity | OpenID Connect / OAuth | OIDC Core 1.0 Errata 2 | Open standard | OpenID Foundation | Very high | discovery, issuer, scopes/claims, standard authentication/token semantics | BaseHarbor authorization/policy and provider lifecycle | **ADOPT** |
| Messaging description | AsyncAPI | 3.1.0 | Open standard | AsyncAPI Initiative; Apache-2.0 | High | channel/operation/message API description | Broker provisioning, placement and lifecycle | **ADOPT when messaging ships** |
| Event envelope | CloudEvents | 1.0.2 compatible with 1.0 | Open standard | CNCF; Apache-2.0 | High | event envelope where event semantics apply | Broker-specific delivery semantics | **ADOPT when applicable** |
| Cache protocol | RESP | RESP2 / RESP3 | De-facto protocol standard | Redis/Valkey ecosystem convention | Very high for Redis-compatible systems | compatibility property for Redis/Valkey/Dragonfly-like providers | Generic cache semantics and non-RESP providers | **ALIGN** |
| Object storage API | S3 API | current AWS S3 API family | De-facto API standard | AWS-defined, widely implemented | Very high | compatibility property for S3-compatible providers | Generic object-storage service semantics and non-S3 providers | **ALIGN** |
| Vector | No accepted neutral service contract | none | No sufficient standard | fragmented ecosystem | Medium / fragmented | reuse generic schema/binding standards where possible | minimal BaseHarbor vector semantics only | **BAHA EXTENSION** |

## Service-family gap analysis

| Service | Existing standard coverage | What BaseHarbor keeps | What must align | BaseHarbor-only semantics |
| --- | --- | --- | --- | --- |
| `sql` | Service Binding; provider/resource patterns; engine conventions | logical database resource, lifecycle, provider resolution | standard connection names; engine-specific fields move behind provider | minimum portable SQL semantics and required feature subset |
| `cache` | Service Binding; RESP for compatible providers | generic cache intent | stop treating Redis/Valkey names as generic service identity | required cache semantic subset |
| `object-storage` | Service Binding; S3 as de-facto API | logical object resource/bucket semantics | `object-storage.s3/v1` becomes compatibility-mapped, not universal object-storage identity | provider-neutral object-storage semantics |
| `secrets` | Service Binding for delivered connection material | required secret names, opaque refs, lifecycle/security model | standard output names where applicable | rotation/revocation/authorization/reference semantics |
| `messaging` | AsyncAPI, CloudEvents; provider protocols such as AMQP/MQTT/Kafka | generic messaging intent | use standards from first implementation | lifecycle, placement and required semantic subsets |
| `vector` | no complete neutral contract | nothing product-specific | audit APIs before adding fields | smallest demonstrated common semantics |
| `observability` | OpenTelemetry/OTLP | intent, policy, provider/backend selection | avoid parallel Baha telemetry wire semantics | placement/ownership/verification |
| `identity` | OIDC/OAuth | provider selection and policy | no proprietary login/token semantics | BaseHarbor lifecycle/authorization policy |

## Current v0.4 classification

### KEEP

- provider-neutral application intent;
- Provider Integration Contract lifecycle;
- gRPC/Protocol Buffers process boundary;
- OCI distribution direction;
- desired/observed/reconcile/verify model;
- provider placement and ownership;
- provider conformance;
- `telemetry.otlp/v1` use of standard OpenTelemetry variables.

### ALIGN

- `secure-binding/v1`: keep BaseHarbor security/lifecycle extensions, use Service Binding 1.1 names for standard connection outputs;
- `database.sql/v1`: keep shipped semantics, map to service kind `sql`;
- `cache.key-value/v1`: keep shipped semantics, map to service kind `cache`, declare RESP separately where required;
- `object-storage.s3/v1`: keep shipped v0.4 compatibility, map to `object-storage` plus S3 compatibility;
- `metrics/v1`, `logs/v1`, `traces/v1`: keep application/platform intent where useful but OpenTelemetry remains authoritative for telemetry data semantics.

### REPLACE

No shipped v0.4 contract is currently scheduled for blind replacement. Any replacement requires an explicit compatibility migration.

### BAHA EXTENSION

- provider placement and lifecycle ownership;
- reconciliation/verification semantics;
- provider catalog metadata above OCI;
- security lifecycle metadata not covered by Service Binding;
- minimal vector service semantics if/when implemented.

## Freeze requirements

Before the v0.5.x contract freeze:

1. every frozen service contract has JSON Schema 2020-12;
2. standard Service Binding names are canonical for connection outputs;
3. service kind, protocol compatibility, provider implementation and product version are distinct;
4. provider catalog metadata is distinct from runtime provider-instance state;
5. existing v0.4 capability IDs have documented compatibility mappings;
6. conformance tests validate adopted standards and BaseHarbor extensions separately;
7. no provider-specific product contract is required by portable application intent.
