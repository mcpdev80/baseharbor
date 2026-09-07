# `baha` CLI

`baha` is the primary operator interface for BaseHarbor. The goal is a single dependable binary that can initialize, inspect, provision, verify, repair and operate BaseHarbor without requiring developers to understand the underlying container or service wiring.

## Design rules

- One binary with no required language runtime.
- Hierarchical, discoverable commands.
- Every command has useful `--help` output.
- Read-only inspection is distinct from mutation.
- Usage errors and operational failures have different exit codes.
- Secrets are not printed unless a future command explicitly requires and documents reveal behavior.
- Commands must report actual readiness, not just process/container state.
- Preflight comes before mutation; verification comes after mutation.

## Current command tree

```text
baha
├── init
├── up
├── down
├── status
├── doctor
├── app
│   ├── create
│   ├── list
│   ├── show
│   ├── plan
│   ├── preflight
│   └── apply
└── version
```

Use help at every level:

```bash
baha --help
baha app --help
baha app create --help
baha app apply --help
baha help app create
```

## Exit codes

| Code | Meaning |
| --- | --- |
| `0` | command completed successfully |
| `1` | operational/runtime failure |
| `2` | invalid command or arguments |

This distinction is part of the CLI contract so scripts and automation can react correctly.

## Application manifests

Create a first application definition:

```bash
baha app create demo
```

With no explicit service selection, PostgreSQL is enabled as the minimal useful default.

Application state is stored under:

```text
.baseharbor/apps/<name>/baseharbor.yaml
```

The directory is owner-only and manifests are written with owner-only permissions. Manifests contain desired configuration, never plaintext service credentials.

## Plan, preflight, apply and verify

```bash
baha app plan demo
baha app preflight demo
baha app apply demo
```

`plan` and `preflight` are read-only. `apply` repeats the required validation, materializes the application runtime, runs Compose convergence, and returns success only after verification succeeds.

The stable lifecycle contract is:

```text
plan → preflight → apply → verify
```

The current convergence milestone supports PostgreSQL-only desired state. It creates a unique Compose project per application/environment, giving the application its own default network and PostgreSQL volume. PostgreSQL has no published host port by default. Generated runtime credentials live in an owner-only runtime environment file and are preserved on repeated apply operations.

Verification executes an authenticated PostgreSQL query and requires `SELECT 1` to succeed. A running container alone is not considered ready.

If Redis/Valkey or managed secrets are enabled in the manifest, `app apply` currently fails closed instead of silently ignoring unsupported desired state.

## Planned command evolution

```text
baha app up NAME
baha app down NAME
baha app status NAME
baha app doctor NAME
baha app env NAME
baha app backup NAME
baha app restore NAME
baha app upgrade NAME
baha app destroy NAME
```

Future certificate and platform lifecycle command families follow the same rules:

```text
baha cert ...
baha pki ...
baha backup ...
baha restore ...
baha upgrade ...
```

The CLI should stay conservative: adding a command is preferable to making one command silently perform unrelated actions.
