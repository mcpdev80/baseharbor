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
