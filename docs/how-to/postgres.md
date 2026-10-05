# Run an order API with PostgreSQL

Create a Go API with SQL intent, then inspect and query its application-owned database. With `baha` installed, run from a parent directory:

```bash
baha app new orders-api --stack go --http --sql
cd orders-api
baha plan
baha up -e dev
baha app env --format json
```

`up` requires a configured local Docker or Podman Target. The scaffold adds `pgx`, declares `DATABASE_URL` and `DATABASE_CA_FILE`, and verifies database connectivity at startup. Protected bindings supply runtime credentials; normal environment output masks the database URL.

## Insert and read a sample order

After successful deployment, open the application's SQL instance:

```bash
baha app psql
```

In that PostgreSQL session:

```sql
CREATE TABLE IF NOT EXISTS orders (id integer PRIMARY KEY, total_cents integer NOT NULL);
INSERT INTO orders VALUES (42, 1990) ON CONFLICT (id) DO NOTHING;
SELECT id, total_cents FROM orders WHERE id = 42;
```

The query returns the sample order with a total of 1990 cents. Exit with `\q`, then run `baha doctor` to verify the SQL capability. This exercises application access, not provider-administrator credentials.

Use [backup and restore](backup-restore.md) before relying on recovery of this order. For an existing API repository, use `baha app inspect .` and `baha app init` instead of creating a second scaffold.

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

