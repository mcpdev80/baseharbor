# `baha` CLI

`baha` is the primary operator interface for BaseHarbor. The goal is a single dependable binary that can initialize, inspect, provision, verify, repair and operate BaseHarbor without requiring developers to understand the underlying container or service wiring.

## Design rules

- One binary with no required language runtime.
- Hierarchical, discoverable commands.
- Every command has useful `--help` output.
- Read-only inspection is distinct from mutation.
- Usage errors and operational failures have different exit codes.
- Secret values are never accepted as normal command-line arguments or printed by current commands.
- Commands report actual protocol readiness, not just process/container state.
- Preflight comes before mutation; verification comes after mutation.
- Destructive commands verify exact resource ownership and fail closed on ambiguity.
- Security bootstrap material is supplied through explicit operator workflows, not hidden defaults.

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
│   ├── destroy
│   └── secret
│       ├── set
│       ├── list
│       └── delete
├── openbao
│   ├── status
│   ├── bootstrap
│   └── unseal
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
baha app create secure-app --postgres --secrets
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

### Managed secrets

Managed secrets use the BaseHarbor OpenBao trust plane and currently require PostgreSQL and/or Valkey so the application has a materialized runtime.

For `NAME` in environment `ENV`, `app apply` provisions:

```text
KV namespace:  baseharbor/apps/NAME/ENV/
Policy:        baseharbor-app-NAME-ENV
AppRole:       baseharbor-app-NAME-ENV
Credentials:   .baseharbor/apps/NAME/runtime/openbao.env
```

The application credential file is owner-only and contains only RoleID/SecretID. Application secret payloads are not written to local runtime files.

Each operator-managed key is stored as its own KV v2 document below the exact application/environment namespace. The application policy can wildcard only below that namespace and cannot cross into another application or environment.

The current milestone establishes server-side scope isolation, application identity and safe operator secret management. It does not yet inject OpenBao credentials or resolved values into workload containers or expose direct workload connectivity to the loopback-only bundled OpenBao listener.

## Plan, preflight, apply and verify

```bash
baha app plan demo
baha app preflight demo
baha app apply demo
```

`plan` and `preflight` are read-only. `apply` validates desired state, materializes the runtime, converges Compose and optional OpenBao application identity, and returns success only after every enabled service passes verification.

The stable lifecycle contract is:

```text
plan -> preflight -> apply -> verify
```

Each application/environment uses its own Compose project, private default network, service containers and persistent volumes. Generated PostgreSQL/Valkey runtime credentials live in an owner-only `runtime.env` and are preserved across repeated apply operations. Managed OpenBao bootstrap credentials live separately in owner-only `openbao.env`.

## Application secret values

Create or replace one value from stdin:

```bash
printf '%s' 'secret-value' | baha app secret set demo API_TOKEN --stdin
```

`secret set` never accepts the value as a positional argument or option value. Input is limited to 1 MiB, must be non-empty UTF-8 text and is stored without being printed. BaseHarbor reads the stored value internally after the write and compares it byte-for-byte before reporting success.

List key names only:

```bash
baha app secret list demo
```

The output contains only the configured key names. There is intentionally no current CLI command that reveals secret values.

Preview deletion:

```bash
baha app secret delete demo API_TOKEN
```

Confirm permanent deletion:

```bash
baha app secret delete demo API_TOKEN --yes
```

Without `--yes`, deletion is read-only. Confirmed deletion removes the selected KV v2 document's metadata and all historical versions, then verifies that the key is no longer present.

Secret key names accept ASCII letters, digits, `_`, `-` and `.`, are limited to 128 characters and may not start with `-` or `.`. BaseHarbor-reserved names are rejected.

An application scope created before per-key secret namespaces were introduced must first be reconciled with:

```bash
baha app apply demo
```

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
- runtime file permissions, including `openbao.env` when present
- managed runtime definition integrity
- Docker/Podman + Compose availability
- Compose configuration validity
- service running state
- PostgreSQL authenticated query readiness when enabled
- Valkey authenticated PING readiness when enabled
- OpenBao application AppRole authentication and managed-policy ownership when managed secrets are enabled

Neither command writes application secrets or prints credentials.

## Stop and resume without deleting data

```bash
baha app down demo
baha app up demo
```

`app down` performs ownership and runtime-definition preflight first. It removes service containers and the transient network while preserving every managed persistent volume, the application manifest, runtime definition, credentials and managed OpenBao scope.

`app up` is deliberately different from `app apply`. It never materializes fresh runtime state. Every expected persistent volume must already exist; if one is missing, `app up` fails closed rather than silently creating an empty replacement. Existing managed OpenBao identity is inspected before start and verified again afterward.

## Permanent destruction

Preview:

```bash
baha app destroy demo
```

Permanent deletion:

```bash
baha app destroy demo --yes
```

Before deletion BaseHarbor verifies the manifest, local file permissions, generated runtime definition, Compose configuration, exact expected resource names, `com.docker.compose.project` ownership labels and, when enabled, the exact OpenBao AppRole/policy ownership definition. Ambiguous or modified ownership fails closed.

With `--yes`, owned service containers, network and all managed persistent volumes are removed. Every managed OpenBao secret document, the namespace marker, probe metadata, AppRole and policy are then removed before local application state is deleted.

## OpenBao trust-plane lifecycle

The bundled OpenBao service is intentionally not initialized with a static development token. Start the control-plane runtime first:

```bash
baha up
```

A fresh OpenBao instance reports not initialized:

```bash
baha openbao status
```

Bootstrap requires an explicit recovery destination:

```bash
baha openbao bootstrap --recovery-file /secure/off-host/openbao-recovery.json
```

Bootstrap:

- refuses an already initialized OpenBao instance
- refuses a recovery path inside `.baseharbor`
- refuses to overwrite an existing recovery file
- creates the recovery file owner-only
- initializes the current single-node profile with one Shamir key share / threshold one
- does not print the unseal key
- does not intentionally persist or print the initial root token
- enables `baseharbor/` as KV v2
- enables AppRole authentication
- creates a restricted BaseHarbor manager policy and AppRole
- permits that manager to provision only BaseHarbor-named application policies/AppRoles
- stores only RoleID/SecretID in owner-only control-plane state
- verifies manager authentication and KV access
- revokes the initial root token

Inspect the resulting trust plane with:

```bash
baha openbao status
```

After a restart, the manual Shamir profile is sealed. Unseal it explicitly:

```bash
baha openbao unseal --recovery-file /secure/off-host/openbao-recovery.json
```

The unseal key is passed through stdin to the local container-runtime boundary and is not placed in the host command argument list.

Current manager bootstrap credentials are stored at:

```text
.baseharbor/runtime/openbao-admin.env
```

This owner-only file is BaseHarbor bootstrap state, not application configuration. It must never be exposed through normal CLI output or committed.

A trust plane bootstrapped before the manager gained application-identity provisioning permissions must be explicitly rebuilt/re-bootstrapped or operator-reconciled during the current pre-release phase. BaseHarbor fails closed rather than silently attempting privilege escalation.

See [secrets-and-openbao.md](secrets-and-openbao.md) for the trust and recovery model.

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
