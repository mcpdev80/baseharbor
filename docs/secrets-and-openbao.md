# Secrets and OpenBao

BaseHarbor keeps secret consumption behind the provider-neutral `credential.Broker` interface. Applications and higher-level platform modules work with opaque credential references rather than OpenBao-specific implementation types.

The bundled single-node control-plane OpenBao now also has an explicit operator bootstrap/unseal lifecycle through `baha`. This lifecycle is separate from the network credential adapter: bootstrap commands execute through the local container-runtime boundary, while the credential adapter continues to require HTTPS.

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

The AppRole receives short-lived service tokens and is restricted to application secret data below the managed `baseharbor/` KV v2 mount. It is not a replacement root identity and does not receive the root policy.

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

The platform bootstrap lifecycle is a separate operator boundary:

```text
baha openbao ...
      |
      v
platform OpenBao lifecycle
      |
      v
local Compose/OpenBao runtime
```

This separation prevents bootstrap privileges from leaking into normal application secret-resolution paths.

## Current scope

Implemented:

- read-only KV v2 secret resolution through the HTTPS-only credential adapter
- bundled OpenBao state inspection
- explicit single-node bootstrap
- explicit manual unseal
- `baseharbor/` KV v2 mount creation
- restricted manager AppRole provisioning
- initial root-token revocation
- owner-only recovery and manager credential files
- real manager authentication/KV verification

Not yet implemented:

- application `services.secrets` convergence
- application-specific OpenBao policies/AppRoles
- moving PostgreSQL/Valkey credentials out of local runtime files into OpenBao
- dynamic PostgreSQL credentials
- credential rotation
- token renewal/agent integration
- TLS/PKI for the bundled OpenBao listener
- KMS/HSM/transit auto-unseal profiles

Until application-secret convergence is implemented, manifests with `services.secrets.enabled: true` continue to fail closed.
