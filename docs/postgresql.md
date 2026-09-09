# PostgreSQL and migrations

BaseHarbor uses PostgreSQL for durable platform state and `pgx/v5` for database access.

## Connection lifecycle

The database package validates configuration, opens a bounded pgx pool and performs a real ping before returning a usable connection. Failed startup closes the pool immediately.

## Migrations

SQL migrations are embedded in the BaseHarbor binary and applied in lexical filename order.

Each migration:

1. runs in its own transaction,
2. is recorded in `schema_migrations` only after successful execution,
3. rolls back completely on error,
4. is skipped after it has been recorded successfully.

The first schema contains only platform-level identity data:

- `tenants`
- `external_identities`
- `memberships`

Application-specific schemas do not belong in BaseHarbor core migrations.

## Row-level security

RLS is intentionally not enabled yet. BaseHarbor first needs a trustworthy contract for propagating tenant identity into each database transaction/session and separate runtime versus administrative database roles. RLS will be introduced only together with those controls so it represents a real isolation boundary rather than a policy that privileged connections can silently bypass.

## Dependency maintenance

PostgreSQL driver and related Go dependencies are maintained by the repository-wide Renovate policy. Renovate checks dependencies daily, but only low-risk patch/pin/digest updates are eligible for automerge after the release-age buffer and green required checks. Minor/major updates plus security-sensitive authentication and cryptography changes require explicit human review.
