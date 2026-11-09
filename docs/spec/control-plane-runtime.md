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

## Explicit LAN development gateway

A core-only installation can bootstrap the managed operator Keycloak realm
without first deploying an application. The shared Keycloak gateway route is
created by the first-owner onboarding path before canonical OIDC discovery.

By default the development gateway stays on `127.0.0.1`, and canonical
`*.baha.localhost` names are **not** valid from another workstation. For an
intentional LAN setup, the owner must:

1. Configure a real DNS domain for the **Core target** with
   `baha dev domain DOMAIN` before first-owner OIDC onboarding. In particular,
   `auth.DOMAIN` must resolve to the Core host from both hosts.
2. Set `BASEHARBOR_DEV_GATEWAY_BIND_ADDRESS` to the Core host's **actual
   non-loopback IPv4 address** for the `baha login --init-tenant` process (and
   subsequent gateway reconcile processes). An empty value retains the
   loopback-only default. Wildcard addresses and `*.localhost` routes fail
   closed for a LAN listener.
3. Trust the **actual** managed Core CA on the workstation through the
   documented operator trust / HTTPS connection-profile boundaries. Never
   disable TLS verification and never generate a fake operator session.
4. Start `baha serve` with a real CA-signed HTTPS API and connector keypair
   that both cover the advertised LAN host/IP in their SANs, and set the
   explicit API/connector listener and public addresses. `node add`
   consumes the resulting verified `node-authority-profile.json`.

The gateway's service certificate is issued by the existing OpenBao-backed
service PKI for the configured DNS route. The control-plane API and connector server normally use separately prepared
keypairs. For an explicitly managed LAN setup, setting
`BASEHARBOR_MANAGED_CORE_TLS=true`, `BASEHARBOR_API_PUBLIC_URL` and
`BASEHARBOR_CONNECTOR_PUBLIC_ADDRESS`, together with concrete matching
`BASEHARBOR_API_LISTEN_ADDR` and `BASEHARBOR_CONNECTOR_LISTEN_ADDR`, enables
`baha serve` to issue both serving certificates from the **existing READY**
OpenBao service issuer. This is a dev-only opt-in, not a new CA, implicit
exposure, ACME, generic PKI management or certificate renewal daemon.
The private serving material stays under the owned Core target's
`managed-serving/` state directory; certificate SANs match advertised hosts.
Any explicit BYOC serving certificate configuration remains authoritative. A DNS name and a gateway bind setting do
not automatically publish `baha serve` or authorize tenant/connector access.

## TLS scope

This runtime consumes an already prepared certificate/key pair. Certificate issuance, ACME, internal PKI, BYOC discovery, chain construction and renewal belong to the dedicated certificate-management milestone and are intentionally not implemented here.

The server requires TLS and uses TLS 1.2 or newer. A missing or invalid certificate/key pair prevents startup.

## Shutdown

The main `baha` process converts SIGINT and SIGTERM into context cancellation. `baha serve` then performs a bounded graceful HTTP shutdown before closing its PostgreSQL pool.

## Authority boundary (server vs local execution)

The control-plane server is a remote boundary. It addresses the selected
installation authority, which owns authoritative deployment, provider, identity,
consent-revision and mutation state for its admitted Targets.

Local direct execution is independent from the server being present or reachable:
it reuses the same semantic Core (`internal/coreinstallation`,
`internal/deployment`) without mandating the OIDC/PostgreSQL/operator boundary the
remote API requires. A remote client that cannot reach the selected installation
addressing through its management connection fails closed; it never substitutes
local execution for a missing foreign or expired remote authority, and a cached
last-known state is a stale `observed-at` observation, never current `READY`.

See [ADR 0021](../decisions/0021-one-installation-authority.md).
