# `baha` CLI

`baha` is the primary operator interface for BaseHarbor. The goal is a single dependable binary that can initialize, inspect, provision, verify, repair and operate BaseHarbor without requiring developers to understand the underlying container or service wiring.

## Design rules

- One binary with no required language runtime.
- Hierarchical, discoverable commands.
- Every command has useful `--help` output.
- Read-only inspection is distinct from mutation.
- Usage errors and operational failures have different exit codes.
- Secrets are not printed unless a future command explicitly requires and documents reveal behavior.
- Commands report actual protocol readiness, not just process/container state.
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

## Exit codes

| Code | Meaning |
| --- | --- |
| `0` | command completed successfully |
| `1` | operational/runtime failure |
| `2` | invalid command or arguments |

## Application manifests

Examples:

```bash
baha app create postgres-app --postgres
baha app create cache-app --redis
baha app create full-app --postgres --redis
```

With no service flag, PostgreSQL remains the minimal default. The manifest field is named `redis` for protocol/API compatibility; BaseHarbor provisions Valkey as the managed implementation.

Application state is stored under:

```text
.baseharbor/apps/<name>/baseharbor.yaml
```

Manifests contain desired configuration, never plaintext service credentials.

## Supported application services

### PostgreSQL

- `postgres:18-alpine`
- dedicated application volume
- no host port by default
- generated application password
- readiness: authenticated `SELECT 1`

### Valkey

- `valkey/valkey:9.1.2-alpine`
- Redis-compatible protocol
- dedicated application volume mounted at `/data`
- AOF persistence enabled
- generated application password
- no host port by default
- readiness: authenticated `PING` must return `PONG`

Managed secrets are still unsupported by app convergence and fail closed when requested.

## Plan, preflight, apply and verify

```bash
baha app plan demo
baha app preflight demo
baha app apply demo
```

`plan` and `preflight` are read-only. `apply` validates desired state, materializes the runtime, converges Compose, and returns success only after every enabled service passes verification.

The stable lifecycle contract is:

```text
plan -> preflight -> apply -> verify
```

Each application/environment uses its own Compose project, private default network, service containers and persistent volumes. Generated runtime credentials live in an owner-only `runtime.env` and are preserved across repeated apply operations.

## Status and doctor

```bash
baha app status demo
baha app doctor demo
```

`app status` is compact and automation-friendly. It checks running state plus protocol readiness for every enabled service.

`app doctor` reports each boundary independently, including:

- manifest validity
- supported desired services
- manifest permissions
- materialized runtime state
- runtime file permissions
- managed runtime definition integrity
- Docker/Podman + Compose availability
- Compose configuration validity
- service running state
- PostgreSQL authenticated query readiness when enabled
- Valkey authenticated PING readiness when enabled

Neither command mutates application state or prints credentials.

## Stop and resume without deleting data

```bash
baha app down demo
baha app up demo
```

`app down` performs ownership and runtime-definition preflight first. It removes service containers and the transient network while preserving every managed persistent volume, the application manifest, runtime definition and credentials. Post-verification checks that existing persistent volumes were retained.

`app up` is deliberately different from `app apply`. It never materializes fresh runtime state. Every expected persistent volume must already exist; if one is missing, `app up` fails closed rather than silently creating an empty replacement. After start, all enabled services must pass authenticated protocol verification.

## Permanent destruction

Preview:

```bash
baha app destroy demo
```

Permanent deletion:

```bash
baha app destroy demo --yes
```

Before deletion BaseHarbor verifies the manifest, local file permissions, generated runtime definition, Compose configuration, exact expected resource names, and `com.docker.compose.project` ownership labels. Ambiguous ownership fails closed.

With `--yes`, owned service containers, network and all managed persistent volumes are removed. BaseHarbor verifies runtime-resource absence before deleting local application state, then verifies state deletion as well.

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
