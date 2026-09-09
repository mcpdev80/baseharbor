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

Before capture, BaseHarbor verifies the managed runtime and, when enabled, the application OpenBao scope. It quiesces the repository workload and application secret broker, captures state, writes the encrypted archive and then restarts the components it stopped.

The encrypted recovery unit contains desired application metadata, every managed PostgreSQL instance and the application-owned OpenBao secret scope when managed secrets are enabled.

## Restore

```bash
baha app restore ./mailflow-production.bhbackup \
  --password-file ./backup-password.txt
```

Restore validates and decrypts before mutation, validates PostgreSQL/OpenBao payloads, verifies prerequisites, rebuilds protected runtime state, restores data while the workload is stopped, regenerates runtime identity material, and restarts/verifies the application boundary.

Malformed archives, wrong passwords, identity mismatches or failed preflight stop before destructive restore work proceeds.

## Scope

The current recovery unit covers BaseHarbor-owned application backend state. Application-owned files or data outside BaseHarbor-managed PostgreSQL/OpenBao require their own backup mechanism.

Backup archive format, cryptography and restore semantics are part of the release compatibility contract.