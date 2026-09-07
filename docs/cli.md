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
│   └── preflight
└── version
```

Use help at every level:

```bash
baha --help
baha app --help
baha app create --help
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

Explicit selection:

```bash
baha app create mailflow \
  --environment prod \
  --postgres \
  --redis \
  --secrets
```

Application state is stored under:

```text
.baseharbor/apps/<name>/baseharbor.yaml
```

The directory is owner-only and manifests are written with owner-only permissions. Manifests contain desired configuration, never plaintext service credentials.

Inspect it:

```bash
baha app list
baha app show mailflow
```

## Plan and preflight

Before BaseHarbor gains app-level mutation, the command model already separates desired state and prerequisite checks:

```bash
baha app plan mailflow
```

prints the resources BaseHarbor intends to converge and makes no changes.

```bash
baha app preflight mailflow
```

checks:

- manifest validity
- local application-state permissions
- Docker/Podman + Compose availability
- desired-state plan construction

and makes no changes.

This becomes the stable lifecycle pattern for future commands:

```text
plan → preflight → apply → verify
```

## Planned command evolution

The next application-runtime milestones add commands without changing the command hierarchy:

```text
baha app apply NAME
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
