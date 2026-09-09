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

Use the executable help output as the precise syntax reference for every command:

```bash
baha --help
baha app --help
baha app backup --help
baha openbao --help
```

## Exit codes

| Code | Meaning |
| --- | --- |
| `0` | command completed successfully |
| `1` | operational/runtime failure |
| `2` | invalid command or arguments |

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

## Repository-first application workflow

Create `baseharbor.yaml` in the application repository:

```bash
baha app init mailflow \
  --environment production \
  --postgres \
  --redis \
  --require-secret SECRET_KEY
```

If the name is omitted, `app init` derives it from the current directory. The generated file is intended to be reviewed and committed.

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

The simple case remains:

```bash
baha app init demo --postgres --redis
```

Named logical instances are explicit:

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

## Application environment and bindings

BaseHarbor publishes normal connection information and protected file bindings rather than requiring an SDK.

Inspect masked values:

```bash
baha app env
baha app env --format json
baha app env --format yaml
baha app env --format shell
```

Print the protected dotenv file path:

```bash
baha app env --path
```

Credential-bearing values are masked by default. Revealing them is an explicit operation.

See [application-contract.md](application-contract.md) for the stable runtime-facing contract.

## Managed secrets

Write a value from stdin:

```bash
printf '%s' "$API_TOKEN" | baha app secret set API_TOKEN --stdin
```

Write from a file:

```bash
baha app secret set TLS_KEY_FILE --file ./private-key.pem
```

List names only:

```bash
baha app secret list
```

Delete with explicit confirmation:

```bash
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

Applications can use app-scoped dynamic secret references without a human BaseHarbor login. The runtime identity is scoped to one application/environment and does not grant operator API access.

Rotate it:

```bash
baha app runtime-identity rotate --yes
```

Revoke it:

```bash
baha app runtime-identity revoke --yes
```

Stored opaque `baseharbor://` secret references remain stable across runtime-identity rotation.

## Status and doctor

```bash
baha app status
baha app doctor
```

`app status` is compact and checks running/readiness state.

`app doctor` diagnoses boundaries independently, including manifest integrity, permissions, Compose ownership/configuration, PostgreSQL/Valkey protocol readiness, OpenBao application scope, required-secret usability, repository workload and runtime broker/mTLS readiness when enabled.

Neither command prints credentials.

## Stop, resume and destroy

```bash
baha app down
baha app up
```

`app down` removes transient containers/network state while preserving owned persistent data and managed application state.

`app up` resumes only already-materialized state. Missing expected persistent state causes a fail-closed error instead of silently creating an empty replacement.

Preview permanent destruction:

```bash
baha app destroy
```

Confirm:

```bash
baha app destroy --yes
```

Destruction verifies exact BaseHarbor ownership before removing managed resources.

## Backup and restore

Create an encrypted recovery unit:

```bash
baha app backup --password-file ./backup-password.txt
```

Choose output explicitly:

```bash
baha app backup \
  --password-file ./backup-password.txt \
  --output ./demo-production.bhbackup
```

Restore:

```bash
baha app restore ./demo-production.bhbackup \
  --password-file ./backup-password.txt
```

See [backup-and-restore.md](backup-and-restore.md) for capture and restore semantics.

## OpenBao trust-plane lifecycle

```bash
baha up
baha openbao status
baha openbao bootstrap --recovery-file /secure/off-host/openbao-recovery.json
baha openbao status
```

After restart:

```bash
baha openbao unseal --recovery-file /secure/off-host/openbao-recovery.json
```

Bootstrap creates the restricted manager identity, verifies it and revokes the initial root token. Recovery material is kept outside normal application state.

See [secrets-and-openbao.md](secrets-and-openbao.md).

## Version

```bash
baha version
```

Development builds may identify themselves as development/untagged builds. Official releases will embed the semantic version, commit and build date as part of the release contract.