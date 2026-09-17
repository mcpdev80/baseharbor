# Application backup and restore

BaseHarbor treats backup as one encrypted application recovery unit rather than a loose set of unrelated dumps. Backup support is considered complete only together with exercised restore and post-restore verification.

## Create a backup

From a repository containing `baseharbor.yaml`, an interactive terminal can use the guided flow:

```bash
baha app backup
```

Before mutation, BaseHarbor shows the application/environment, target archive, durable PostgreSQL resources, whether managed secrets are included, and the temporary snapshot impact. Password entry disables terminal echo, requires confirmation, and never places the password in argv. Interactive short-password or confirmation errors are retried instead of immediately aborting.

Automation keeps the deterministic password-file path:

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

After a successful backup, BaseHarbor records non-secret metadata such as archive path, creation time, PostgreSQL logical resources and managed-secret inclusion under protected application state. `baha app show` can display that metadata without revealing secret names or values.

## Restore

Interactive restore:

```bash
baha app restore ./mailflow-production.bhbackup
```

The archive is decrypted and validated before destructive mutation. BaseHarbor then shows the application identity/environment, archive creation time, included managed resources and restore impact. Interactive restore defaults to **No** and proceeds only after explicit confirmation.

Automation remains available with:

```bash
baha app restore ./mailflow-production.bhbackup \
  --password-file ./backup-password.txt
```

Restore validates PostgreSQL/OpenBao payloads and prerequisites, rebuilds protected runtime state, restores data while the workload is stopped, regenerates runtime identity material, and restarts/verifies the application boundary.

Repository workloads receive a bounded readiness window after restore so real applications can reach service health and HTTP/TLS exposure readiness. This does not weaken fail-closed semantics: success is not reported merely because containers started, and the operation still fails if the verified boundary does not become READY within the owned timeout.

A successful restore records protected non-secret recovery metadata and prints `Status: READY` only after backend state, managed secrets, regenerated runtime identity, repository workload and applicable HTTP/TLS exposure checks have passed.

Malformed archives, wrong passwords, identity mismatches, failed preflight or failed final verification do not become successful restores.

## Progress output

Guided interactive backup and restore render visible activity immediately while the underlying hardened command runs. The indicator is intentionally indeterminate rather than inventing percentage estimates. Buffered command output remains available so failures stay observable and actionable.

Non-interactive `--password-file` automation retains deterministic command behavior and does not depend on interactive terminal rendering.

## Scope

The current recovery unit covers BaseHarbor-owned application backend state: desired application metadata, managed PostgreSQL instances and the application-owned OpenBao scope when enabled. Application-owned files, external databases, object storage or other data outside the BaseHarbor-managed recovery unit require their own backup/recovery mechanism.

Backup archive format, cryptography and restore semantics are part of the release compatibility contract.
