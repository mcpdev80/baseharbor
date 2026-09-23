# PostgreSQL and migrations

BaseHarbor uses PostgreSQL as its primary durable control-plane store.

## Driver

The database layer uses `pgx/v5` and `pgxpool` directly. No ORM is introduced at this stage.

The connection layer:

- parses a PostgreSQL DSN
- supports explicit min/max pool sizing
- applies a bounded initial connection timeout
- returns only after a successful `Ping`
- closes the pool on failed startup verification

## Migrations

SQL migrations are embedded into the BaseHarbor binary from `internal/database/migrations`.

Forward migrations keep their stable historical names, for example:

```text
0001_core_identity.sql
0002_tenant_rls.sql
```

Optional rollback files use the same version stem plus `.down.sql`:

```text
0001_core_identity.down.sql
0002_tenant_rls.down.sql
```

The forward runner:

1. creates `baseharbor_schema_migrations` if needed
2. loads embedded forward `.sql` files while ignoring `.down.sql` files
3. applies them in lexical order
4. skips versions already recorded
5. runs each migration in a transaction
6. records a migration only inside the same successful transaction

A failed migration is therefore not marked as applied.

`RollbackLast` reverses exactly the most recently applied migration and requires an explicit matching rollback file. It never guesses how to undo schema. The rollback SQL and deletion of the migration record run in the same transaction.

A rollback migration must only undo objects or settings owned by its matching forward migration. In particular, rolling back a later migration must not remove schema created by an earlier migration.

## Core schema

The core schema currently contains only identity and tenancy primitives:

- `tenants`
- `external_identities`
- `memberships`

Application-specific tables do not belong in the BaseHarbor core schema.

## Database-enforced tenant isolation

Tenant-bound database access uses transaction-local PostgreSQL context.

`WithTenantTx` validates the tenant UUID, begins a transaction and sets:

```sql
SELECT set_config('baseharbor.tenant_id', '<tenant-uuid>', true);
```

The final `true` makes the setting transaction-local. It is discarded automatically on commit or rollback and therefore cannot intentionally persist as tenant state on a pooled connection.

`memberships` has row-level security enabled and forced. Its policy compares `tenant_id` with `current_setting('baseharbor.tenant_id', true)::uuid` for both reads and writes.

Consequences:

- no tenant context exposes no tenant membership rows
- tenant A cannot read tenant B memberships
- tenant A cannot insert or update rows into tenant B
- application code does not need to remember to add a tenant predicate to every query
- `FORCE ROW LEVEL SECURITY` also prevents ordinary table owners from silently bypassing policies

Global tables such as `tenants` and `external_identities` are not currently tenant-scoped. Future tenant-bound core or module tables must receive equivalent RLS policies as part of the migration that creates them.

## Database roles

`deploy/postgres/roles.sql` defines two non-login capability roles:

- `baseharbor_runtime`
- `baseharbor_migrator`

Both are explicitly `NOSUPERUSER` and `NOBYPASSRLS`.

Runtime service identities should receive the `baseharbor_runtime` capability only. Administrative/bootstrap credentials must not be used by normal BaseHarbor request handling.

Role reconciliation is deliberately separate from schema migrations because PostgreSQL role creation requires elevated cluster privileges. The future `baha` provisioning lifecycle will create/reconcile roles with an administrative connection, apply migrations with the migration identity, then reconcile runtime grants.

## Verification

CI uses a real PostgreSQL service in the existing test job. Integration tests prove that:

- access without tenant context returns no membership rows
- tenant A sees only tenant A rows
- direct lookup of tenant B data from tenant A is hidden
- cross-tenant writes are rejected by PostgreSQL
- malformed tenant IDs are rejected before tenant scope is established
- rolling back the RLS migration leaves the earlier core identity schema intact
- rolling back the core identity migration removes only its own schema

Tenant isolation and migration rollback ownership are therefore tested as PostgreSQL boundaries rather than only as application conventions.
