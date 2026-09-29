# Back up and restore

A BaseHarbor backup is not successful only because an archive or snapshot exists.

## Back up

```bash
baha backup
```

BaseHarbor must verify the backup and record enough ownership and compatibility metadata to support recovery.

## Restore

Use the supported restore flow for the application and environment.

A successful restore includes:

```text
preflight
-> restore state/data
-> rebind
-> reconcile
-> verify
```

BaseHarbor must not report recovery success before the restored application capabilities are verified.

Provider-owned data is backed up through the provider that understands its data semantics.

## Shared PostgreSQL

Application backup remains application-scoped even when PostgreSQL infrastructure is shared.

`baha app backup` derives the SQL backup set from the protected provider registration for the selected Application + Environment and dumps only that application's registered databases. It never uses `pg_dumpall`, sibling databases or provider-global state.

For a multi-instance application:

```text
app-a
├── default
└── analytics
```

both app-a databases are included; app-b databases are not.

Restore targets exactly the registered databases for that application. It does not drop sibling databases, alter sibling roles, rotate sibling credentials or restore the provider as a whole. Ownership is re-verified before/after mutation, and sibling applications must remain usable.

Application destroy follows the same boundary: terminate connections only to the owned database, verify registered ownership, drop that database and role, remove its credential reference and registration, then preserve the shared provider while any sibling application remains. Only the last consumer may remove the shared provider runtime and volume.

