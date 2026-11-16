# Service contract model

BaseHarbor service contracts describe what an application needs without selecting a provider product.

## Canonical layers

```text
service kind
  -> service contract version
  -> optional protocol/semantic requirements
  -> provider
  -> product/engine
  -> runtime realization
```

Provider placement, product versions, runtime topology, credentials and artifact digests are not portable application intent.

## Service families

| Service kind | Standards baseline | BaseHarbor-specific semantics |
| --- | --- | --- |
| `sql` | Service Binding 1.1 for connection outputs; SQL/provider ecosystem conventions | small provider-neutral SQL resource/semantic contract |
| `cache` | Service Binding 1.1; RESP as compatibility signal for Redis/Valkey-compatible providers | cache intent and required semantic subsets |
| `object-storage` | Service Binding 1.1; S3 API as de-facto compatibility capability | logical object-storage resources and required semantics |
| `secrets` | Service Binding 1.1 where values represent service connection material | secret requirements, references, lifecycle/security metadata |
| `messaging` | AsyncAPI; CloudEvents where event envelopes apply | provider-neutral messaging requirement and semantic subsets |
| `vector` | no accepted neutral service standard | minimal common BaseHarbor semantics only |
| `observability` | OpenTelemetry/OTLP | intent/policy and backend-neutral selection |
| `identity` | OpenID Connect/OAuth | application identity requirements and provider-neutral policy |

## Version axes

BaseHarbor keeps these independent:

1. BaseHarbor service contract version.
2. Provider protocol version.
3. Provider implementation version.
4. Product/engine version.
5. Artifact digest/identity.

A provider upgrade does not imply a service-contract change. A product upgrade does not imply a provider-protocol change.

## JSON Schema

Portable service contract structure is expressed using JSON Schema 2020-12 under `contracts/service/v1/`.

The schemas are provider-neutral. Provider configuration uses separate provider schemas and MUST NOT add product fields to the portable service contract.

## Existing capability specifications

The existing `spec/capabilities/` documents continue to define shipped v0.4 semantics until an explicit compatibility migration is approved. The new service schemas provide the standards-first target model and freeze boundary; they do not silently rewrite deployed state.
