# Local control-plane runtime

BaseHarbor's current operational control plane is intentionally single-node and local-first.

## Services

`baha up` materializes an embedded Compose definition and starts:

- PostgreSQL 18;
- OpenBao 2.6.x.

Both services bind to loopback by default.

## First-run port selection

The default host ports are PostgreSQL `5432` and OpenBao `8200`, but BaseHarbor checks them before first initialization.

```bash
baha up
```

If a default port is occupied, `baha` proposes a free alternative. Non-interactive setup can accept safe proposals:

```bash
baha up --yes
```

Explicit ports are supported and still fail closed when occupied:

```bash
baha up --postgres-port 15432 --openbao-port 18200
```

After initialization, `baha up` does not silently rewrite configured ports.

## Runtime state

The control plane is machine/user scoped, so its runtime files are user-global by default:

```text
$XDG_DATA_HOME/baseharbor/runtime/
```

or, when `XDG_DATA_HOME` is unset:

```text
~/.local/share/baseharbor/runtime/
```

Typical files include:

```text
runtime/
├── compose.yaml
├── runtime.env
└── openbao-admin.env
```

`BASEHARBOR_STATE_DIR` remains an explicit operator/CI override. For compatibility, a legacy repository-local `.baseharbor/runtime` is reused only when no global state exists yet.

Credential-bearing files are owner-only. Generated PostgreSQL credentials and selected ports are preserved across subsequent starts.

## Commands

```bash
baha up
baha status
baha doctor
baha down
```

`baha status` reports actual readiness, not only whether containers are alive.

## OpenBao lifecycle

OpenBao does not run with a static development root token.

```bash
baha openbao status
baha openbao bootstrap --recovery-file /secure/off-host/openbao-recovery.json
baha openbao status
```

Bootstrap initializes and unseals the current single-node Shamir profile, enables the `baseharbor/` KV v2 mount and AppRole auth, creates and verifies the restricted BaseHarbor manager identity, then revokes the initial root token.

After a restart:

```bash
baha openbao unseal --recovery-file /secure/off-host/openbao-recovery.json
baha openbao status
```

Automatic KMS/HSM/transit unseal is a future deployment profile, not current single-node behavior.

## Scope

The current control plane does not imply high availability, public network exposure, Kubernetes deployment or KMS/HSM automatic unseal. Those are separate deployment/architecture concerns.