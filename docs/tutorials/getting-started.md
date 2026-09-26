# Getting started

This tutorial gets an existing application running under BaseHarbor without requiring provider knowledge or manual manifest editing.

## Canonical developer path

From a normal existing repository:

```text
optional inspection
      |
      v
baha app init
      |
      v
human-readable adoption summary
      |
      v
baha up
      |
      v
READY
```

The normal happy path does **not** require manual YAML edits, Compose rewrites, explicit OpenBao bootstrap commands, a separate preflight/apply sequence, or shell-piped secret commands.

## 1. Optional: inspect the repository

```bash
baha app inspect .
```

Inspection is read-only. BaseHarbor reports detected workload, replaceable infrastructure and capability evidence without mutating the repository.

Use detailed evidence when needed:

```bash
baha app inspect . --verbose
```

## 2. Create the portable application contract

```bash
baha app init
```

BaseHarbor detects what it can and asks only for ambiguous or user-owned decisions.

The guided flow may ask you to:

- choose the application Compose file when multiple candidates exist;
- confirm application workload versus replaceable infrastructure;
- confirm provider-neutral SQL, cache, object-storage and observability intent;
- name application-owned secrets and mark them required or optional;
- choose whether an application secret is generated, entered during first apply, or configured later;
- confirm Runtime API permissions derived from concrete source evidence.

Before writing `baseharbor.yaml`, BaseHarbor shows a human-readable adoption summary. Raw YAML is secondary detail available with `--verbose`.

For deterministic automation with unambiguous evidence:

```bash
baha app init --quick
```

`--quick` fails closed on ambiguity and never silently promotes heuristic secret candidates.

## 3. Start the application

```bash
baha up
```

On the first run BaseHarbor may ask for information it cannot safely invent, for example:

- whether to accept or override the proposed Target-scoped location for the operator-held OpenBao recovery file;
- a missing required application-secret value;
- confirmation of a safe port fallback.

Interactive secret input disables terminal echo. Provider/runtime credentials are managed by BaseHarbor and are not requested from the developer.

The same `baha up` operation continues after these decisions and converges managed infrastructure, workload bindings and readiness. After successful OpenBao bootstrap, BaseHarbor persists only the recovery-file path reference on the effective Target. Later `baha up` runs automatically reuse that reference to unseal the shared OpenBao provider when the file is available.

## 4. Verify

```bash
baha status
```

```bash
baha doctor
```

A successful BaseHarbor operation means the relevant capability was verified, not merely that a container started.

## Advanced and automation commands

These commands remain available, but are not required knowledge for the basic happy path:

```bash
baha plan
baha app preflight
baha app apply
baha app secret set APP_SECRET
```

Automation can use explicit non-interactive secret input:

```bash
printf '%s' "$APP_SECRET" | baha app secret set APP_SECRET --stdin
```

## Reference end-to-end demo

The external `mcpdev80/baseharbor-demo` repository is the release-facing proof of this journey. Its README documents a complete pristine-repository test from `baha app init` through `baha up`, READY verification, restart and cleanup.

Pre-release validation executes both:

- the guided human adoption scenario;
- deterministic CI/component scenarios.

The final release reuses that immutable pre-release evidence instead of rerunning the same expensive matrix.

## Next steps

- [Application contract](../explanation/application-contract.md)
- [Environments](../how-to/environments.md)
- [PostgreSQL](../how-to/postgres.md)
- [Secrets](../how-to/secrets.md)
- [Backup and restore](../how-to/backup-restore.md)
- [CLI reference](../reference/cli.md)
