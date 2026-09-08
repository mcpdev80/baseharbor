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

Managed secrets are currently supported alongside PostgreSQL and/or Valkey. A secrets-only application is rejected until BaseHarbor has an application lifecycle that does not depend on a materialized Compose workload.

`baha app apply NAME` provisions:

- one application/environment KV v2 namespace below `baseharbor/apps/<app>/<environment>/`
- one policy named `baseharbor-app-<app>-<environment>`
- one AppRole with the same BaseHarbor-managed name
- one owner-only local bootstrap credential file at `.baseharbor/apps/<app>/runtime/openbao.env`
- one reserved namespace marker named `_baseharbor`

The local file contains only the application's RoleID and SecretID. It does not contain application secret payloads, a manager token, an unseal key, or a root token.

Each operator-managed secret key is stored as its own KV v2 document:

```text
baseharbor/apps/<app>/<environment>/<KEY>
```

The application policy may wildcard only below the exact application/environment namespace. It has no access to another application or environment. BaseHarbor also reserves a separate verification document below `baseharbor/apps/_baseharbor-probes/<app>/<environment>`. Apply/up verification writes only to that probe document; it never overwrites or deletes operator-managed application secrets.

`baha app status NAME` and `baha app doctor NAME` are read-only. They authenticate the application AppRole and validate that the AppRole and policy still match the BaseHarbor-managed definition. They do not perform KV writes or deletes.

`baha app down NAME` preserves the OpenBao scope. `baha app destroy NAME --yes` removes every managed application secret document, the namespace marker, probe metadata, AppRole and policy as part of permanent destruction. Destructive OpenBao mutation is refused when the AppRole/policy ownership definition has been changed unexpectedly.

## Managing application secret values

Secret values are managed through `baha` without placing them on the command line or printing them back to the terminal.

Create or replace a value:

```bash
printf '%s' 'secret-value' | baha app secret set demo API_TOKEN --stdin
```

The value is read only from stdin. BaseHarbor rejects an empty value, values larger than 1 MiB and non-UTF-8 input. After the KV write, BaseHarbor reads the stored value internally and compares it byte-for-byte before reporting success. The value is not included in normal output or errors.

List configured key names:

```bash
baha app secret list demo
```

Only key names are returned. Secret values are never rendered by this command family.

Preview permanent deletion:

```bash
baha app secret delete demo API_TOKEN
```

Confirm permanent deletion:

```bash
baha app secret delete demo API_TOKEN --yes
```

Confirmed deletion uses KV v2 metadata deletion, removing the selected key's metadata and all historical versions. BaseHarbor then verifies that the key is no longer present. Without `--yes`, the operation is read-only.

Application secret keys are limited to ASCII letters, digits, `_`, `-` and `.`, with a maximum length of 128 characters. Keys may not start with `-` or `.`, and BaseHarbor-reserved names are rejected.

There is intentionally no operator command that prints a secret value. A future workload-consumption path will use provider-standard runtime mechanisms rather than turning `baha` into a general-purpose secret reveal tool.

### Current application-consumption boundary

This milestone provides isolated server-side scope, application identity and safe operator CRUD for secret values. It does **not** yet inject OpenBao credentials or resolved secret values into application containers, and it does not claim that arbitrary application code can reach the bundled loopback-only OpenBao listener directly.

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

The platform bootstrap, managed-identity lifecycle and operator secret mutations are control-plane boundaries:

```text
baha openbao ... / baha app apply ... / baha app secret ...
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

Application scopes created before per-key secret namespaces were introduced must be reconciled with `baha app apply NAME` before `baha app secret ...` is used. Apply rewrites only the BaseHarbor-owned policy/AppRole definition and creates the reserved namespace marker; it does not replace existing RoleID/SecretID state implicitly.

During the current pre-release phase, a trust plane whose manager policy itself is too old must still be explicitly rebuilt/re-bootstrapped or reconciled by an operator. A dedicated in-place trust-plane reconciliation workflow remains future work.

## Current scope

Implemented:

- read-only KV v2 secret resolution through the HTTPS-only credential adapter
- bundled OpenBao state inspection
- explicit single-node bootstrap and manual unseal
- `baseharbor/` KV v2 mount creation
- restricted manager AppRole provisioning and initial root-token revocation
- owner-only recovery and manager credential files
- application `services.secrets` convergence alongside PostgreSQL/Valkey
- exact per-application/environment KV namespaces
- application-specific policies and AppRoles
- owner-only application RoleID/SecretID bootstrap state
- read-only application scope status/doctor inspection
- stdin-only operator secret writes with post-write verification
- secret-key listing without values
- previewed and confirmed permanent per-key deletion
- fail-closed destructive ownership validation
- negative cross-application isolation verification in CI

Not yet implemented:

- secrets-only application runtimes
- injecting application OpenBao credentials or resolved values into workload containers
- direct workload connectivity to the bundled loopback-only OpenBao listener
- moving PostgreSQL/Valkey credentials out of local runtime files into OpenBao
- dynamic PostgreSQL credentials
- credential rotation
- token renewal/agent integration
- TLS/PKI for the bundled OpenBao listener
- KMS/HSM/transit auto-unseal profiles
