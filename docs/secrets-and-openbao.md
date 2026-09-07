# Secrets and OpenBao

BaseHarbor keeps secret handling behind the provider-neutral `credential.Broker` interface. Applications and higher-level platform modules work only with opaque credential references and never with OpenBao-specific types or paths.

## Security model

- OpenBao connections require HTTPS.
- Authentication tokens are configuration-only secret material and are excluded from JSON serialization.
- Credential references are resolved through a KV v2 mount.
- Secret payloads are returned through `credential.Data`, whose payload is excluded from JSON serialization.
- OpenBao response bodies and authentication tokens are never included in returned errors.
- 403 and 404 responses are mapped to stable, non-sensitive BaseHarbor adapter errors.
- The caller-provided scope is not automatically interpolated into an OpenBao path. Authorization and tenant isolation must remain explicit rather than being inferred from string concatenation.

## Boundary

The OpenBao adapter lives below the generic credential contract:

```text
application/module
      |
      v
credential.Broker
      |
      +-- NoopBroker
      +-- openbao.Client
```

This allows BaseHarbor to add other secret backends later without changing application-facing contracts.

## Current scope

The first adapter supports read-only OpenBao KV v2 secret resolution. Secret creation, rotation, leases, dynamic credentials, AppRole/OIDC authentication, token renewal and lifecycle management are intentionally separate future capabilities.
