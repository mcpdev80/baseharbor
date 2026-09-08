# Application secret service and API boundary

BaseHarbor application secret operations have one trusted orchestration boundary: `internal/applicationsecret.Service`.

The `baha app secret` commands and the HTTP application-secret handler use that service instead of implementing independent OpenBao workflows. This keeps validation, runtime resolution, provider access, timeouts and secret metadata semantics in one place.

## HTTP contract

The current handler defines these BaseHarbor-specific operations:

```text
GET    /api/v1/apps/{app}/secrets
PUT    /api/v1/apps/{app}/secrets/{name}
DELETE /api/v1/apps/{app}/secrets/{name}
```

`GET` returns metadata only:

```json
{
  "secrets": [
    {
      "name": "OPENAI_API_KEY",
      "required": true,
      "present": true,
      "usable": true
    }
  ]
}
```

There is deliberately no API operation that reveals a stored secret value.

`PUT` accepts a new or replacement value:

```json
{
  "value": "..."
}
```

A successful response confirms only the key name and configured state. The submitted value is not echoed.

`DELETE` permanently removes the managed secret through the same application-secret service used by the CLI.

## Authorization boundary

The HTTP handler is fail-closed and requires all of the following before a secret operation can reach the service:

1. an authenticated `identity.Principal` in the request context
2. a resolved `tenancy.Context`
3. an RBAC role that grants the requested read/update/delete permission
4. positive application ownership proof from an `OwnershipResolver`

Missing, ambiguous or negative ownership never falls back to application-name access.

Knowing an application name is therefore not sufficient authorization to inspect or mutate its secret metadata.

## Authoritative application ownership

Application-to-Tenant ownership is persisted in PostgreSQL in `application_ownerships`.

The invariant is intentionally small and strict:

```text
application_name  -> exactly one tenant_id
```

`application_name` is the primary key and `tenant_id` references `tenants(id)`. PostgreSQL row-level security scopes reads and writes to the current BaseHarbor tenant context.

`ApplicationOwnershipStore.Claim` is idempotent for the same tenant and application. A different tenant cannot claim an application name that is already owned. Ownership is never reassigned implicitly or with last-write-wins behavior.

`ApplicationOwnershipStore.OwnedByTenant` performs the read inside the existing tenant-scoped transaction boundary, so the HTTP handler can use the database-backed store directly as its `OwnershipResolver`.

## Protected request chain

Protected control-plane handlers are composed behind one mandatory request-security chain:

```text
Authorization: Bearer <token>
        ↓
OIDC discovery / JWKS verification
        ↓
identity.Principal
        ↓
identity-scoped TenantResolver
        ↓
tenancy.Context
        ↓
RBAC
        ↓
ApplicationOwnershipStore
        ↓
application secret handler
```

`auth.OIDCVerifier` uses the established `go-oidc` implementation for signature, issuer, expiry and audience validation rather than implementing JWT cryptography inside BaseHarbor.

`database.IdentityTenantResolver` resolves memberships before a tenant context exists through a dedicated PostgreSQL `FOR SELECT` RLS policy keyed by the already verified issuer and subject. It does not use a superuser connection or a role with `BYPASSRLS`.

The resolver fails closed when there is no membership, an invalid principal, or memberships spanning more than one tenant. Multiple roles inside the same tenant are resolved by the existing `tenancy.Resolve` rules.

See [authentication.md](authentication.md) for the complete trust-boundary description.

## Current exposure status

The protected handler, concrete OIDC verifier, database-backed application ownership and safe pre-tenant tenant resolver are implemented and testable.

BaseHarbor still does **not** claim that this API is publicly listening. Runtime OIDC settings, database wiring, listener/TLS configuration and startup verification still need to be assembled explicitly before opening the control-plane network endpoint.

## Secret non-disclosure

Secret values must never appear in:

- GET responses
- successful PUT responses
- API errors
- application metadata
- normal logs or telemetry
- audit payloads

The API tests include negative authorization cases and verify that a submitted secret value is not echoed in the response.

## Runtime delivery remains separate

This API manages secret values. It does not define how a workload consumes them.

Runtime delivery remains a provider concern and can later use standard mechanisms such as:

- in-memory secret files
- explicit environment injection where appropriate
- OpenBao/Vault-compatible workload identity
- Kubernetes-native secret projection

The application contract continues to declare **what** is required, not a mandatory BaseHarbor-specific delivery mechanism.
