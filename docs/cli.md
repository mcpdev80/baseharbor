# `baha` CLI

`baha` is the primary operator and developer interface for BaseHarbor. The CLI owns setup, inspection, provisioning, verification, backup/restore and controlled lifecycle operations while applications continue to consume standard protocols, environment variables and files.

## Design rules

- one binary with no required language runtime for released builds;
- hierarchical, discoverable commands with `--help`;
- repository-owned `baseharbor.yaml` is the preferred application source of truth;
- read-only inspection is distinct from mutation;
- usage errors and operational failures use different exit semantics;
- secret values are not accepted as normal positional arguments and are not printed by status/doctor/list operations;
- readiness means authenticated protocol/application verification, not merely a running container;
- mutation follows preflight and is followed by verification;
- destructive operations verify exact ownership and fail closed on ambiguity.

## Current command tree

```text
baha
├── init
├── up
├── down
├── status
├── doctor
├── serve
├── app
│   ├── init
│   ├── create
│   ├── list
│   ├── show
│   ├── plan
│   ├── preflight
│   ├── apply
│   ├── env
│   ├── status
│   ├── doctor
│   ├── backup
│   ├── restore
│   ├── down
│   ├── up
│   ├── destroy
│   ├── runtime-identity
│   │   ├── rotate
│   │   └── revoke
│   └── secret
│       ├── set
│       ├── list
│       ├── delete
│       └── tls-set
├── openbao
│   ├── status
│   ├── bootstrap
│   └── unseal
└── version
```

Use executable help output for precise syntax:

```bash
baha --help
baha app --help
baha app backup --help
baha openbao --help
```

## Control-plane startup

Interactive first run:

```bash
baha up
```

Automation accepts BaseHarbor's safe free-port proposals:

```bash
baha up --yes
```

Explicit first-run ports:

```bash
baha up --postgres-port 15432 --openbao-port 18200
```

The default ports are checked before first initialization. An occupied default port is not blindly bound.

Control-plane state is user-global by default under `$XDG_DATA_HOME/baseharbor/runtime` or `~/.local/share/baseharbor/runtime` when XDG is unset. `BASEHARBOR_STATE_DIR` is the explicit override.

## Repository-first application workflow

The normal developer path can start inside an existing application repository with:

```bash
baha app init
```

Before asking setup questions, `baha` analyzes the repository read-only and detects as much as it can safely derive, including:

- common Compose files in the repository root and under `deploy/` or `docker/`;
- PostgreSQL and Redis/Valkey usage;
- likely application workload services;
- infrastructure variables from `.env.example`, `.env.template`, `.env.sample` and `.env`;
- likely required application secret names.

Secret values are never copied into the manifest. The interactive rule is **detect first, ask only what is unclear**.

Example detection summary:

```text
Analyzing repository...
✓ Application name: mailflow
✓ Compose file: deploy/docker-compose.yml
✓ PostgreSQL detected
✓ Redis/Valkey detected
✓ Potential required secret names:
    OPENAI_API_KEY
    SMTP_PASSWORD
```

The wizard then shows a compact capability selection with detected choices preselected. Developers may override them. If several Compose files are plausible, BaseHarbor asks explicitly instead of guessing.

When more than one PostgreSQL or Redis/Valkey backend is visible, the wizard proposes logical instance names automatically. A single detected backend stays the simple `default` instance.

Example:

```text
✓ PostgreSQL detected from compose.yaml service postgres-primary
  logical instances proposed: analytics, primary
✓ Redis/Valkey detected from compose.yaml service redis-cache
  logical instances proposed: cache, sessions

PostgreSQL instances (comma-separated) [analytics,primary]:
Valkey / Redis instances (comma-separated) [cache,sessions]:
```

Before writing anything, the generated `baseharbor.yaml` is shown as a preview. An existing manifest is never silently overwritten.

For a non-interactive detection-based path:

```bash
baha app init --quick
```

`--quick` accepts only unambiguous detections plus safe defaults. Ambiguous project structure fails closed and points back to the interactive flow. Multiple detected logical PostgreSQL or Redis/Valkey instances are preserved automatically.

The explicit flag-based path remains available and deterministic for CI, scripts and developers who already know the desired contract:

```bash
baha app init mailflow \
  --environment production \
  --postgres \
  --redis \
  --require-secret SECRET_KEY
```

