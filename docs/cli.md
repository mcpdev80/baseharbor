# `baha` CLI

`baha` is the primary operator and developer interface for BaseHarbor. The CLI owns setup, inspection, provisioning, verification, backup/restore, updates and controlled lifecycle operations while applications continue to consume standard protocols, environment variables and files.

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
├── update
├── app
│   ├── init
│   ├── inspect
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
│   ├── update
│   ├── psql
│   ├── redis
│   ├── valkey
│   ├── creds
│   ├── logs
│   ├── shell
│   ├── exec
│   ├── tls
│   │   └── update
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
baha app update --help
baha app tls update --help
baha update --help
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

## Read-only repository inspection

```bash
baha app inspect .
baha app inspect . --json
```

`app inspect` is strictly read-only. The shared repository-inspection core collects deterministic evidence from Compose files, Dockerfiles, dependency manifests, example/env variable names, source imports, configuration files, published ports and health checks.

Findings are classified as:

- **Detected** — strong evidence that may be accepted automatically by `app init --quick`;
- **Suggested** — useful evidence requiring developer confirmation;
- **Possible** — weak/configuration evidence that is never auto-selected.

The JSON form is the canonical machine-readable result intended for reuse by future API/Web UI/Operator adapters. Environment values are discarded during collection; only variable names are retained. Symlinked files and generated/vendor directories are ignored.

## Repository-first application workflow

The normal developer path can start inside an existing application repository with:

```bash
baha app init
```

Before asking setup questions, `baha` analyzes the repository read-only and detects as much as it can safely derive, including:

- common Compose files in the repository root and supported conventional subdirectories;
- PostgreSQL and Redis/Valkey usage;
- likely application workload services;
- infrastructure variables from common example/template env files;
- likely required application secret names.

Secret values are never copied into the manifest. The interactive rule is **detect first, ask only what is unclear**.

The wizard shows a compact capability selection with detected choices preselected. Developers may override them. If several Compose files are plausible, BaseHarbor asks explicitly instead of guessing.

When more than one PostgreSQL or Redis/Valkey backend is visible, the wizard proposes logical instance names automatically. A single detected backend stays the simple `default` instance.

Before writing anything, the generated `baseharbor.yaml` is shown as a preview. An existing manifest is never silently overwritten.

For a non-interactive detection-based path:

```bash
baha app init --quick
```

`--quick` accepts only unambiguous **Detected** evidence plus an explicit detected workload. `Suggested` or `Possible` evidence is never promoted automatically. Credential-looking names from env/example files remain heuristic input suggestions and are never turned into `secrets.required` without explicit developer confirmation. If no strong requirement and no workload is detected, quick mode fails closed and points to interactive setup or explicit flags. Ambiguous project structure also fails closed. Multiple detected logical PostgreSQL or Redis/Valkey instances are preserved automatically.

The explicit flag-based path remains available and deterministic for CI, scripts and developers who already know the desired contract:

```bash
baha app init mailflow \
  --environment production \
  --postgres \
  --redis \
  --require-secret SECRET_KEY
```

If the name is omitted from the explicit path, `app init` derives it from the current directory. The generated file is intended to be reviewed and committed.

In v0.4, repository deployments initialize protected deployment/runtime state for the current Compose realization through the declarative input resolver. Interactive setup may request a **Public FQDN** and TLS mode. Existing/BYOC certificate mode accepts a source directory, validates the matching certificate/key pair and FQDN coverage, and normalizes the pair into owner-only BaseHarbor state. These deployment details do not become portable fields in `baseharbor.yaml`.

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

Each logical instance receives independent credentials, persistent state and stable bindings. Multiple instances are not HA replicas; HA is a separate topology concern behind one logical service contract.

## Plan, preflight, apply and verify

```text
plan -> preflight -> apply -> verify
```

`plan` and `preflight` are read-only. `apply` validates desired state, materializes owned runtime state, converges managed services, secret scope, runtime identity/broker and repository workload where applicable, then returns success only after verification.

Required secrets are a startup gate. Missing or unusable required secrets prevent the workload from starting.

Repository workload readiness is service-level and health-aware. When conventional HTTP/HTTPS publishers exist, BaseHarbor also probes the locally published endpoint. Redirects count as reachable exposure; 5xx/unreachable endpoints do not. For hostname-bound HTTPS, the local socket is probed using the configured public FQDN as HTTP Host/TLS ServerName.

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

## Trusted-local developer access

The current trusted-local workflow can open normal clients and application workload tooling through logical BaseHarbor resource/service names:

```bash
baha app psql [INSTANCE]
baha app redis [INSTANCE]
baha app valkey [INSTANCE]
baha app creds postgres [INSTANCE]
baha app creds valkey [INSTANCE]
baha app logs [SERVICE]
baha app shell SERVICE
baha app exec SERVICE COMMAND [ARG...]
```

Database/cache commands avoid placing passwords in normal command arguments. Credential output is masked by default; explicit reveal is separate. Compose container selection remains an internal provider detail rather than a developer-facing identity.

See [developer-access.md](developer-access.md).

