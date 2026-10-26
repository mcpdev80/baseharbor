# ADR 0003: Managed runtime API lifecycle

## Status

Proposed for the next MVP operations slice.

## Context

Applications using BaseHarbor-managed dynamic secret references need the `/runtime/v1/...` API at runtime. Requiring an operator to keep `baha serve` running manually would make application availability depend on a foreground CLI process and would violate the BaseHarbor MVP contract.

## Decision

The runtime API is a BaseHarbor control-plane service managed by the same lifecycle as the bundled control-plane runtime.

- `baha up` starts the runtime API together with the BaseHarbor control-plane dependencies.
- Applications never start or supervise `baha serve` themselves.
- `baha app apply/up` only consume the stable runtime API endpoint and app-scoped runtime identity injected by BaseHarbor.
- Runtime API transport remains TLS-only.
- Runtime-only operation does not require human OIDC or the operator control-plane database API.
- Enabling the operator `/api/...` surface remains a separate explicit concern and still requires OIDC plus the control-plane database.
- The runtime API service receives only the BaseHarbor state and container-runtime access required to resolve application scopes; applications never receive manager credentials.
- No application SDK, `baha login`, or human token is introduced.

## Runtime contract

Applications continue to receive only standard process/file inputs:

```text
BASEHARBOR_RUNTIME_API_URL=https://...
BASEHARBOR_RUNTIME_TOKEN_FILE=/run/baseharbor/runtime/token
```

The token is mounted only into workload services that explicitly consume the runtime identity variables.

## Consequences

The next operations slice must provide a self-contained service artifact, TLS material lifecycle, health/readiness checks, stable endpoint discovery, and `baha up/down/status/doctor` coverage. A clean BaseHarbor installation must not depend on an operator keeping an interactive CLI process alive.
