# Provider and management-surface acceptance v1

This matrix is normative for v0.4.21 provider/access/availability acceptance.

Legend: source `managed-secret` means the selected managed secret provider is authoritative; `protected-state` is BaseHarbor-owned protected machine state; projections are not a second durable truth. HA is the guarantee the shipped BaseHarbor reference realization actually proves.

| Provider / surface | Credential class | Authoritative source | Delivery / projection | Human auth class | Role mapping | Environment policy | Rotation | HA |
|---|---|---|---|---|---|---|---|---|
| PostgreSQL application binding | B | managed-secret/protected provider state | secure binding / protected runtime projection | n/a | n/a | app/environment/resource scoped | reconciliation lifecycle | UNSUPPORTED: current managed topology is single-instance |
| pgAdmin | A | managed-secret / resolved management credential | protected management projection | native-credential | limited provider-native mapping | dev shared; test/prod isolated by default | reconciliation lifecycle | UNSUPPORTED |
| Valkey cache / durable key-value | B | managed-secret/protected provider state | secure binding / protected runtime projection | n/a | n/a | app/environment/resource scoped | reconciliation lifecycle | UNSUPPORTED: current managed topology is single-instance |
| Cache management UI | A | managed-secret / resolved management credential | protected management projection | standards-auth-adapter | limited | dev shared; test/prod isolated by default | reconciliation lifecycle | UNSUPPORTED |
| MongoDB document database | B | managed-secret/protected provider state | secure binding / protected runtime projection | n/a | n/a | app/environment/resource scoped | reconciliation lifecycle | UNSUPPORTED: no verified replica set |
| MongoDB management UI | A | managed-secret / resolved management credential | protected management projection | standards-auth-adapter | limited | dev shared; test/prod isolated by default | reconciliation lifecycle | UNSUPPORTED |
| RabbitMQ messaging | B | managed-secret/protected provider state | secure binding / protected runtime projection | n/a | n/a | app/environment/resource scoped | reconciliation lifecycle | UNSUPPORTED: no verified cluster |
| RabbitMQ Management | A | managed-secret / provider-native user | protected management projection | native-credential | limited native mapping | dev shared; test/prod isolated by default | reconciliation lifecycle | UNSUPPORTED |
| SeaweedFS S3 | B | managed-secret/protected provider state | secure binding / runtime resolution | n/a | n/a | bucket/application scoped | reconciliation lifecycle | UNSUPPORTED: no verified HA topology |
| SeaweedFS Admin | A | managed-secret / resolved management credential | protected management projection | native-credential | limited native mapping | dev shared; test/prod isolated by default | reconciliation lifecycle | UNSUPPORTED |
| OpenBao secret API | C for control; B for application secrets | OpenBao | opaque references/runtime resolution preferred | n/a | provider policy | always isolated for machine identity | native/reconcile | UNSUPPORTED: one managed server |
| OpenBao UI | A | managed-secret / provider-native credential | protected management projection | native-credential | limited native mapping | dev shared; test/prod isolated by default | reconciliation lifecycle | UNSUPPORTED |
| Keycloak application login | A | Keycloak managed identity | OIDC/OAuth2 | native-oidc | exact BaseHarbor management profile where configured | central OIDC preferred | provider/native lifecycle | UNSUPPORTED: current reference realization single-instance |
| Keycloak Admin | A | managed-secret / provider-native admin | protected management projection | native-credential | limited native mapping | dev shared fallback; test/prod isolated default | reconciliation lifecycle | UNSUPPORTED |
| Prometheus | C for service access | protected-state/managed PKI | TLS/mTLS or native credential projection | n/a | n/a | environment-aware | cert/credential lifecycle | UNSUPPORTED: single-instance |
| Prometheus management surface | A | managed access policy | native credential / protected projection | native-credential or explicit unsupported when anonymous | limited | dev convenience; test/prod authenticated | reconciliation lifecycle | UNSUPPORTED |
| Loki | C | protected-state/managed PKI | protected runtime projection | n/a | n/a | isolated | cert/config lifecycle | UNSUPPORTED |
| Tempo | C | protected-state/managed PKI | protected runtime projection | n/a | n/a | isolated | cert/config lifecycle | UNSUPPORTED |
| OpenTelemetry Collector | C | protected-state/managed PKI | protected runtime projection | n/a | n/a | isolated | cert/config lifecycle | UNSUPPORTED |
| Caddy exposure | C for managed boundary | protected-state/managed PKI | protected runtime projection | n/a | n/a | isolated | certificate/config lifecycle | UNSUPPORTED |
| External OIDC / OTLP | provider-declared | external provider | provider contract | provider-declared | provider-declared | explicit external policy | provider-declared | UNSUPPORTED until an external provider explicitly declares and proves the requested guarantee |

## Acceptance rules

- Class C machine credentials are never replaced by shared developer/human credentials.
- Application business users/roles are not represented in this matrix.
- A management surface may claim only `native-oidc`, `standards-auth-adapter`, `native-credential` or `unsupported`.
- Role mappings are exact, limited or unsupported; no provider is credited with finer authorization than it enforces.
- Secret-bearing authoritative host state stays owner-only. Broader runtime readability is a separate read-only projection.
- Effective HA requests against every current UNSUPPORTED row fail before mutation.
- A future provider may change only its realization/classification, not the portable application contract.
