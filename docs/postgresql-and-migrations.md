# PostgreSQL and migrations

BaseHarbor uses PostgreSQL as its primary durable store.

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

The runner:

1. creates `baseharbor_schema_migrations` if needed
2. loads embedded `.sql` files
3. applies them in lexical order
4. skips versions already recorded
5. runs each migration in a transaction
6. records a migration only inside the same successful transaction

A failed migration is therefore not marked as applied.

## Initial core schema

The first migration contains only identity and tenancy primitives:

- `tenants`
- `external_identities`
- `memberships`

Application-specific tables do not belong in the BaseHarbor core schema.

## Row-level security

PostgreSQL RLS is a BaseHarbor goal, but it is intentionally not enabled by this migration yet.

RLS is only useful when the database session receives a trustworthy tenant identity and application database roles cannot bypass the policy accidentally. Enabling policies before the request/session propagation contract exists would create false confidence.

The RLS follow-up must therefore include together:

- trusted tenant context propagation into the PostgreSQL session or transaction
- deny-by-default policies
- separation of migration/administrative and runtime database roles
- tests proving cross-tenant reads and writes are denied

Until that layer exists, tenant isolation must not be claimed as database-enforced.
