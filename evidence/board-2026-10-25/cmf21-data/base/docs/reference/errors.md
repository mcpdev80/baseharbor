# Errors and outcomes

BaseHarbor uses typed outcomes so humans and agents can distinguish different failure classes.

Important semantic outcomes include:

- unsupported;
- unverifiable;
- denied / not authorized;
- conflict;
- foreign ownership;
- degraded;
- failed verification.

Errors should be actionable and secret-safe.

Machine interfaces return structured error envelopes. Human CLI rendering may add context, but it must not change the underlying semantic result.
