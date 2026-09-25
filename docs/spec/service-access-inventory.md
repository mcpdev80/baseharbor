# Managed Service Access Inventory

Status: normative inventory for the current v0.4.15 Compose reference runtime.

This inventory records the actual managed network-service boundary behind issue #397. It describes deployment/runtime behavior only. None of these provider products or concrete security mechanisms become portable application intent.

## Environment policy

| Environment | TLS | Human/developer access | Workload/service auth |
| --- | --- | --- | --- |
| dev | mandatory | local/loopback access is zero-ceremony; no developer-held mTLS certificate or copied token is required | native credentials or automatically projected service bindings; mTLS remains available where the runtime already uses it |
| test | mandatory | fully automatable | protected generated credentials, mTLS or another configured service-access mechanism |
| prod | mandatory | no anonymous management/observability access | authentication is mandatory; native auth, mTLS, scoped token or a configured external adapter may satisfy policy |

For shared HTTP providers, the service-access state never silently downgrades from an authentication-required environment back to anonymous access when a later reconciliation only sees development consumers.

Loopback and internal networks are defense in depth. They are not treated as authentication.

## Current managed services

| Managed service | Exposure | Transport | Authentication / authorization | Credential / trust ownership | Enforcement boundary |
| --- | --- | --- | --- | --- | --- |
| Control-plane PostgreSQL | host loopback plus control-plane network | native PostgreSQL TLS | native PostgreSQL credentials; control-plane `pg_hba.conf` requires TLS and SCRAM-SHA-256 | generated BaseHarbor protected state; certificate lifecycle through the selected service-access issuer | PostgreSQL native TLS/auth |
| Application PostgreSQL | host loopback plus application backend network | native PostgreSQL TLS | generated per-application database credentials | generated BaseHarbor application runtime state; CA projected through secure file binding | PostgreSQL native TLS/auth |
| Valkey / Redis-compatible cache | application backend network; host access through loopback TLS gateway | TLS gateway in front of the native Valkey service | generated per-application `requirepass` credential | generated BaseHarbor application runtime state; CA projected through secure file binding | Valkey native password + shared TCP TLS gateway |
| OpenBao | control-plane network; host loopback through HTTPS gateway | HTTPS/TLS gateway | OpenBao native authentication; manager operations use least-privilege AppRole policy | OpenBao owns managed-local issuer state and CA private key; manager credentials are protected state | OpenBao native auth behind shared HTTPS access layer |
| SeaweedFS S3 | provider network; host loopback through HTTPS gateway | HTTPS/TLS gateway | native S3 access key/secret; bucket/user actions scoped by generated SeaweedFS S3 user policy | provider-admin credentials remain inside provider/executor boundary; application credentials are scoped protected bindings | SeaweedFS native S3 auth behind shared HTTPS access layer |
| Prometheus | provider/internal networks; developer endpoint on host loopback | HTTPS/TLS gateway | dev resolves to zero-ceremony local TLS access; test/prod require selected service-access auth, currently mTLS by default unless another configured adapter is selected | certificate lifecycle through selected issuer; client material is protected/generated state | shared HTTP service-access gateway |
| Loki | internal/provider networks; developer endpoint on host loopback | HTTPS/TLS gateway | same environment-aware service-access policy as Prometheus; application ingestion remains registration/scoping controlled | certificate lifecycle through selected issuer; collector/provider state remains BaseHarbor-owned | shared HTTP service-access gateway plus Loki/Alloy registration boundary |
| Tempo | trace provider network; developer endpoint on host loopback | HTTPS/TLS gateway | environment-aware service-access auth; mTLS is the current managed default where authentication is required | certificate lifecycle through selected issuer | shared HTTP service-access gateway |
| OpenTelemetry Collector / OTLP | telemetry network; developer/test endpoint on host loopback when managed | HTTPS/TLS gateway | environment-aware service-access auth; current managed test/prod default is mTLS; workload CA/client material is projected automatically | certificate/client material is protected runtime binding, not application intent | shared HTTP service-access gateway |
| Application Runtime Broker | application backend/control networks; docs listener loopback only | native HTTPS | application-scoped mTLS identity plus protected runtime bearer/service tokens | leaf identities issued through managed issuer; app token and permissions are application-scoped protected state | runtime broker |
| Runtime Provider Executor | internal `baseharbor-runtime-control` network only; no host-published API port | native HTTPS | requires and verifies client certificate | executor leaf identity through managed issuer; provider-admin credentials stay inside executor boundary | runtime executor |
| Managed HTTP exposure | provider/application exposure network and configured listener | HTTPS/TLS according to deployment TLS state | application-facing auth remains application/provider responsibility unless a later identity/auth capability is selected | current public ingress certificate lifecycle remains separate deployment TLS state | exposure provider |
| Grafana | not currently instantiated by the v0.4.15 reference runtime | n/a | n/a | n/a | n/a |

## PKI / trust source models

The same service-access boundary accepts:

- `managed-local`: current reference issuer is OpenBao PKI;
- `external-pki`: static external material or an issuer-backed integration identified by a provider-neutral issuer reference;
- `byoc`: static operator-owned certificate/key/trust material;
- external trust bundles;
- future runtime/platform issuers, including in-cluster Kubernetes/OpenShift implementations.

Changing the trust/issuer realization does not change portable application intent.

## Certificate lifecycle responsibility

| Source | Owner | Renewal mode | BaseHarbor behavior |
| --- | --- | --- | --- |
| managed-local | selected issuer provider | automatic reconcile | inspect expiry, renew through `Issuer.Renew`, atomically project replacement material and let normal provider/runtime reconciliation roll it out |
| external-pki with issuer adapter | external issuer integration | automatic reconcile when supported | require exact issuer-reference match, request renewal through the same `Issuer` contract and preserve workload binding shape |
| external-pki static files | external/operator | replace and reconcile | validate material and surface expiry state; BaseHarbor does not claim issuer ownership |
| byoc | operator | replace and reconcile | validate key/certificate/trust, warn before expiry and atomically consume replacement material on reconcile |

The lifecycle observation exposed to status/doctor is secret-safe and includes only source, owner, renewal mode, issuer reference, expiry and health. It never contains private keys, bearer tokens or credential values.

## Workload projection boundary

Connection metadata and file references may be projected into workload environment variables. Certificate and key contents are not.

Examples:

```text
DATABASE_URL=postgresql://...
DATABASE_CA_FILE=/run/baseharbor/bindings/postgres/default/ca.pem

REDIS_URL=rediss://...
REDIS_CA_FILE=/run/baseharbor/bindings/valkey/default/ca.pem

AWS_CA_BUNDLE=/run/baseharbor/tls/s3/ca.pem
OTEL_EXPORTER_OTLP_CERTIFICATE=/run/baseharbor/tls/otlp/ca.pem
```

The referenced CA/certificate/key material is mounted as read-only or protected secret files. This is the stable boundary that a future Kubernetes/OpenShift runtime can realize through Secret/ConfigMap/CSI-style projections without changing application intent.
