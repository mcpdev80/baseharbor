# ADR 0009: Evolving application intent and non-destructive reconciliation

## Status

Accepted as the architecture foundation after v0.4.7 and before v0.4.8.

## Context

BaseHarbor must support applications that evolve continuously. A repository may begin with PostgreSQL, add Redis later, then object storage, metrics, telemetry or other capabilities. Repository inspection therefore cannot be a one-time bootstrap helper, and the application contract cannot be treated as a frozen install recipe.

The same application may also need two different resource moments:

1. deployment-time resources that BaseHarbor provisions and binds before workloads start;
2. application-time resources that an already running workload may request later, for example one logical S3 storage resource per tenant.

At the same time, the portable contract must stay small. Rich repository evidence and provider detail must not be copied blindly into committed YAML.

## Decision

### 1. The application contract is sparse desired state

Applications declare only capabilities they require and only values that differ from safe defaults. Omitted optional capabilities mean not requested. Writers must not emit explicit negative configuration merely to enumerate every known capability.

Manifest v1 readers remain backward compatible with existing explicit `enabled: false` input.

### 2. Inspection is continuous and delta-oriented

`baha app inspect` is safe to run repeatedly throughout development.

Inspection produces repository evidence and reconciles it against the explicit contract into four states:

- `satisfied`: declared intent has supporting repository evidence;
- `new`: strong evidence exists for an undeclared capability;
- `ambiguous`: weaker evidence exists and requires developer judgment;
- `stale`: declared intent was not rediscovered in the current inspection.

A stale result is informational only. It never means removal.

### 3. Explicit contract wins over inference

Repository inspection is read-only and advisory. It never overwrites an existing manifest, never removes a capability because evidence disappeared, and never grants runtime authorization from source-code evidence.

Additions may be proposed as minimal deltas. Retirement is always an explicit developer/operator decision.

### 4. Capability direction is first-class inspection semantics

Evidence may describe that an application:

- `consume`s a capability;
- `provide`s/exposes a capability;
- `export`s data;
- `receive`s data;
- may `provision` logical resources at runtime.

The first concrete examples are PostgreSQL/Redis/S3 consumption, OpenMetrics provided through an application endpoint, and OTLP export.

### 5. Runtime operations belong to the capability, not the product

A capability may later expose authorized runtime operations such as:

- `runtime.create`;
- `runtime.get`;
- `runtime.delete`;
- `runtime.rotate`.

For example, source code that calls an S3-compatible `CreateBucket` operation can be reported as evidence that the application may need `object-storage.s3` `runtime.create`. That evidence does not itself authorize bucket creation.

Static and runtime resources remain under the same logical capability and provider contract. BaseHarbor must not create a parallel SeaweedFS/AWS/Ceph-specific runtime architecture.

### 6. Runtime Resource API uses OpenAPI and environment-scoped interactive docs

The application-facing Runtime Resource API uses HTTP/JSON with a versioned OpenAPI 3.1 contract. OpenAPI is normative; Swagger UI, Scalar, Redoc or another renderer is presentation only.

Interactive documentation follows deployment policy:

- development: enabled by default;
- test/staging: disabled by default and opt-in;
- production: disabled by default and opt-in.

This setting is not application intent and must not become a portable `baseharbor.yaml` field. Enabling interactive documentation never changes API authentication/authorization.

Mutating runtime operations require idempotency, may return asynchronous operation identities, and use the existing secure-binding/runtime-identity boundary for credentials.

### 7. Inspection may be richer than the committed manifest

The inspection model may retain evidence paths, confidence, capability direction and runtime-operation hints. The committed application contract stays intentionally smaller and provider-neutral.

This allows the system to understand code evolution without turning every detected implementation detail into YAML.

## Consequences

- BaseHarbor can accompany an application from first bootstrap through later capability additions.
- Existing manifest v1 files remain readable.
- Canonical manifest output becomes sparse and omits disabled capabilities.
- Future `baha up` reconciliation can reuse the same inspect/reconcile core instead of inventing a second scanner.
- Future runtime resource APIs must authorize operations explicitly and resolve them through the existing capability/provider boundary.
- Metrics/Prometheus work in v0.4.8 can model application-provided OpenMetrics separately from the selected metrics backend/provider.
- Missing evidence can never cause destructive infrastructure changes.
