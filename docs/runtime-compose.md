# Local control-plane runtime

BaseHarbor's current operational control plane is intentionally single-node and local-first.

## Services

`baha up` materializes an embedded Compose definition and starts:

- PostgreSQL 18;
- OpenBao 2.6.x.

Both services bind to loopback by default. They are not exposed on all host interfaces.

## First-run port selection

The default host ports are PostgreSQL `5432` and OpenBao `8200`, but BaseHarbor checks them before first initialization.

Interactive setup:

```bash
baha up
```

If a default port is occupied, `baha` proposes a free alternative. The operator can accept the proposal or enter another port.

Non-interactive setup accepts BaseHarbor's safe proposals:

```bash
baha up --yes
```

Explicit ports are supported and still fail closed when occupied:

```bash
baha up --postgres-port 15432 --openbao-port 18200
```

After control-plane state has been initialized, `baha up` does not silently rewrite configured ports.

## Runtime state

On the current default branch, control-plane runtime files are still resolved through the legacy local state location:

```text
.baseharbor/runtime/
├── compose.yaml
├── runtime.env
└── openbao-admin.env     # after OpenBao bootstrap
```

Files are owner-only where credentials are present. The PostgreSQL password is generated from cryptographically secure random bytes on first materialization and preserved across subsequent starts.

**Pre-v0.1 release blocker:** the Docker control plane and named volumes are machine/user scoped, so the default runtime state must also be machine/user scoped. PR #72 moves the default to XDG/Home global state while preserving explicit overrides and legacy compatibility. Public release documentation must be updated to the merged final path before `v0.1.0` is tagged.

## Commands

```bash
baha up
baha status
baha doctor
baha down
```

`baha up` validates the generated Compose configuration before starting containers.

`baha status` reports actual readiness, not only whether containers are alive.

`baha doctor` verifies host/runtime prerequisites and reports control-plane failures independently.

## OpenBao lifecycle

OpenBao does not run with a static development root token.

Start the runtime:

```bash
baha up
```

Inspect initialization/seal state:

```bash
baha openbao status
```

Bootstrap a fresh OpenBao instance with an explicit recovery destination:

```bash
baha openbao bootstrap --recovery-file /secure/off-host/openbao-recovery.json
```

Bootstrap:

- initializes and unseals the current single-node Shamir profile;
- creates the `baseharbor/` KV v2 mount;
- enables AppRole authentication;
- creates and verifies the restricted BaseHarbor manager identity;
- stores manager RoleID/SecretID only in protected control-plane state;
- does not persist or print the initial root token;
- revokes the initial root token after manager verification;
- writes recovery material only to the explicit owner-controlled recovery file.

After a restart, explicitly unseal the current manual profile:

```bash
baha openbao unseal --recovery-file /secure/off-host/openbao-recovery.json
baha openbao status
```

Automatic KMS/HSM/transit unseal is a future deployment profile, not current single-node behavior.

## Scope

The current control plane intentionally does not yet imply:

- high availability;
- bundled OIDC provider;
- public network exposure;
- Kubernetes deployment;
- KMS/HSM automatic unseal.

Those are separate deployment/architecture concerns so BaseHarbor can preserve a small, testable default profile.