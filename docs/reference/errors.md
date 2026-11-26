# Errors and outcomes

BaseHarbor uses typed outcomes so humans and agents can distinguish different failure classes.

The semantic result of a command maps onto a single stable process exit code. See [Process exit codes](cli.md#process-exit-codes) for the full documented set (`0` success, `1` runtime failure, `2` usage error, `130` SIGINT, `143` SIGTERM, plus the `baha app exec` child-status passthrough `1-255`).

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
