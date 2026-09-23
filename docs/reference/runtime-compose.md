# Local control-plane runtime

BaseHarbor's current operational control plane is intentionally single-node and local-first.

## Services

`baha up` materializes the BaseHarbor runtime definition and starts:

- PostgreSQL 18;
- OpenBao 2.6.x.

Both services bind to loopback by default.

Docker executes the generated runtime through Docker Compose. Podman consumes the same Compose-based runtime model, renders native Quadlet units, and manages them through rootless `systemd --user`. BaseHarbor does not require `podman-compose` for the Podman lifecycle.

The managed control-plane containers are hardened runtime components rather than privileged bootstrap helpers. PostgreSQL and OpenBao run with explicit non-root identities, read-only root filesystems, all Linux capabilities dropped and `no-new-privileges`. Only the paths that must remain writable are exposed as dedicated volumes or tmpfs mounts.

OpenBao writes its generated local configuration into an ephemeral writable `/openbao/config` tmpfs while durable provider data remains on the dedicated `/openbao/file` volume. BaseHarbor sets the image-supported `SKIP_CHOWN` mode because the container already starts as the non-root `openbao` user; no root startup phase or `CAP_CHOWN` exception is required.

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

Typical protected runtime-state files include:

```text
provider-registry.json
runtime/
├── compose.yaml   # canonical generated runtime model; Podman renders this to Quadlet units
├── runtime.env
└── openbao-admin.env
```

With automatic state selection, the provider registry is stored beside `runtime/`, not inside it. This prevents provider metadata from materializing or switching the selected runtime-state location. An explicit `BASEHARBOR_STATE_DIR` remains self-contained and owns its provider registry as well. For compatibility, a legacy repository-local `.baseharbor/runtime` is reused only when no global runtime state exists yet.

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