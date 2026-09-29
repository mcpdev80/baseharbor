# Use PostgreSQL

Declare that the application needs SQL. Do not make PostgreSQL-specific deployment topology part of portable application intent unless the capability contract requires a PostgreSQL-specific semantic.

Typical flow:

```bash
baha app inspect .
baha app init
baha plan
baha up -e dev
baha doctor
```

The application consumes the binding BaseHarbor provides, typically through a standard database URL.

BaseHarbor owns lifecycle only for resources it owns. Shared or external database providers keep their own lifecycle boundary.

For exact fields, see [Manifest reference](../reference/manifest.md).

## Shared PostgreSQL isolation

The default managed PostgreSQL placement is `shared`: one Target-owned PostgreSQL provider can serve multiple applications without sharing application access.

The security boundary is:

```text
one shared PostgreSQL provider
├── baseharbor_admin        BaseHarbor control plane only
├── app-a/dev/default      own database + own role + own credential
├── app-a/dev/analytics    own database + own role + own credential
└── app-b/dev/default      own database + own role + own credential
```

`baseharbor_admin` is a provider-administration identity. BaseHarbor uses it only for provider lifecycle operations such as creating/dropping databases and roles, ownership reconciliation, repair and provider-level verification. It is never projected into an application environment, Service Binding, status, evidence or normal logs.

Every logical SQL resource receives a deterministic collision-safe PostgreSQL role, a dedicated database and a protected application credential. Role/database identity includes Application, Environment and SQL instance. BaseHarbor revokes public database/schema access and verifies both positive and negative isolation:

```text
App A role -> App A database  ALLOW
App A role -> App B database  DENY
App B role -> App B database  ALLOW
App B role -> App A database  DENY
```

Set `BASEHARBOR_PROVIDER_POSTGRESQL_SCOPE=application` only when a deliberately dedicated PostgreSQL provider is required.

