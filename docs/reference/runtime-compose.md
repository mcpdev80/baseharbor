# Local control-plane runtime

BaseHarbor's current operational control plane is intentionally single-node. On Docker and Podman Targets it is local-first and owned by the effective Target.

## Services

`baha up` materializes the BaseHarbor runtime definition and starts:

- PostgreSQL 18;
- OpenBao 2.7.x.

Both services bind to loopback by default.

Docker executes the generated runtime through Docker Compose. Podman consumes the same Compose-based runtime model, renders native Quadlet units, and manages them through rootless `systemd --user`. BaseHarbor does not require `podman-compose` for the Podman lifecycle.

The managed control-plane containers are hardened runtime components rather than privileged bootstrap helpers. PostgreSQL and OpenBao run with explicit non-root identities, read-only root filesystems, all Linux capabilities dropped and `no-new-privileges`. Only the paths that must remain writable are exposed as dedicated volumes or tmpfs mounts.

OpenBao 2.7 starts directly on its final storage model: a dedicated `openbao` database and least-privilege `openbao` login inside the existing BaseHarbor PostgreSQL control-plane provider. No separate Raft lifecycle and no compatibility path for unreleased `storage.file` state are retained.

The initial control-plane bootstrap is TLS-only. BaseHarbor creates short-lived bootstrap trust for PostgreSQL and OpenBao, provisions the dedicated OpenBao database/user during first PostgreSQL initialization, and starts OpenBao with PostgreSQL `verify-full`. After OpenBao PKI is ready, BaseHarbor rotates both control-plane services onto managed PKI material.

OpenBao terminates HTTPS natively. OpenBao 2.7 `tls_auto_reload` is enabled for listener certificate rotation; PostgreSQL certificate rotation is reconciled independently because PostgreSQL copies its private key into an owner-only tmpfs path at process start.

Credential-bearing files are owner-only. Generated PostgreSQL credentials, the dedicated OpenBao storage credential and selected ports are preserved across subsequent starts.

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

After a restart, the normal path is simply:

```bash
baha up
baha openbao status
```

`baha up` uses the recovery-file path persisted for the effective Target. The recovery material remains operator-held outside normal BaseHarbor state. `baha up --recovery-file PATH` remains the explicit override when the file was moved or a custom location should be used.

Automatic KMS/HSM/transit unseal is a future deployment profile, not current single-node behavior.

## Scope

The current control plane does not imply high availability, public network exposure, Kubernetes deployment or KMS/HSM automatic unseal. Those are separate deployment/architecture concerns.