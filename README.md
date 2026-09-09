# BaseHarbor

[![CI](https://github.com/mcpdev80/baseharbor/actions/workflows/ci.yml/badge.svg)](https://github.com/mcpdev80/baseharbor/actions/workflows/ci.yml)
[![GitHub Release](https://img.shields.io/github/v/release/mcpdev80/baseharbor?display_name=tag&sort=semver)](https://github.com/mcpdev80/baseharbor/releases)
[![License](https://img.shields.io/github/license/mcpdev80/baseharbor)](LICENSE)

Secure, modular, self-hosted backend infrastructure for independent applications, operated through the `baha` CLI.

BaseHarbor provides common backend capabilities such as PostgreSQL, Valkey, managed secrets, runtime identity, backup/restore and lifecycle management without forcing applications into a proprietary SDK or monolith.

> BaseHarbor should hide operational complexity without hiding standard interfaces.

Applications keep using normal protocols, environment variables and files. A repository can declare the backend it needs in `baseharbor.yaml`; developers do not need a BaseHarbor login and workloads do not need the `baha` process at runtime.

## Status

BaseHarbor is **pre-v1 and already consumed by a real application**. The project is preparing its first official `v0.1.0` release.

The current default branch includes:

- single-node BaseHarbor control plane with PostgreSQL and OpenBao;
- guided first-run host-port selection for the control plane;
- user-global control-plane runtime state that survives application checkout changes;
- repository-owned `baseharbor.yaml` application contracts;
- repository discovery so most `baha app` commands do not require repeating the application name;
- one or multiple named PostgreSQL instances per application;
- one or multiple named Valkey/Redis-protocol instances per application;
- application environment/file bindings using standard connection information;
- managed required secrets with fail-closed workload startup gates;
- static secret environment/file delivery;
- app-scoped dynamic secret references and runtime API;
- per-application runtime identity and mTLS broker isolation;
- lifecycle status/doctor/down/up/destroy operations;
- encrypted application backup/restore covering metadata, PostgreSQL and the application OpenBao scope.

The public compatibility contract is still allowed to evolve during `0.x`. Patch releases are expected to remain compatible; minor releases may contain documented breaking changes until `v1.0.0`.

## Install `baha`

Released Linux binaries are the normal installation path. Releases are published for amd64 and arm64 together with SHA-256 checksums and GitHub build-provenance attestations.

Install the latest stable release:

```bash
curl -fsSL https://raw.githubusercontent.com/mcpdev80/baseharbor/main/scripts/install.sh | bash
```

For production automation, pin both installer and requested version to the immutable release tag:

```bash
curl -fsSL https://raw.githubusercontent.com/mcpdev80/baseharbor/v0.1.0/scripts/install.sh \
  | bash -s -- v0.1.0
```

The installer downloads the matching archive over HTTPS, verifies it against the published SHA-256 manifest, installs `baha` to `~/.local/bin/baha` by default and prints the installed build metadata.

```bash
baha version
```

Building from source is a development/contributor path, not the production install contract:

```bash
go build -o baha ./cmd/baha
```

## Preferred application workflow

The preferred contract lives with the application source:

```yaml
version: 1

app:
  name: mailflow
  environment: production

services:
  postgres:
    enabled: true
  redis:
    enabled: true
  secrets:
    enabled: true

secrets:
  required:
    - name: SECRET_KEY
```

Create a repository manifest non-interactively:

```bash
baha app init mailflow \
  --environment production \
  --postgres \
  --redis \
  --require-secret SECRET_KEY
```

Then operate from the repository without repeating the application name:

```bash
baha app plan
baha app preflight
baha app apply
baha app status
baha app doctor
```

The planned guided checkbox-style `baha app init` flow will generate the same manifest contract; the current flags are the deterministic automation path.

## Control-plane bootstrap

On first initialization:

```bash
baha up
```

`baha` checks the default loopback ports before writing runtime state. Interactive terminals can accept or change the proposed ports. Automation can accept safe proposals non-interactively:

```bash
baha up --yes
```

Explicit ports are also supported and still fail closed when occupied:

```bash
baha up --postgres-port 15432 --openbao-port 18200
```

Control-plane state is user-global by default:

```text
$XDG_DATA_HOME/baseharbor/runtime
```

or, when `XDG_DATA_HOME` is unset:

```text
~/.local/share/baseharbor/runtime
```

`BASEHARBOR_STATE_DIR` remains an explicit operator/CI override, and a legacy `.baseharbor/runtime` is reused only when no global state exists yet.

Inspect the runtime:

```bash
baha status
baha doctor
```

Bootstrap the bundled OpenBao trust plane with an explicit recovery destination:

```bash
baha openbao bootstrap --recovery-file /secure/off-host/openbao-recovery.json
baha openbao status
```

After a restart, unseal the current manual Shamir profile explicitly:

```bash
baha openbao unseal --recovery-file /secure/off-host/openbao-recovery.json
```

The initial root token is not persisted and is revoked after the restricted BaseHarbor manager identity has been established and verified.

## Multiple service instances

One default instance stays simple:

```yaml
services:
  postgres:
    enabled: true
  redis:
    enabled: true
```

Applications that need several independent logical services use names:

```yaml
services:
  postgres:
    instances:
      primary: {}
      analytics: {}
  redis:
    instances:
      cache: {}
      sessions: {}
```

Each named instance gets independent credentials, persistent data and stable bindings. Multiple logical instances are not HA; HA is a topology behind one stable logical service and is tracked separately as an architecture decision.

## Native application consumption

BaseHarbor materializes owner-only standard application connection information and file bindings. Examples include:

```text
DATABASE_URL=postgresql://...
REDIS_URL=redis://...
VALKEY_URL=redis://...
```

For named instances, stable variables such as `DATABASE_PRIMARY_URL` and `REDIS_SESSIONS_URL` are generated.

Inspect the contract without making BaseHarbor a runtime dependency:

```bash
baha app env
baha app env --format json
baha app env --path
```

Credential-bearing values are masked by default; revealing them requires an explicit option.

## Managed secrets

Applications declare secret **names**, never values:

```yaml
secrets:
  required:
    - name: OPENAI_API_KEY
    - name: SMTP_PASSWORD
```

Set values without exposing them on the command line:

```bash
printf '%s' "$OPENAI_API_KEY" | baha app secret set OPENAI_API_KEY --stdin
printf '%s' "$SMTP_PASSWORD" | baha app secret set SMTP_PASSWORD --stdin
```

`baha app apply` and `baha app up` fail closed before workload start when required secrets are missing or unusable. Status and doctor expose readiness metadata only, never secret values.

BaseHarbor also supports app-scoped dynamic secret references through the runtime broker so applications can store an opaque reference while the credential remains in OpenBao.

## Backup and restore

Create one encrypted recovery unit:

```bash
baha app backup --password-file ./backup-password.txt
```

Optionally choose the output path:

```bash
baha app backup \
  --password-file ./backup-password.txt \
  --output ./mailflow-production.bhbackup
```

Restore and verify it:

```bash
baha app restore ./mailflow-production.bhbackup \
  --password-file ./backup-password.txt
```

The recovery unit includes desired application metadata, every managed PostgreSQL instance and the application-owned OpenBao secret scope. Restore validates and decrypts before mutation, rebuilds protected state, restores data while the workload is stopped, regenerates runtime identities and verifies the restarted application boundary.

## CLI discovery

```bash
baha --help
baha app --help
baha openbao --help
baha version
```

The detailed current command tree is documented in [`docs/cli.md`](docs/cli.md).

## Documentation

The repository Markdown under [`docs/`](docs/index.md) is the canonical documentation source and feeds the bilingual GitHub Pages site.

- English (default): <https://mcpdev80.github.io/baseharbor/>
- Deutsch: <https://mcpdev80.github.io/baseharbor/de/>

Start with:

- [documentation index](docs/index.md)
- [application contract](docs/application-contract.md)
- [repository workflow](docs/repository-application-workflow.md)
- [`baha` CLI](docs/cli.md)
- [control-plane runtime](docs/runtime-compose.md)
- [secrets and OpenBao](docs/secrets-and-openbao.md)
- [backup and restore](docs/backup-and-restore.md)
- [release policy](docs/releases.md)
- [architecture](docs/architecture.md)
- [roadmap](docs/roadmap.md)
- [development guidelines](docs/DEVELOPMENT_GUIDELINES.md)

## Design goals

- one dependable CLI for setup and lifecycle management;
- secure defaults, least privilege and fail-closed behavior;
- isolated backend service stacks for independent applications;
- native protocols and standard interfaces for application consumption;
- applications remain runnable without BaseHarbor when equivalent interfaces are supplied elsewhere;
- self-hosted first, cloud-native where useful;
- Docker/Podman first; Kubernetes optional;
- mature open-source components instead of unnecessary reinvention;
- observable health, backup/restore, certificates and lifecycle operations;
- optional first-class AI, MCP and RAG capabilities where they add value.

## Release policy

`main` is development state. Real products should consume published releases. During `0.x`, patch releases remain compatible within a minor line; minor releases may contain explicitly documented breaking changes. See [`docs/releases.md`](docs/releases.md).

## License

Apache License 2.0. See [LICENSE](LICENSE).
