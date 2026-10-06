# Security Policy

BaseHarbor handles secrets, credentials, TLS material, runtime access and deployment state. Security reports are treated as private until a fix and disclosure plan are ready.

## Supported versions

BaseHarbor is currently pre-v1.

Security fixes are applied to the latest released version. Older releases may not receive backports unless a specific issue requires it.

## Reporting a vulnerability

Please do **not** open a public GitHub issue for a suspected security vulnerability.

Use GitHub's private vulnerability reporting / Security Advisory flow for this repository:

1. Open the repository's **Security** tab.
2. Choose **Report a vulnerability**.
3. Include enough detail to reproduce and assess the issue.

Useful information includes:

- affected BaseHarbor version or commit;
- affected runtime/provider (for example Docker or Podman);
- reproduction steps;
- expected and observed behavior;
- security impact;
- relevant logs or configuration with all secrets removed.

Do not include passwords, tokens, private keys, recovery material or other live credentials.

## What to expect

Security reports will be triaged privately. Confirmed issues will be handled through a coordinated fix and disclosure process appropriate to their impact.

## Scope

Security-sensitive areas include, among others:

- secret handling and OpenBao integration;
- TLS, PKI and trust boundaries;
- runtime and provider credentials;
- authorization and ownership checks;
- application-to-application connectivity;
- deployment state and backup material;
- CLI, JSON and MCP boundaries;
- container/runtime isolation and privilege handling.

For normal bugs, feature requests and documentation issues, use the public GitHub issue tracker.

## Session and stream bounds

The OIDC verifier validates issuer, audience, signature and expiry at each new
protected HTTP request. Tenant/operator policy is evaluated before execution.
Operator CLI sessions are Target/environment-bound and must have more than
30 seconds of remaining lifetime before persisted reuse.

Group or role changes at the identity provider do not imply instant revocation
of an already issued token. Their effect depends on token expiry and the
authoritative membership/policy resolver. There is no claimed fixed group
revocation interval in the current contract.

The machine HTTP projection bounds asynchronous execution to 30 minutes.
Event/log/exec streams end at the earlier of verified token expiry and five
minutes after admission. Request cancellation closes the producer stream, even
when it is idle. Each socket write has a maximum 30-second backpressure bound
clamped to the session deadline. A new connection requires fresh authentication
and policy evaluation. Browser requests with an Origin must match the protected
HTTPS Core authority; cookie-only requests are not bearer authentication.

This bounds one admitted stream, not token-wide instant group revocation. A
still-valid token may be accepted for another request until its expiry unless
the authoritative membership/policy resolver denies it. Already authorized
mutations have their own completion/deadline semantics.

Private vulnerability reporting was verified enabled through the repository's
GitHub API on 2026-10-06. The direct private reporting route is
https://github.com/mcpdev80/baseharbor/security/advisories/new .

Connector transport sessions require TLS 1.3 client-chain verification and the
exact active PostgreSQL certificate scope. A trusted CA alone does not admit a
node. Sessions expire after at most five minutes or earlier peer expiry/Core
shutdown. Each operation rechecks admission; ongoing sessions also recheck every
five seconds with a two-second registry deadline. Revocation or registry failure
retires the connection within that bound. Interrupted control operations are not
automatically replayed; reconcile observed state before issuing another mutation.
