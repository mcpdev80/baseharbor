# Back up and restore an application

Create one encrypted recovery unit for an application's owned data. Run these commands inside a deployed application repository, such as `orders-api` with SQL data.

## Interactive backup

```bash
baha app backup --output ../orders-api.baha-backup
```

The terminal flow asks for a hidden password and shows the recovery scope. Preserve that password separately from the archive. An archive without a verified backup result is not proof of recoverability.

## Backup from automation

Prepare an owner-only password file outside Git through your secret manager and keep the same password available for recovery:

```bash
baha --no-input app backup   --password-file "$HOME/.config/orders/backup-password"   --output ../orders-api.baha-backup
```

Copy the encrypted archive and required recovery material to your protected off-host backup storage. The application name alone is not enough to restore an archive whose password has been lost.

## Restore deliberately

Restore replaces the selected application's owned recovery state. Check the destination Target and application before proceeding:

```bash
baha target show
baha app show
baha app restore ../orders-api.baha-backup   --password-file "$HOME/.config/orders/backup-password"
baha status
baha doctor
```

BaseHarbor validates/decrypts the archive before mutation, restores owned state, rebinds and verifies the recovered capabilities. Finally check a known business record, such as order `42`, through your application's own API or database query.

## Shared PostgreSQL boundary

For `orders-api` with SQL instances `default` and `analytics`, backup includes both registered application databases. It excludes a sibling application's databases and provider-global state; it never uses `pg_dumpall`.

Restore targets those registered databases without dropping sibling databases, changing sibling roles or rotating sibling credentials. Ownership is checked before and after mutation. Shared provider infrastructure stays available while sibling applications consume it.

See [Backup and restore reference](../reference/backup-and-restore.md) for exact recovery behavior and supported state classes.