If the name is omitted from the explicit path, `app init` derives it from the current directory. The generated file is intended to be reviewed and committed.

Afterward, commands resolve the nearest repository manifest and normally do not need `NAME`:

```bash
baha app show
baha app plan
baha app preflight
baha app apply
baha app status
baha app doctor
```

`baha app create NAME ...` remains for legacy/BaseHarbor-managed stored application state. New application repositories should prefer `baha app init`.

## Multiple PostgreSQL and Valkey instances

```bash
baha app init demo --postgres --redis
```

Named logical instances:

```bash
baha app init demo \
  --postgres-instance primary \
  --postgres-instance analytics \
  --redis-instance cache \
  --redis-instance sessions
```

The interactive wizard and `--quick` path generate the same logical-instance contract as these deterministic flags. Each logical instance receives independent credentials, persistent state and stable bindings. Multiple instances are not HA replicas; HA is a separate topology concern behind one logical service contract.

## Plan, preflight, apply and verify

```text
plan -> preflight -> apply -> verify
```

`plan` and `preflight` are read-only. `apply` validates desired state, materializes owned runtime state, converges managed services, secret scope, runtime identity/broker and repository workload where applicable, then returns success only after verification.

Required secrets are a startup gate. Missing or unusable required secrets prevent the workload from starting.

Once the application secret scope exists, `baha app preflight` reports required-secret readiness directly:

```text
No application secrets have been configured yet.
REQUIRED SECRET       STATUS                         ACTION
OPENAI_API_KEY        missing - user input required  baha app secret set OPENAI_API_KEY --stdin
SMTP_PASSWORD         missing - user input required  baha app secret set SMTP_PASSWORD --stdin
```

Present values are reported as `present`; unreadable values are reported as `present but unusable` with the same safe replacement command. Secret values are never printed.

## Application environment and bindings

BaseHarbor publishes normal connection information and protected file bindings rather than requiring an SDK.

```bash
baha app env
baha app env --format json
baha app env --format yaml
baha app env --format shell
baha app env --path
```

Credential-bearing values are masked by default. Revealing them is an explicit operation.

See [application-contract.md](application-contract.md).

## Managed secrets

```bash
printf '%s' "$API_TOKEN" | baha app secret set API_TOKEN --stdin
baha app secret set TLS_KEY_FILE --file ./private-key.pem
baha app secret list
baha app secret delete API_TOKEN --yes
```

Validate and import a certificate/key pair:

```bash
baha app secret tls-set \
  --cert-file ./certificate.pem \
  --key-file ./private-key.pem \
  --chain-file ./intermediate.pem
```

The CLI never uses status/list operations to reveal secret values.

## Dynamic runtime identity

```bash
baha app runtime-identity rotate --yes
baha app runtime-identity revoke --yes
```

Stored opaque `baseharbor://` secret references remain stable across runtime-identity rotation.

## Status and doctor

```bash
baha app status
baha app doctor
```

`app doctor` diagnoses manifest integrity, permissions, Compose ownership/configuration, PostgreSQL/Valkey protocol readiness, OpenBao application scope, required-secret usability, repository workload and runtime broker/mTLS readiness when enabled.

## Stop, resume and destroy

```bash
baha app down
baha app up
baha app destroy
baha app destroy --yes
```

`app up` resumes only already-materialized state. Missing expected persistent state causes a fail-closed error instead of silently creating an empty replacement.

## Backup and restore

```bash
baha app backup --password-file ./backup-password.txt
baha app backup --password-file ./backup-password.txt --output ./demo-production.bhbackup
baha app restore ./demo-production.bhbackup --password-file ./backup-password.txt
```

See [backup-and-restore.md](backup-and-restore.md).

## OpenBao trust-plane lifecycle

```bash
baha up
baha openbao status
baha openbao bootstrap --recovery-file /secure/off-host/openbao-recovery.json
baha openbao status
baha openbao unseal --recovery-file /secure/off-host/openbao-recovery.json
```

Bootstrap creates the restricted manager identity, verifies it and revokes the initial root token. Recovery material is kept outside normal application state.

See [secrets-and-openbao.md](secrets-and-openbao.md).

## Version

```bash
baha version
```

Official releases embed semantic version, commit and build date as part of the release contract.