## Managed secrets

```bash
printf '%s' "$API_TOKEN" | baha app secret set API_TOKEN --stdin
baha app secret set TLS_KEY_FILE --file ./private-key.pem
baha app secret list
baha app secret delete API_TOKEN --yes
```

Validate and import a certificate/key pair into managed application secrets when an application explicitly needs secret material:

```bash
baha app secret tls-set \
  --cert-file ./certificate.pem \
  --key-file ./private-key.pem \
  --chain-file ./intermediate.pem
```

This command is separate from the repository deployment TLS lifecycle described below. The CLI never uses status/list operations to reveal secret values.

## Deployment TLS lifecycle

For repository deployments initialized with `tls: existing`:

```bash
baha app tls update --check
baha app tls update
```

`--check` is read-only. Mutation validates the configured source certificate/key pair and FQDN coverage, refuses certificate downgrades, writes owner-only normalized files, restarts the repository workload when required and verifies readiness. If recovery fails, the previous protected certificate state is restored and the operation returns failure.

`baha app status` reports TLS mode, expiry, source/update information when deployment TLS state exists. `baha app doctor` adds certificate/key/FQDN/expiry diagnostics.

ACME automation, OpenBao PKI issuance and provider-neutral certificate lifecycle contracts are future work; v0.4 does not claim them.

## Dynamic runtime identity

```bash
baha app runtime-identity rotate --yes
baha app runtime-identity revoke --yes
```

Stored opaque `baseharbor://` secret references remain stable across runtime-identity rotation.

## Status, show and doctor

```bash
baha app show
baha app status
baha app doctor
```

`app show` gives a read-only human-facing overview. `app status` and `app doctor` use the same workload truth for selected Compose services and HTTP/TLS exposure. Diagnostics include manifest integrity, permissions, Compose ownership/configuration, PostgreSQL/Valkey protocol readiness, OpenBao application scope, required-secret usability, repository workload and runtime broker/mTLS readiness when enabled. Secret values and credential-bearing URLs are not printed.

## Stop, resume and destroy

```bash
baha app down
baha app up
baha app destroy
baha app destroy --yes
baha app destroy --yes --full-reset
```

Normal `app destroy` preserves repository deployment settings (`.baseharbor/init.env`) and normalized local TLS state so an application can be recreated with the same deployment choices. The destruction plan shows these preserved paths. `--full-reset` additionally removes those BaseHarbor-owned repository deployment files, but still preserves `baseharbor.yaml`, application-owned Compose data/volumes and any external certificate source directory.

`app up` resumes only already-materialized state. Missing expected persistent state causes a fail-closed error instead of silently creating an empty replacement.

## Backup and restore

Interactive guided flow:

```bash
baha app backup
baha app restore ./demo-production.bhbackup
```

Deterministic automation:

```bash
baha app backup --password-file ./backup-password.txt
baha app backup --password-file ./backup-password.txt --output ./demo-production.bhbackup
baha app restore ./demo-production.bhbackup --password-file ./backup-password.txt
```

Interactive password entry disables terminal echo, requires confirmation and never places the password in argv. Restore validates/decrypts before mutation, remains fail-closed, and reports READY only after backend, secret/runtime identity, workload and exposure verification succeed. Last successful backup/recovery metadata is stored without secret-bearing detail and shown by `baha app show`.

See [backup-and-restore.md](backup-and-restore.md).

## Application updates

Read-only Git update inspection:

```bash
baha app update --check
```

Mutation uses a strict fast-forward-only model: named branch, configured upstream, clean working tree, no divergence, exact fetched target SHA and post-mutation verification. BaseHarbor never resets, stashes, rebases or silently discards local work.

For applications with durable BaseHarbor-managed state, mutation requires either an encrypted pre-update recovery point or explicit acknowledgement:

```bash
baha app update --backup-password-file ./backup-password.txt
baha app update --no-backup
```

After source advancement, BaseHarbor reuses the normal application apply/readiness lifecycle. Success is reported only after the updated application is READY. Protected update metadata records non-secret before/after state and recovery metadata where applicable.

## BaseHarbor self-update

Read-only release inspection:

```bash
baha update --check
```

Stable is the default channel. Prerelease or exact-version selection is explicit. Mutation requires explicit confirmation and performs checksum/release-asset validation before replacing the current regular executable atomically. BaseHarbor does not invoke `sudo` automatically. A recovery binary is retained, and failed post-update CLI/runtime verification triggers rollback.

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


## Managed HTTP exposure

Managed exposure is declared in `baseharbor.yaml`; it does not add a separate product-specific Caddy command surface. Normal lifecycle commands converge and observe it:

```bash
baha app apply
baha app status
baha app doctor
baha app down
baha app up
baha app destroy --yes
```

`app status` and `app doctor` render end-to-end managed exposure readiness from the shared endpoint/exposure state. Redirects remain reachable; HTTP 5xx and unreachable routes are NOT READY.

Existing application-owned HTTP/HTTPS publishers keep the previous discovery/observation path and are not taken over by the managed provider lifecycle.
