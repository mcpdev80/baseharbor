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
  realization metadata. A Perses surface that is not selected (or is declared
  with an invalid or unavailable pin) MUST report `not_configured`
  (fail-closed); it is never `configured_unverified` or `verified`.
- Authentication is opt-in; when enabled, honor the upstream mandatory
  encryption-key requirement. Do not enable external notifications/routing by
  default.
- Never false-declare metrics/logs/traces application capabilities and never
  replace the Prometheus management UI silently.

The public status (`baha status --json`, `baha app status --json`) reports the
Perses management surface through a machine-readable `perses` field holding
EXACTLY one of three distinguishable states:

- `not_configured`: the manifest does not select a working Perses surface —
  either no `perses` selection at all, or a selection with an invalid or
  unavailable pin. This is the fail-closed default, never "configured".
- `configured_unverified`: the surface is selected and its pinned release
  validated against the compiled default pin, but this Core defines the seam
  only and ships no Perses health probe, so reachability stays unverified.
  This is the only reachable state for a selected surface: selection plus a
  valid pin but no real health-path observation is always reported here.
- `verified`: only when a real Perses health probe observes the surface. Pin
  validation alone MUST NOT emit this state.

A configured state additionally requires that the effective security posture
validates, not only that a pin exists. BaseHarbor reports `not_configured` when
the surface would be reachable without authentication, when authentication is
enabled without the encryption key the pinned release requires, or when the
datasource proxy is left unrestricted. Publishing such a surface would turn an
optional dashboard into an unauthenticated proxy into the Target network.

### Transport and authorization

Perses binds a loopback address that BaseHarbor never publishes. Operators reach
it through the managed service-access gateway, which enforces TLS 1.2 or later
and either a verified client certificate or a bearer token. Credential material
is referenced by file path and never inlined into a rendered manifest, a
command line or a status record.

The gateway refuses the datasource-proxy route prefix. Panels reach
datasources through the ordinary query routes; the raw proxy is not an
operator-facing surface, and BaseHarbor never converts a management credential
into an upstream datasource credential.

### Datasource proxy destinations

The pinned release denies loopback, "this network", link-local, cloud metadata
and credential endpoints, multicast and reserved addresses by default. Those
lists are site-specific in part, so BaseHarbor adds the Target knowledge the
product cannot have: the deployment's own operator networks and, on a Kubernetes
Target, the API service address. Where a private-network allow and a default
deny overlap, the most specific prefix wins, and an allow at equal length wins —
the same precedence the upstream server applies, so the gateway and Perses can
never disagree about a destination.

A hostname is never cleared by name. Datasource destinations are re-verified at
connection time against every address the name resolves to, including the
addresses reached through redirects.

### Environment policy for this surface

The gateway authentication requirement follows the managed-environment rule in
this specification: production and staging require an authenticated surface, and
a development Target may reuse the single Target-scoped developer credential.
The Perses process itself must be authenticated in every environment, because
its own default would expose the API and the proxy without credentials.

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
