# Authentication and API error foundations

BaseHarbor keeps authentication, authorization and transport concerns separate.

## Authentication boundary

The `internal/auth` package defines provider-neutral OIDC/JWT requirements without coupling BaseHarbor to a specific identity provider or HTTP framework.

Current guarantees:

- OIDC issuers must use HTTPS.
- At least one non-empty audience must be configured.
- Authorization headers accept only the Bearer scheme.
- Bearer parsing errors never include token material.
- JWT verification is represented by a small `Verifier` interface.
- Verifier implementations must validate signature, issuer, audience, expiry and not-before claims before producing an `identity.Principal`.

A concrete OIDC discovery/JWKS implementation will be added behind this boundary rather than leaking provider-specific concepts into the core.

## API error boundary

The `internal/apierror` package provides stable machine-readable error codes and safe client-facing messages.

Internal causes can be wrapped for logs and diagnostics, but are explicitly excluded from JSON serialization. This prevents database errors, secret material and provider-specific details from leaking through APIs.

Unknown error codes fall back to HTTP 500 with the generic message `internal error`.

## Security principles

1. Authentication configuration fails closed.
2. Tokens are never included in public errors.
3. Internal causes are never serialized to clients.
4. Authentication verifies identity only; authorization remains a separate concern.
5. Identity-provider-specific behavior stays behind interfaces.
