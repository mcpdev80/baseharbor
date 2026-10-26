# Developer gateway effective port

## Signatures

```text
curl: (7) Failed to connect ... port 443
OIDC discovery unavailable
```

## Root cause

The effective development gateway port can fall back from the preferred port. Parts of the stack previously used the persisted dynamic port while listeners or tests still assumed fixed 443/8443 values.

## Fix

The persisted effective gateway port is the source of truth for:

- listener configuration,
- published port mapping,
- canonical development URLs,
- OIDC issuer access,
- management UI routes,
- demo acceptance checks.

The demo must fail if the gateway state is missing instead of guessing a port.
