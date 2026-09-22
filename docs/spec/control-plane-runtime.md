# Control-plane runtime

BaseHarbor exposes its protected control-plane API through the `baha serve` process. The server is deliberately separate from application service protocols: PostgreSQL, Valkey, OpenBao and future S3-compatible services remain directly consumable through their native interfaces.

## Startup contract

`baha serve` fails closed before opening the listener unless all required runtime dependencies can be constructed:

- the control-plane PostgreSQL DSN is configured and reachable
- the configured OIDC issuer is HTTPS and discovery succeeds
- at least one OIDC audience is configured
- the TLS certificate and private key are both present and form a usable key pair
- authentication, tenant resolution, RBAC, application ownership and application-secret handlers can be composed

The server never falls back to an unauthenticated or plaintext API listener when one of these requirements is missing.

## Configuration

The current runtime configuration is explicit process configuration:

```text
BASEHARBOR_API_DATABASE_URL
BASEHARBOR_API_OIDC_ISSUER
BASEHARBOR_API_OIDC_AUDIENCES
BASEHARBOR_API_TLS_CERT_FILE
BASEHARBOR_API_TLS_KEY_FILE
BASEHARBOR_API_LISTEN_ADDR
```

`BASEHARBOR_API_OIDC_AUDIENCES` is a comma-separated list. `BASEHARBOR_API_LISTEN_ADDR` defaults to `127.0.0.1:8443` when omitted.

Secret-bearing database URLs must be injected through the runtime environment or another trusted process boundary and must not be committed to source control.

## HTTP boundaries

The runtime exposes:

```text
GET /healthz
/api/v1/...
```

`/healthz` is an unauthenticated process-liveness endpoint and does not expose configuration or dependency details.

Every `/api/` request passes through the mandatory security chain:

```text
TLS
  -> bearer token parsing
  -> OIDC discovery/JWKS-backed token verification
  -> identity-scoped membership resolution under PostgreSQL RLS
  -> tenant context
  -> RBAC
  -> authoritative application ownership
  -> protected application API
```

The current protected API surface contains the application-secret endpoints. Secret values are never exposed through a reveal endpoint.

## TLS scope

This runtime consumes an already prepared certificate/key pair. Certificate issuance, ACME, internal PKI, BYOC discovery, chain construction and renewal belong to the dedicated certificate-management milestone and are intentionally not implemented here.

The server requires TLS and uses TLS 1.2 or newer. A missing or invalid certificate/key pair prevents startup.

## Shutdown

The main `baha` process converts SIGINT and SIGTERM into context cancellation. `baha serve` then performs a bounded graceful HTTP shutdown before closing its PostgreSQL pool.
