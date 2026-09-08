# Secrets and OpenBao

BaseHarbor keeps secret consumption behind the provider-neutral `credential.Broker` interface. Applications and higher-level platform modules work with opaque credential references rather than OpenBao-specific implementation types.

The bundled single-node control-plane OpenBao has an explicit operator bootstrap/unseal lifecycle through `baha`. Application manifests can additionally request an isolated managed secret scope. These lifecycle operations use the local container-runtime boundary; the separate network credential adapter continues to require HTTPS.

## Security model

### Credential consumption

- OpenBao network connections through the credential adapter require HTTPS.
- Authentication tokens are configuration-only secret material and are excluded from JSON serialization.
- Credential references are resolved through a KV v2 mount.
- Secret payloads are returned through `credential.Data`, whose payload is excluded from JSON serialization.
- OpenBao response bodies and authentication tokens are never included in returned errors.
- 403 and 404 responses are mapped to stable, non-sensitive BaseHarbor adapter errors.
- Caller-provided scope is not automatically interpolated into an OpenBao path. Authorization and tenant/application isolation remain explicit.

### Platform bootstrap

The bundled single-node profile follows a manual Shamir unseal workflow first, matching the roadmap requirements.

```bash
baha up
baha openbao bootstrap --recovery-file /secure/off-host/openbao-recovery.json
baha openbao status
```

Bootstrap performs these steps:

1. verifies the control-plane runtime exists and OpenBao is not already initialized
2. requires an explicit recovery destination outside `.baseharbor`
3. reserves that recovery file owner-only before initializing OpenBao
4. initializes OpenBao with one key share and threshold one for the current single-node profile
5. writes only the unseal material to the recovery file
6. unseals OpenBao without placing the key in the host process argument list
7. enables a KV v2 mount at `baseharbor/`
8. enables AppRole authentication
9. installs a restricted `baseharbor-manager` policy
10. creates a BaseHarbor manager AppRole and stores only its RoleID/SecretID in owner-only control-plane runtime state
11. verifies that the manager can authenticate and perform application-secret KV operations
12. revokes the initial root token
13. verifies manager authentication again after root revocation

The manager policy may provision only BaseHarbor-named application policies and AppRoles (`baseharbor-app-*`). It does not receive the root policy or unrestricted policy/auth administration.

The initial root token is never printed and is not intentionally persisted. If bootstrap fails after OpenBao initialization, the operator-held recovery material remains the recovery boundary for generating a new root token through standard OpenBao recovery procedures.

## Recovery material

The recovery file is deliberately not given a default location. The operator must choose it explicitly:

```bash
baha openbao bootstrap --recovery-file /secure/off-host/openbao-recovery.json
```

Rules:

- it must be outside `.baseharbor`
- BaseHarbor creates it with owner-only permissions
- an existing file is never overwritten
- unseal material is not printed by `baha`
- the file should be stored separately from BaseHarbor application and OpenBao storage

After an OpenBao restart, the current Shamir profile is expected to be sealed:

```bash
baha openbao unseal --recovery-file /secure/off-host/openbao-recovery.json
baha openbao status
```

The unseal command reads only an owner-only recovery file and passes the key through stdin to the container-runtime boundary.

## Manager identity

BaseHarbor does not retain the initial root token for normal management. Bootstrap creates a restricted AppRole instead.

The persistent local bootstrap credential is stored at:

```text
.baseharbor/runtime/openbao-admin.env
```

It contains only:

```text
OPENBAO_ROLE_ID=...
OPENBAO_SECRET_ID=...
```

The file is owner-only and must never be logged, rendered through normal status output, committed, or copied into application manifests.

## Application secret scopes

An application enables managed secrets declaratively:

```yaml
services:
  postgres:
    enabled: true
  secrets:
    enabled: true
```

For the current milestone, managed secrets are supported alongside PostgreSQL and/or Valkey. A secrets-only application is rejected until BaseHarbor has an application lifecycle that does not depend on a materialized Compose workload.

`baha app apply NAME` provisions:

- one exact KV v2 secret document at `baseharbor/apps/<app>/<environment>`
- one policy named `baseharbor-app-<app>-<environment>`
- one AppRole with the same BaseHarbor-managed name
- one owner-only local bootstrap credential file at `.baseharbor/apps/<app>/runtime/openbao.env`

The local file contains only the application's RoleID and SecretID. It does not contain application secret payloads, a manager token, an unseal key, or a root token.

The application policy has no wildcard access to another application or environment. BaseHarbor also reserves a separate verification document below `baseharbor/apps/_baseharbor-probes/<app>/<environment>`. Apply/up verification writes only to that probe document; it never overwrites or deletes the application's real secret document.

`baha app status NAME` and `baha app doctor NAME` are read-only. They authenticate the application AppRole, inspect required token capabilities, and validate that the AppRole and policy still match the BaseHarbor-managed definition. They do not perform KV writes or deletes.

`baha app down NAME` preserves the OpenBao scope. `baha app destroy NAME --yes` removes the application's secret metadata, probe metadata, AppRole and policy as part of permanent destruction. Destructive OpenBao mutation is refused when the AppRole/policy ownership definition has been changed unexpectedly.

### Current application-consumption boundary

This milestone establishes the isolated server-side scope and application identity. It does **not** yet inject OpenBao credentials into application containers or claim that arbitrary application code can reach the bundled loopback-only OpenBao listener directly.

Applications will continue to use native/provider-standard mechanisms as runtime connectivity and credential injection are added. BaseHarbor does not require a proprietary application SDK.

## Boundary

The secret-consumption adapter remains below the generic credential contract:

```text
application/module
      |
      v
credential.Broker
      |
      +-- NoopBroker
      +-- openbao.Client
```

The platform bootstrap and managed-identity lifecycle are operator/control-plane boundaries:

```text
baha openbao ... / baha app apply ...
      |
      v
platform OpenBao lifecycle
      |
      v
local Compose/OpenBao runtime
```

This separation prevents bootstrap privileges from leaking into normal application secret-resolution paths.

## Trust-plane compatibility

A trust plane bootstrapped by a BaseHarbor version before application-scope provisioning does not automatically gain the newer manager-policy permissions. BaseHarbor fails closed when those permissions are absent instead of attempting privilege escalation with insufficient credentials.

During the current pre-release phase, such a trust plane must be explicitly rebuilt/re-bootstrapped or reconciled by an operator before `services.secrets` convergence can succeed. A dedicated in-place trust-plane reconciliation workflow remains future work.

## Current scope

Implemented:

- read-only KV v2 secret resolution through the HTTPS-only credential adapter
- bundled OpenBao state inspection
- explicit single-node bootstrap and manual unseal
- `baseharbor/` KV v2 mount creation
- restricted manager AppRole provisioning and initial root-token revocation
- owner-only recovery and manager credential files
- application `services.secrets` convergence alongside PostgreSQL/Valkey
- exact per-application/environment KV scopes
- application-specific policies and AppRoles
- owner-only application RoleID/SecretID bootstrap state
- read-only application scope status/doctor inspection
- fail-closed destructive ownership validation
- negative cross-application isolation verification in CI

Not yet implemented:

- secrets-only application runtimes
- injecting application OpenBao credentials into workload containers
- direct workload connectivity to the bundled loopback-only OpenBao listener
- moving PostgreSQL/Valkey credentials out of local runtime files into OpenBao
- dynamic PostgreSQL credentials
- credential rotation
- token renewal/agent integration
- TLS/PKI for the bundled OpenBao listener
- KMS/HSM/transit auto-unseal profiles
