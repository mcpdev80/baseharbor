# Management surface access v1

## Scope

This specification defines authentication, authorization mapping and environment policy for BaseHarbor-managed human-facing infrastructure and management surfaces.

It does not define application business users or application business RBAC.

## Authentication integration classes

Every managed human-facing surface MUST declare exactly one effective class:

```text
native-oidc
standards-auth-adapter
native-credential
unsupported
```

Resolution order is:

```text
native OIDC/OAuth2
    -> existing standards-based auth adapter/proxy
    -> provider-native credential
    -> unsupported
```

BaseHarbor MUST NOT introduce a proprietary authentication proxy merely to make a management GUI fit this model.

A surface that cannot be safely integrated MUST be reported as unsupported rather than silently creating an unrelated human-user source.

## Optional Perses management surface

Perses is an OPTIONAL, provider-owned management/visualization surface. It is
never an application capability and never a required BaseHarbor Core bundle; it
is a realization of the existing optional observability `management-ui` surface
that the backend data owners (Prometheus, and Loki/Tempo where the pinned
Perses release actually ships and supports their plugin) already declare.

- Pin the Perses release (product version + production image/digest) as
  realization metadata; a selected but unprovisioned Perses surface MUST be
  reported as `unsupported` (fail-closed "not configured"), never as configured.
- Authentication is opt-in; when enabled, honor the upstream mandatory
  encryption-key requirement. Do not enable external notifications/routing by
  default.
- Never false-declare metrics/logs/traces application capabilities and never
  replace the Prometheus management UI silently.

## Management roles

The only provider-neutral BaseHarbor management-role profile is:

```text
admin
edit
view
audit
```

These roles apply only to BaseHarbor-managed infrastructure and management surfaces.

Provider adapters map them to native roles/policies where safe.

Each mapping is classified as exact, limited or unsupported. BaseHarbor MUST NOT claim finer authorization than the provider can actually enforce.

Application business users, groups, roles and permissions remain application/IdP-owned.

## Environment policy

### Development

- Native OIDC/OAuth2 is preferred.
- Where OIDC is unavailable, one Target-scoped developer-facing Class A credential is the default across compatible managed surfaces when technically safe.
- Internal machine credentials remain isolated.

### Test and production

- Central managed Identity/OIDC is preferred.
- Where OIDC is unavailable, isolated per-surface credentials are the default.
- An explicit supported shared credential may be selected only when provider authorization and isolation semantics remain correct.
- Internal machine credentials remain isolated.

## Surface acceptance

Every shipped managed management surface records and verifies:

- HTTPS/trust behavior;
- authentication integration class;
- Identity integration or fallback;
- management-role mapping;
- credential ownership/source;
- rotation/revocation behavior;
- secret-safe diagnostics;
- HA continuity classification where applicable.

The machine representation MUST expose the same effective authentication class and role mapping as human CLI/status output.
