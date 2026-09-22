# Authentication and tenant resolution

BaseHarbor authenticates control-plane HTTP requests through OpenID Connect and resolves each verified external identity to exactly one tenant scope before authorization or application ownership checks run.

## Request trust chain

```text
Authorization: Bearer <ID token>
        ↓
OIDC discovery + JWKS verification
        ↓
identity.Principal { issuer, subject, audience }
        ↓
identity-scoped membership resolution
        ↓
tenancy.Context { tenant, external identity, roles }
        ↓
RBAC + application ownership + protected handler
```

Every step fails closed. A request never reaches a protected handler when token verification, identity resolution or tenant resolution is missing, invalid or ambiguous.

## OIDC verification

`internal/auth.OIDCVerifier` uses `github.com/coreos/go-oidc/v3/oidc` rather than implementing JWT cryptography in BaseHarbor.

Startup discovery resolves the provider metadata and JWKS endpoint from the configured HTTPS issuer. Token verification delegates signature, issuer, expiry and audience/authorized-party validation to the OIDC library.

BaseHarbor supports multiple configured audiences by constructing one upstream verifier per accepted audience. A token must pass the upstream verifier for at least one configured audience.

Successful verification produces only the provider-neutral identity fields required by BaseHarbor:

- issuer
- subject
- audience

Raw bearer tokens are not included in BaseHarbor errors, API responses, logs or persisted identity state.

## Pre-tenant membership resolution

Tenant membership must be resolved before a `baseharbor.tenant_id` context exists. This is intentionally not implemented through a superuser connection or a PostgreSQL role with `BYPASSRLS`.

Migration `0004_identity_membership_resolution` adds a second row-level-security policy to `memberships` that is restricted to `FOR SELECT`.

The policy allows a row to be read only when its external identity matches the transaction-local, already verified claims:

```text
baseharbor.identity_issuer
baseharbor.identity_subject
```

`database.IdentityTenantResolver` opens a read-only transaction, sets those two values with transaction-local `set_config`, reads only the matching membership rows, and passes the result through `tenancy.Resolve`.

The resulting rules are:

- no matching membership -> `ErrNoMembership`
- memberships from one tenant -> one resolved tenant context
- multiple roles in the same tenant -> deduplicated resolved roles
- memberships spanning more than one tenant -> `ErrAmbiguousTenant`
- incomplete principal -> rejected before database lookup

The transaction-local identity claims disappear when the transaction ends and do not become persistent session state.

## RLS boundaries

The existing tenant policy and the identity-resolution policy serve different phases:

```text
pre-tenant request phase
    verified issuer + subject
    -> SELECT-only identity policy

post-resolution request phase
    resolved tenant_id
    -> normal tenant RLS policy
```

The identity policy does not grant INSERT, UPDATE or DELETE access. It is a narrow bootstrap path for determining tenant scope, not a second tenancy model.

## Network exposure

These authentication and tenant-resolution components complete the security dependencies required by the protected control-plane handler chain.

BaseHarbor still does not start a public control-plane network listener in this change. Listener address/TLS settings, OIDC runtime configuration, database wiring and startup verification must be assembled explicitly before network exposure is enabled.
