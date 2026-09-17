# BaseHarbor

[![CI](https://github.com/mcpdev80/baseharbor/actions/workflows/ci.yml/badge.svg)](https://github.com/mcpdev80/baseharbor/actions/workflows/ci.yml)
[![GitHub Release](https://img.shields.io/github/v/release/mcpdev80/baseharbor?display_name=tag&sort=semver)](https://github.com/mcpdev80/baseharbor/releases)
[![License](https://img.shields.io/github/license/mcpdev80/baseharbor)](LICENSE)

Secure, modular, self-hosted backend infrastructure for independent applications, operated through the `baha` CLI.

BaseHarbor provides common backend capabilities such as PostgreSQL, Valkey, managed secrets, runtime identity, backup/restore and lifecycle management without forcing applications into a proprietary SDK or monolith.

> BaseHarbor should hide operational complexity without hiding standard interfaces.

Applications keep using normal protocols, environment variables and files. A repository can declare the backend it needs in `baseharbor.yaml`; developers do not need a BaseHarbor login for the current trusted local/Compose workflow and workloads do not need the `baha` process at runtime.

## Status

BaseHarbor is **pre-v1 and already consumed by real reference applications**. The current stable release is `v0.4.0`. Compose remains the complete runtime implementation, while v0.4 establishes the portable application-contract, runtime-provider and declarative-input seams that future Kubernetes/OpenShift providers will consume.

The v0.4.0 line includes:

- single-node BaseHarbor control plane with PostgreSQL and OpenBao;
- guided first-run host-port selection for the control plane;
- user-global control-plane runtime state that survives application checkout changes;
- repository-owned Manifest v1 `baseharbor.yaml` application contracts;
- a provider-neutral `PortableContract` compatibility adapter for logical application intent;
- explicit separation between application requirements, runtime-provider selection and capability-provider/product selection;
- Compose as the current runtime provider behind an explicit provider/capability seam;
- protected deployment-owned runtime-provider state and fail-closed unsupported-provider behavior;
- declarative input resolution for default, generated, external and conditional deployment values;
- `baha app init --input NAME=VALUE` for automation-safe non-secret deployment input injection;
- repository-aware `baha up` that resolves only missing deployment inputs before application convergence;
- detect-first guided repository initialization and deterministic automation flags;
- one or multiple named PostgreSQL instances per application;
- one or multiple named Valkey/Redis-protocol instances per application;
- explicit workload-only Compose applications without artificial backend dependencies;
- application environment/file bindings using standard connection information;
- managed required/generated secrets with fail-closed workload startup gates;
- app-scoped dynamic secret references and per-application mTLS runtime broker isolation;
- service-level, health-aware workload status plus application-owned HTTP/HTTPS exposure readiness;
- trusted-local developer access through database/cache clients, logs, shell and exec;
- guided encrypted backup/restore with verified recovery metadata and fail-closed post-restore readiness;
- strict fast-forward Git-backed application updates with optional encrypted pre-update recovery points;
- guarded BaseHarbor self-update with checksum verification, atomic replacement and rollback;
- repository deployment initialization for public FQDN and TLS mode;
- existing/BYOC TLS certificate lifecycle with validation, downgrade protection, reload and readiness verification;
- automatic persisted fallback for configurable workload host-port conflicts, including IPv4/IPv6 Docker bind errors.

Kubernetes and OpenShift are not implemented in v0.4. They are future runtime providers that should map the same logical application requirements to their native primitives instead of requiring applications to adopt a second operational contract.

The public compatibility contract is still allowed to evolve during `0.x`. Patch releases are expected to remain compatible; minor releases may contain documented breaking changes until `v1.0.0`.

## Install `baha`

Released Linux binaries are the normal installation path. Releases are published for amd64 and arm64 together with SHA-256 checksums and GitHub build-provenance attestations.

Install the latest stable release:

```bash
curl -fsSL https://raw.githubusercontent.com/mcpdev80/baseharbor/main/scripts/install.sh | bash
```

For production automation, pin both installer and requested version to an immutable published release tag:

```bash
curl -fsSL https://raw.githubusercontent.com/mcpdev80/baseharbor/v0.4.0/scripts/install.sh \
  | bash -s -- v0.4.0
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

The preferred portable contract lives with the application source:

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

`app.name` is the stable logical application identity. In manifest v1, `app.environment` identifies deployment context; it is not an intrinsic business property of the application. The same logical application may later be realized independently in development, staging, production or customer-specific environments. See [`docs/decisions/0002-application-environment-is-deployment-context.md`](docs/decisions/0002-application-environment-is-deployment-context.md).

Manifest v1 remains the supported compatibility surface in v0.4. Internally, BaseHarbor translates portable intent into provider-neutral logical capabilities while keeping Compose/deployment implementation details outside that contract.

Interactive repository setup is detect-first:

```bash
baha app init
```

For deterministic application-contract creation:

```bash
baha app init mailflow \
  --environment production \
  --postgres \
  --redis \
  --require-secret SECRET_KEY
```

The guided flow detects the current project first and asks only for missing or ambiguous application-contract information. Deployment/runtime values use the reusable input resolver and remain protected deployment state rather than portable requirements in `baseharbor.yaml`.

Existing dedicated deployment flags remain supported, and non-interactive automation can inject declared non-secret inputs explicitly:

```bash
baha app init --yes \
  --input hostname=mail.example.com \
  --input tls_mode=existing \
  --input cert_dir=/secure/certificates
```

Then operate from the repository without repeating the application name:

```bash
baha app plan
baha app preflight
baha app apply
baha app show
baha app status
baha app doctor
```

## Control-plane bootstrap

On first initialization:

```bash
baha up
```

`baha` checks the default loopback ports before writing runtime state. Interactive terminals can accept or change the proposed ports. Automation can accept safe proposals non-interactively:

```bash
baha up --yes
```

Inside an application repository, `baha up` also resolves required deployment inputs before convergence. Complete protected state causes no additional questions; non-interactive mode uses only safe defaults/derivations and never invents an external certificate path.

Explicit control-plane ports are also supported and still fail closed when occupied:

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

Each named instance gets independent credentials, persistent data and stable bindings. Multiple logical instances are not HA; HA is a topology behind one stable logical service and is tracked separately as future architecture work.

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

Credential-bearing values are masked by default; revealing them requires an explicit operation.

Trusted local developer access is available through logical resources and services:

```bash
baha app psql
baha app valkey
baha app logs
baha app shell SERVICE
baha app exec SERVICE COMMAND
```

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

`baha app apply` and `baha app up` fail closed before workload start when required secrets are missing or unusable. Status, show and doctor expose readiness metadata only, never secret values.

BaseHarbor also supports app-scoped dynamic secret references through the runtime broker so applications can store an opaque reference while the credential remains in OpenBao.

The v0.4 declarative input resolver can represent secret inputs, but generic persistable output excludes them and string rendering is redacted. Secret delivery/storage remains a dedicated capability-provider concern rather than becoming ordinary committed deployment state.

## Deployment TLS

The v0.4 Compose deployment initializer supports deployment TLS modes without adding TLS provider details to the portable application manifest. For existing/BYOC certificates, BaseHarbor validates and stores the normalized certificate/key pair in protected runtime state.

Check or install a newer certificate from the configured source directory:

```bash
baha app tls update --check
baha app tls update
```

The update path validates certificate/key matching and FQDN coverage, refuses certificate downgrades, restarts the repository workload when required and verifies readiness. ACME automation, OpenBao PKI issuance and provider-neutral certificate realization remain future capability-provider work.

## Backup and restore

Interactive terminals can use the guided flow:

```bash
baha app backup
baha app restore ./mailflow-production.bhbackup
```

Automation keeps the deterministic password-file path:

```bash
baha app backup --password-file ./backup-password.txt
baha app restore ./mailflow-production.bhbackup --password-file ./backup-password.txt
```

The recovery unit includes desired application metadata, every managed PostgreSQL instance and the application-owned OpenBao secret scope. Restore validates and decrypts before mutation, rebuilds protected state, restores data while the workload is stopped, regenerates runtime identities and reports READY only after the restarted application boundary has been verified.

## Updates

Inspect a Git-backed application update without mutation:

```bash
baha app update --check
```

Application mutation is strict fast-forward only and reuses the normal application reconciliation/readiness lifecycle. Durable applications require either an encrypted pre-update recovery point or explicit acknowledgement to proceed without one.

Inspect BaseHarbor itself for an available stable release:

```bash
baha update --check
```

Actual self-update requires explicit confirmation and verifies release artifacts before atomic replacement. A retained recovery binary is used to roll back when post-update verification fails.

## Runtime/provider direction

v0.4 introduces runtime-provider identity and capability negotiation without pretending Kubernetes/OpenShift already exist. The current application runtime is Compose. Future providers must satisfy the requested runtime capabilities or fail clearly; they must not silently downgrade into Compose-specific behavior.

Runtime-provider choice and backend capability-provider choice remain independent. A future Kubernetes/OpenShift deployment may therefore use bundled, external or customer-managed PostgreSQL, Valkey, S3 and secrets providers behind the same logical application requirements.

See [`docs/roadmap.md`](docs/roadmap.md) for the staged provider roadmap.

## CLI discovery

```bash
baha --help
baha app --help
baha openbao --help
baha version
```

The detailed current command tree is documented in [`docs/cli.md`](docs/cli.md).

## Design goals

- one dependable CLI for setup and lifecycle management;
- secure defaults, least privilege and fail-closed behavior;
- isolated backend service stacks for independent applications;
- native protocols and standard interfaces for application consumption;
- applications remain runnable without BaseHarbor when equivalent interfaces are supplied elsewhere;
- developer-first from local/self-hosted development through progressively stricter environments;
- Compose first and complete; later runtime providers may include Kubernetes and OpenShift without redefining logical application requirements;
- mature open-source components instead of unnecessary reinvention;
- observable health, backup/restore, certificates and lifecycle operations;
- independent runtime-provider and capability-provider boundaries that avoid locking applications to bundled infrastructure products.

## Release policy

`main` is development state. Real products should consume published releases. During `0.x`, patch releases remain compatible within a minor line; minor releases may contain explicitly documented breaking changes. See [`docs/releases.md`](docs/releases.md).

## License

Apache License 2.0. See [LICENSE](LICENSE).
