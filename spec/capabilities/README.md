# BaseHarbor Capability Specifications

BaseHarbor capability specifications define **what an application receives** from a capability. Provider products do not define these semantics.

Specification identifiers are versioned:

```text
<capability>/v<major>
```

Current examples:

```text
database.sql/v1
cache.key-value/v1
secrets/v1
exposure.http/v1
object-storage.s3/v1
telemetry.otlp/v1
metrics/v1
logs/v1
traces/v1
```

Future examples include:

```text
messaging.queue/v1
messaging.pubsub/v1
messaging.stream/v1
```

## Standards-first service model

Capability specifications remain the shipped v0.4 semantic contracts. Before the v0.5 freeze they are mapped onto the broader provider-neutral service model under `contracts/service/v1/`.

The mapping intentionally separates service identity from protocol compatibility:

| Existing capability | Standards-first classification |
| --- | --- |
| `database.sql/v1` | SQL service semantics; keep compatible while `sql/v1` service contract is frozen |
| `cache.key-value/v1` | cache service semantics; RESP is a provider compatibility declaration, not the service identity |
| `secrets/v1` | secrets service contract |
| `object-storage.s3/v1` | S3-compatible object-storage semantics; migrate toward generic object-storage service + S3 compatibility |
| `telemetry.otlp/v1` | OTLP protocol/transport capability under backend-neutral observability intent |
| `metrics/v1`, `logs/v1`, `traces/v1` | observability intent/storage semantics; telemetry transport remains OpenTelemetry/OTLP |

Existing IDs remain accepted until their compatibility migration is documented and tested.

## Standards-first service mapping

Capability specifications are the shipped v0.4 semantic compatibility surface. They map to broader provider-neutral service families:

| Capability specification | Service family | Standard/protocol role |
| --- | --- | --- |
| `database.sql/v1` | `sql` | SQL semantics; Service Binding for connection outputs |
| `cache.key-value/v1` | `cache` | RESP may be a provider compatibility requirement |
| `secrets/v1` | `secrets` | Service Binding names where service connection material is projected |
| `object-storage.s3/v1` | `object-storage` | S3 is API compatibility, not provider/product identity |
| `telemetry.otlp/v1` | `observability` | OpenTelemetry/OTLP transport |
| `metrics/v1`, `logs/v1`, `traces/v1` | `observability` | backend-neutral platform intent; OpenTelemetry remains authoritative for telemetry data |

The mapping is additive. Existing IDs remain valid until an explicit migration is documented and tested.

## Versioning rules

- A provider must declare the exact capability specification versions it implements.
- BaseHarbor must reject unknown or ambiguous specification versions before provider mutation.
- Backward-compatible clarification may occur within one specification major version.
- Breaking application-facing semantics require a new specification major version.
- Provider-specific configuration, object names, ports, paths and credentials are not capability specification content.
- Capability specifications describe application-facing behavior and required verification, not a product implementation.

## Provider conformance

A provider may claim compatibility with a capability specification only after passing the required BaseHarbor conformance checks for that specification.

The provider protocol and capability specification are separate version axes:

```text
Provider protocol:        baseharbor.provider/v1
Capability specification: database.sql/v1
```

A future provider can therefore implement a newer capability specification without requiring a new transport protocol when the protocol can already express it.

## Current v1 specifications

- [database.sql/v1](database.sql/v1.md)
- [cache.key-value/v1](cache.key-value/v1.md)
- [secrets/v1](secrets/v1.md)
- [exposure.http/v1](exposure.http/v1.md)
- [object-storage.s3/v1](object-storage.s3/v1.md)
- [telemetry.otlp/v1](telemetry.otlp/v1.md)
- [metrics/v1](metrics/v1.md)
- [logs/v1](logs/v1.md)
- [traces/v1](traces/v1.md)
