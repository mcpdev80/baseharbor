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
```

Future examples include:

```text
object-storage.s3/v1
telemetry.otlp/v1
metrics/v1
logs/v1
traces/v1
messaging.queue/v1
messaging.pubsub/v1
messaging.stream/v1
```

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
