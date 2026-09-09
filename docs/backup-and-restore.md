# Application backup and restore

BaseHarbor treats backup as one encrypted application recovery unit rather than a loose set of unrelated dumps.

## Create a backup

From a repository containing `baseharbor.yaml`:

```bash
baha app backup --password-file ./backup-password.txt
```

Choose an explicit output path when needed:

```bash
baha app backup \
  --password-file ./backup-password.txt \
  --output ./mailflow-production.bhbackup
```

The command can also target legacy stored application state by passing `NAME` explicitly.

Before capture, BaseHarbor verifies the managed runtime and, when enabled, the application OpenBao scope. It quiesces the repository workload and application secret broker, captures state, writes the encrypted archive and then restarts the components it stopped.

The encrypted recovery unit contains:

- desired application metadata;
- every managed PostgreSQL instance;
- the application-owned OpenBao secret scope when managed secrets are enabled.

The backup password is read from a file, not accepted as a normal command-line value.

## Restore

```bash
baha app restore ./mailflow-production.bhbackup \
  --password-file ./backup-password.txt
```

An optional application `NAME` may be supplied for compatibility, but it must match the application identity stored in the archive.

Restore is deliberately fail-closed:

1. read and validate the archive;
2. decrypt and validate application metadata before mutation;
3. validate all PostgreSQL/OpenBao payloads;
4. verify BaseHarbor/OpenBao prerequisites;
5. reset only the owned restore target;
6. rebuild protected BaseHarbor runtime state;
7. restore PostgreSQL while the workload is stopped;
8. restore the matching OpenBao secret scope;
9. regenerate runtime identity material;
10. restart and verify the application broker/workload boundary.

A malformed archive, wrong password, mismatched application identity or failed preflight stops before destructive restore work proceeds.

## Scope and limitations

The current recovery unit covers BaseHarbor-owned application backend state. It is not a generic backup of arbitrary files owned by the application.

Application-owned data outside BaseHarbor-managed PostgreSQL/OpenBao must be backed up by the application or by another declared BaseHarbor provider when such providers are added.

Backup/restore must remain compatible with the release contract. Changes to archive format, cryptography or restore semantics require explicit release notes and compatibility handling.