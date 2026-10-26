# Managed Service Access Inventory

Status: normative inventory for the current v0.4.17 Compose reference runtime.

This inventory records the actual managed network-service boundary behind issue #397. It describes deployment/runtime behavior only. None of these provider products or concrete security mechanisms become portable application intent.

## Environment policy

| Environment | TLS | Human/developer access | Workload/service auth |
| --- | --- | --- | --- |
| dev | mandatory | trusted-local operator mode; local/loopback access is zero-ceremony and no `baha login` is required | native credentials or automatically projected service bindings; mTLS remains available where the runtime already uses it |
| test | mandatory | BaseHarbor application operations require an authenticated Target/Environment OIDC operator; management UIs remain restricted/non-public by default | protected generated credentials, mTLS or another configured service-access mechanism |
| prod | mandatory | BaseHarbor application operations require an authenticated Target/Environment OIDC operator; no anonymous management/observability access | authentication is mandatory; native auth, mTLS, scoped token or a configured external adapter may satisfy policy |

For shared HTTP providers, the service-access state never silently downgrades from an authentication-required environment back to anonymous access when a later reconciliation only sees development consumers.

Loopback and internal networks are defense in depth. They are not treated as authentication.

## Current managed services

| Managed service | Exposure | Transport | Authentication / authorization | Credential / trust ownership | Enforcement boundary |
| --- | --- | --- | --- | --- | --- |
| Control-plane PostgreSQL | host loopback plus control-plane network | native PostgreSQL TLS | native PostgreSQL credentials; control-plane `pg_hba.conf` requires TLS and SCRAM-SHA-256 | generated BaseHarbor protected state; certificate lifecycle through the selected service-access issuer | PostgreSQL native TLS/auth |
| Application PostgreSQL | host loopback plus application backend network | native PostgreSQL TLS | generated per-application database credentials | generated BaseHarbor application runtime state; CA projected through secure file binding | PostgreSQL native TLS/auth |
| Valkey / Redis-compatible cache | application backend network; host access through loopback TLS gateway | TLS gateway in front of the native Valkey service | generated per-application `requirepass` credential | generated BaseHarbor application runtime state; CA projected through secure file binding | Valkey native password + shared TCP TLS gateway |
| OpenBao | control-plane network plus host loopback; optional web UI uses the same native listener | native HTTPS/TLS | OpenBao native authentication; manager operations use least-privilege AppRole policy | OpenBao owns managed-local issuer state and CA private key; manager credentials are protected state | OpenBao 2.7 native TLS/auth with `tls_auto_reload` for certificate replacement |
| SeaweedFS S3 | provider network plus host loopback; optional shared Admin UI remains a separate HTTPS adapter | native S3 HTTPS | native S3 access key/secret for applications; Admin UI keeps its dedicated browser access policy | provider-admin credentials remain inside provider state; application credentials are scoped protected bindings | SeaweedFS native TLS/S3 auth; only the Admin UI retains an adapter |
| Managed Keycloak identity | provider networks plus one native loopback HTTPS listener; canonical dev login/admin hosts route through the Target gateway | native Keycloak HTTPS | standard OIDC/OAuth2 for applications/users; Keycloak-native admin authentication remains provider administration | application client secret is protected application binding state; provider-admin credentials remain provider state | Keycloak native TLS/OIDC; certificates reload every 30s; Target gateway supplies canonical browser routing |
| pgAdmin companion | application runtime; host loopback only | native HTTPS | pgAdmin-native login; managed PostgreSQL connections are preconfigured through protected runtime state | UI login and database credentials remain BaseHarbor application runtime state and are not application intent | app-scoped optional management UI |
| Redis Commander companion | application runtime; host loopback through HTTPS proxy | HTTPS/TLS gateway + HTTP Basic | generated UI Basic Auth plus application-scoped cache credential behind the UI | UI and cache credentials remain BaseHarbor application runtime state | app-scoped optional management UI |
| Prometheus | provider/internal networks; native HTTPS on host loopback and canonical dev route through the Target gateway | native Prometheus HTTPS | dev uses native Basic Auth when management credentials are configured; managed test/prod use native mTLS | certificate lifecycle through selected issuer; web TLS configuration is re-read on every request | Prometheus native web TLS/auth |
| Loki | internal/provider networks; developer endpoint on host loopback | HTTPS/TLS gateway | same environment-aware service-access policy as Prometheus; application ingestion remains registration/scoping controlled | certificate lifecycle through selected issuer; collector/provider state remains BaseHarbor-owned | shared HTTP service-access gateway plus Loki/Alloy registration boundary |
| Tempo | trace provider network; developer endpoint on host loopback | HTTPS/TLS gateway | environment-aware service-access auth; mTLS is the current managed default where authentication is required | certificate lifecycle through selected issuer | shared HTTP service-access gateway |
| OpenTelemetry Collector / OTLP | telemetry network; developer/test endpoint on host loopback when managed | native collector HTTPS | environment-aware service-access auth; managed test/prod use native mTLS; workload CA/client material is projected automatically | certificate/client material is protected runtime binding, not application intent; collector reloads certificates every 30s | OpenTelemetry receiver native TLS/mTLS |
| Application Runtime Broker | application backend/control networks; docs listener loopback only | native HTTPS | application-scoped mTLS identity plus protected runtime bearer/service tokens | leaf identities issued through managed issuer; app token and permissions are application-scoped protected state | runtime broker |
| Runtime Provider Executor | internal `baseharbor-runtime-control` network only; no host-published API port | native HTTPS | requires and verifies client certificate | executor leaf identity through managed issuer; provider-admin credentials stay inside executor boundary | runtime executor |
| Managed HTTP exposure | provider/application exposure network and configured listener | HTTPS/TLS according to deployment TLS state | application-facing auth remains application/provider responsibility unless a later identity/auth capability is selected | current public ingress certificate lifecycle remains separate deployment TLS state | exposure provider |
| Grafana | not currently instantiated by the v0.4.17 reference runtime | n/a | n/a | n/a | n/a |

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


## Native-TLS-first topology rule

The v0.4.17 reference runtime follows this order:

1. use provider-native TLS when the provider can enforce the required transport and authentication policy;
2. use the single Target-scoped Developer Gateway for canonical browser routing in development;
3. retain a dedicated adapter only when it adds a security or protocol property the native endpoint does not provide.

Current justified adapters are:

- Valkey TCP access, which adds TLS around the password-authenticated Valkey protocol;
- Redis Commander/cache management UI, which has no equivalent native HTTPS listener in the selected component;
- SeaweedFS Admin UI, while S3 itself uses native HTTPS;
- Loki and Tempo host/API access, because their current BaseHarbor topology also contains internal Alloy/collector ingestion and provider-scrape relationships that remain intentionally isolated on clear internal transport; converting those edges requires a coordinated client-side TLS migration rather than merely deleting a proxy;
- managed application exposure, where the proxy is the actual ingress/exposure provider rather than a service-access TLS wrapper.

Keycloak, OpenBao, PostgreSQL, SeaweedFS S3, Prometheus and OTLP no longer require a dedicated Caddy container merely to obtain TLS.
