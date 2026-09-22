# Getting started

This tutorial gets an existing application running under BaseHarbor without requiring you to understand provider internals first.

## 1. Inspect the repository

```bash
baha app inspect .
```

Inspection is read-only. BaseHarbor reports what it detected and does not adopt ambiguous infrastructure silently.

## 2. Create the portable application contract

```bash
baha app init
```

For a repository with unambiguous evidence:

```bash
baha app init --quick
```

## 3. Review the plan

```bash
baha plan
```

Planning is read-only.

## 4. Start the application

```bash
baha up -e dev
```

Compose is the current complete runtime. Kubernetes and OpenShift are later runtime providers.

## 5. Verify

```bash
baha status
baha doctor
```

A successful BaseHarbor operation means the relevant capability was verified, not only that a container started.

## Next steps

- [Application contract](../explanation/application-contract.md)
- [Environments](../how-to/environments.md)
- [PostgreSQL](../how-to/postgres.md)
- [Secrets](../how-to/secrets.md)
- [Backup and restore](../how-to/backup-restore.md)
