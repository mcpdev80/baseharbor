# Authentication, managed identity and operator access

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


## Managed application identity

Application-facing identity is separate from the control-plane tenant-resolution path above. When an application declares managed identity, BaseHarbor realizes standard OIDC/OAuth2 behavior through the selected provider and projects a standard application binding rather than a BaseHarbor authentication SDK.

The managed Keycloak reference provider creates an isolated application/environment identity scope and client, derives redirect/logout URIs from realized application exposure, configures portable authentication requirements and verifies the resulting provider state. External OIDC is supported as an authentication-only provider when provisioning is not available.

Applications consume standard issuer/discovery/JWKS/client metadata. When the issuer uses BaseHarbor-managed or explicitly configured private PKI, the identity binding also projects the trust bundle as a file. Compatibility environment variables include `OIDC_ISSUER`, `OIDC_CLIENT_ID`, `OIDC_SCOPES`, optional `OIDC_CLIENT_SECRET_FILE` and optional `OIDC_CA_FILE`. The same trust bundle is available in the Service Binding identity directory as `ca.crt`.

Applications are expected to use their normal OIDC/OAuth2 library and configure its HTTPS client with the supplied trust file when one is present. BaseHarbor does not replace the application's OIDC client library.

Provider administration credentials are never projected into the workload.

## Development management identity

Local development has a separate convenience boundary for browser-facing provider administration.

Each development Target owns one management account, default username `developer`, with a strong generated password. When managed application Identity is present, BaseHarbor reconciles that developer identity through managed OIDC as the central development identity. Provider UIs that cannot consume OIDC directly may receive the same Target-scoped credentials through a provider-native adapter.

This does not merge application-user identity, BaseHarbor operator identity and provider implementation credentials into one security domain. It is a local-development UX layer only:

- the credential is Target-scoped, not application-manifest state;
- normal status, doctor, plan, logs, audit and evidence never reveal the password;
- provider/runtime service credentials remain separate least-privilege credentials;
- test and prod never reuse the shared development credential.

Use `baha dev credentials` only when the local development secret must be explicitly revealed or rotated.

The same Target owns one development domain, default `baseharbor.localhost`. Browser-facing OIDC uses the canonical `https://<app>-identity.<domain>` issuer through the local development gateway. Workloads keep a separate internal issuer endpoint and trust projection, so a `.localhost` browser name is never incorrectly used as a container-local network address. Public/browser discovery and workload discovery describe the same realm/client contract while using the correct network endpoint for each side.

## Operator authentication

BaseHarbor operator identity is a third boundary and is not interchangeable with application-user identity.

- `dev` uses trusted-local operator mode and requires no login.
- `test` and `prod` require a valid OIDC operator session scoped to the effective Target and Environment.
- The CLI uses Authorization Code + PKCE and stores only short-lived owner-only local session state.
- Managed Keycloak and external OIDC can provide the operator boundary.
- A physical IdP may serve both application and operator authentication, but their clients, scopes and logical identity domains remain separate.

Use `baha login -e ENV`, `baha whoami -e ENV` and `baha logout -e ENV` for explicit session management.

Advanced BaseHarbor RBAC/JIT/approval/break-glass governance is separate from this minimum authenticated operator boundary.

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
