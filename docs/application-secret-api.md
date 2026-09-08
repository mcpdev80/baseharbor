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

## Current exposure status

This change defines and tests the HTTP handler contract but does **not** start a public HTTP listener.

BaseHarbor does not yet have an authoritative persisted Application-to-Tenant ownership relation that can safely back `OwnershipResolver`. Wiring a public server before that relation exists would weaken the tenant isolation model, so the handler remains unexposed until that ownership boundary is implemented and tested.

This is intentional, not a missing authentication shortcut.

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
