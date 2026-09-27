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

Before capture, BaseHarbor verifies the managed runtime and, when enabled, the application OpenBao scope. It quiesces the repository workload and Application Runtime Broker, captures state, writes the encrypted archive and then restarts the components it stopped.

The encrypted recovery unit is built from typed recovery contributors. Application metadata is always included. Managed SQL, the application-owned OpenBao secret scope, managed S3 objects and BaseHarbor-owned repository workload volumes are supported application-owned state. Application log history can be selected explicitly when log collection is declared and BaseHarbor owns the log-history path.

After a successful backup, BaseHarbor records non-secret metadata such as archive path, creation time and the typed recovery contributors with ownership, support, selection and verification state under protected application state. Secret values, access keys, tokens and private keys are never written into this metadata.

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

Restore validates every selected payload and the recovery manifest before mutation, rebuilds protected runtime state, restores selected SQL, secret, S3, workload-volume and log-history state while the workload is stopped, regenerates application/runtime identity material, and restarts/verifies the application boundary.

Repository workloads receive a bounded readiness window after restore so real applications can reach service health and HTTP/TLS exposure readiness. This does not weaken fail-closed semantics: success is not reported merely because containers started, and the operation still fails if the verified boundary does not become READY within the owned timeout.

A successful restore records protected non-secret recovery metadata and prints `Status: READY` only after backend state, managed secrets, regenerated runtime identity, repository workload and applicable HTTP/TLS exposure checks have passed.

Malformed archives, wrong passwords, identity mismatches, failed preflight or failed final verification do not become successful restores.

## Progress output

Guided interactive backup and restore render visible activity immediately while the underlying hardened command runs. The indicator is intentionally indeterminate rather than inventing percentage estimates. Buffered command output remains available so failures stay observable and actionable.

Non-interactive `--password-file` automation retains deterministic command behavior and does not depend on interactive terminal rendering.

## Recovery selection and scope

Automation can select typed state classes explicitly:

```bash
baha app backup \
  --include-state observability.logs \
  --exclude-state workload.storage \
  --password-file ./backup-password.txt
```

The guided backup flow uses the same typed state classes and presents supported application-owned recovery choices interactively. Application metadata remains mandatory. Runtime leaf identities and application trust edges are reconstructed from desired state during restore rather than copying private CA keys into an application archive.

Supported application-owned state in v0.4.16 includes:

- `database.sql` managed SQL data;
- `secrets` application-owned OpenBao secret scope;
- `object-storage.s3` managed S3 bucket contents;
- `workload.storage` BaseHarbor-owned repository workload named volumes;
- `observability.logs` application log history when selected and managed by BaseHarbor;
- `security.pki` application/runtime identity reconstruction.

Application log history is selectable operational history and is excluded by default unless explicitly selected. Metrics and trace history remain explicitly unsupported because BaseHarbor does not yet provide a safe application-scoped restore path for those histories.

External named volumes, bind mounts, external databases, external object stores and other operator-owned state remain outside BaseHarbor recovery ownership. They are represented explicitly as external/excluded contributors rather than copied silently.

A durable unsupported application-owned contributor blocks backup unless the operator explicitly excludes it. BaseHarbor never presents a partial recovery unit as complete without recording that boundary.

Backup archive format, cryptography, typed recovery manifest and restore semantics are part of the release compatibility contract.
