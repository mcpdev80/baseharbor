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
- Destructive commands verify exact resource ownership and fail closed on ambiguity.

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
│   ├── apply
│   ├── status
│   ├── doctor
│   ├── down
│   ├── up
│   └── destroy
└── version
```

Use help at every level:

```bash
baha --help
baha app --help
baha app create --help
baha app apply --help
baha app status --help
baha app doctor --help
baha app down --help
baha app up --help
baha app destroy --help
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

## Status and doctor

After apply, inspect the operational state without mutating it:

```bash
baha app status demo
baha app doctor demo
```

`app status` is compact and automation-friendly. It confirms that the PostgreSQL service is actually running and that an authenticated `SELECT 1` succeeds. It exits non-zero if the application is not ready.

`app doctor` performs deeper diagnostics and reports each boundary separately:

- manifest validity
- supported desired services
- manifest permissions
- materialized runtime state
- runtime file permissions
- Docker/Podman + Compose availability
- Compose configuration validity
- PostgreSQL service running state
- authenticated PostgreSQL readiness

Neither command changes application state or prints runtime credentials.

## Stop and resume without deleting data

Stop an application while preserving persistent state:

```bash
baha app down demo
```

`app down` performs a read-only safety preflight first. It verifies the manifest and runtime permissions, verifies that the Compose file still exactly matches the BaseHarbor-managed definition, validates the Compose configuration and checks ownership labels for the expected project resources.

After the preflight succeeds, it removes the application container and transient Compose network while preserving:

- the dedicated PostgreSQL volume
- the application manifest
- the generated runtime environment and credentials
- the BaseHarbor runtime definition

Post-verification confirms that the container and network are gone and that an existing PostgreSQL volume was not removed.

Resume the existing materialized runtime with:

```bash
baha app up demo
```

`app up` is deliberately different from `app apply`. It does not materialize new runtime state and refuses to recreate a missing PostgreSQL volume. Before starting anything it requires the existing manifest, owner-only runtime files, unchanged BaseHarbor-managed Compose definition, valid Compose configuration, unambiguous resource ownership, and the already-existing managed PostgreSQL volume.

After start it waits for the authenticated PostgreSQL `SELECT 1` verification to succeed before reporting the application ready. Existing runtime credentials remain unchanged and the preserved PostgreSQL volume is reused.

## Permanent destruction

Preview first:

```bash
baha app destroy demo
```

This command is non-mutating without `--yes`. It runs the ownership and safety preflight and prints the exact currently present BaseHarbor-managed runtime resources plus the local application-state directory that would be deleted.

Permanent deletion requires explicit confirmation:

```bash
baha app destroy demo --yes
```

Before deletion BaseHarbor verifies:

- the manifest and local file permissions
- the generated runtime definition has not been modified
- Compose configuration validity
- exact expected container, network and volume names
- the `com.docker.compose.project` ownership label for every expected resource that exists

If an expected resource name exists but its ownership label does not match the application project, destruction fails closed and nothing is intentionally deleted by BaseHarbor.

After the preflight, `--yes` removes the owned Compose runtime including persistent volumes, then verifies that the managed runtime resources are absent before deleting the local application state. The final state deletion is also verified.

## Planned command evolution

```text
baha app env NAME
baha app backup NAME
baha app restore NAME
baha app upgrade NAME
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
