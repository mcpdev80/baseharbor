# Standards matrix

This matrix is the pre-v0.5 standards baseline for service/provider work.

| Area | Standard / pattern | Current version | Classification | BaseHarbor use |
| --- | --- | --- | --- | --- |
| Schemas | JSON Schema | 2020-12 | open standard | canonical machine-readable portable service/provider configuration schema |
| Service connection output | Service Binding Specification | 1.1.0 | open ecosystem specification | well-known connection entry names and workload projection semantics |
| Provider/resource architecture | Crossplane | v2.4 reference | established architecture pattern | desired/observed managed-resource/provider/reconciliation concepts; no Kubernetes API leakage into portable intent |
| Broker lifecycle | Open Service Broker API | 2.17 | open API specification | catalog/plan/provision/bind/unbind/deprovision/async lifecycle concepts where useful |
| Artifact distribution | OCI Image/Distribution | 1.1 family | open standard | registry-neutral provider artifacts, digest-first identity, multi-platform distribution |
| Observability | OpenTelemetry / OTLP | OTel 1.61.0 / OTLP 1.11.0 | open standard/ecosystem specification | traces/metrics/logs data plane and standard workload variables |
| Identity | OpenID Connect Core | 1.0 + Errata Set 2 | open identity standard | authentication/identity baseline; OAuth-based authorization where applicable |
| Messaging description | AsyncAPI | 3.1.0 | open specification | message/channel API description |
| Event envelope | CloudEvents | 1.0.2 | CNCF specification | event envelope where event semantics apply |
| Cache wire compatibility | RESP | RESP2/RESP3 | de-facto protocol standard | provider compatibility declaration for Redis/Valkey-compatible implementations |
| Object storage API | Amazon S3 REST API | API 2006-03-01 | de-facto ecosystem API | optional protocol compatibility declaration; never generic object-storage identity |

## Service-family decisions

| Service | KEEP | ALIGN | BaseHarbor extension required |
| --- | --- | --- | --- |
| SQL | provider-neutral SQL intent, provider lifecycle/conformance | Service Binding connection outputs; Crossplane-like resource pattern | small common SQL semantic contract because no complete neutral SQL service standard exists |
| Cache | generic cache/key-value intent | RESP compatibility is provider capability, not service identity | demonstrated semantic subsets such as operations/features not covered by RESP alone |
| Object storage | logical application-owned storage resources | separate generic object-storage service from S3 API compatibility | lifecycle/ownership/verification semantics |
| Secrets | secret requirements/references/security lifecycle | standard Service Binding names for connection material | identity refs, authorization, rotation/revocation/ownership |
| Messaging | generic provider-neutral intent | AsyncAPI + CloudEvents; protocols remain capabilities | BaseHarbor lifecycle/ownership/verification |
| Vector | provider-neutral intent only | inspect existing APIs before adding semantics | minimal common vector semantics; no accepted neutral service contract exists |
| Observability | backend-neutral intent/provider resolution | OpenTelemetry/OTLP is authoritative telemetry plane | policy/placement/verification only |
| Identity | provider-neutral identity requirement | OIDC/OAuth authoritative where applicable | provider lifecycle/placement/policy integration only |

## Gap classification

### KEEP

- provider-neutral application intent;
- capability/provider separation;
- provider placement and ownership;
- desired/observed/reconcile/verify;
- Provider Integration Contract;
- gRPC/Protocol Buffers process boundary;
- OCI artifact direction;
- provider conformance;
- independent provider implementation version;
- OTLP workload integration.

### ALIGN

- `secure-binding/v1`: keep BaseHarbor security/lifecycle extensions, use Service Binding 1.1 names for connection outputs.
- `object-storage.s3/v1`: retain compatibility while separating generic object-storage service identity from S3 protocol compatibility.
- `telemetry.otlp/v1`: retain as protocol/transport capability beneath backend-neutral observability intent.
- `metrics/v1`, `logs/v1`, `traces/v1`: ensure they do not define a competing telemetry wire protocol.
- runtime Provider Registry: keep instance/ownership state separate from future Provider Catalog metadata.

### REPLACE

No current provider/runtime primitive requires wholesale replacement.

Any product-specific field found in a portable service contract must instead move to provider configuration/extension state.

### BASEHARBOR EXTENSION

BaseHarbor still needs its own versioned semantics for:

- placement and lifecycle ownership;
- desired/observed reconciliation and drift;
- end-to-end verification/conformance;
- provider catalog metadata not covered by OCI itself;
- secure references/authorization/lifecycle metadata not covered by Service Binding;
- minimal vector-service semantics;
- compatibility/migration classification across BaseHarbor service contracts.

## Freeze rule

Before v0.5.x freezes contracts, each shipped v0.4 capability specification must be classified and, where required, receive an explicit compatibility migration. No compatibility identifier is removed or renamed solely to make the new model look cleaner.
