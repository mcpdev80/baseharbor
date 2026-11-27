# Run the Core operator API

The Core operator API is the authenticated HTTPS surface that `baha serve` starts
on the machine that owns a BaseHarbor Core. Workstations and Console reach it
over TLS instead of SSH. This page states which process serves which routes,
which `BASEHARBOR_*` variables the operator API requires and which are optional,
and how to point a client at the result.

The conceptual contract and the full route semantics stay in
[Control-plane runtime](../spec/control-plane-runtime.md) and
[Protected Machine HTTP v1](../spec/machine-http-v1.md). This is the startup
runbook; it documents the manually verified paths and marks the rest.

## Which process serves the operator API

`baha serve` is a single command with three mutually distinct modes, selected by
environment variables. Only the first two are the operator API.

- Operator control plane (this runbook): `BASEHARBOR_RUNTIME_APP_NAME` unset.
- Per-application runtime broker: `BASEHARBOR_RUNTIME_APP_NAME` set. This is a
  different, app-scoped surface and cannot expose the operator API.
- Connectivity relay / runtime executor: `BASEHARBOR_CONNECTIVITY_RELAY_MODE` or
  `BASEHARBOR_RUNTIME_EXECUTOR_MODE` set to `true`.

The operator HTTP API is enabled when `BASEHARBOR_API_OIDC_ISSUER` or at least
one `BASEHARBOR_API_OIDC_AUDIENCES` entry is configured. Without them,
`baha serve` starts only the runtime surface; requests under `/api/` are not
served at all.

## Prerequisites

The server fails closed before it opens the listener when any requirement is
missing. Run these on the Core host.

1. `baha up` has finished and `baha status` reports a READY Core (PostgreSQL,
   OpenBao and Keycloak managed by this installation).
2. The first-owner onboarding ran: `baha login -e dev --init-tenant` (see
   [authentication](../explanation/authentication.md)). This establishes the
   operator OIDC issuer/client the API later verifies against.
3. The Core host's real, non-loopback address is known to DNS or to the
   clients, and the managed Core CA can be transferred to them.
4. A serving keypair whose SANs cover that address, or the opt-in managed
   issuance below. Certificate issuance and renewal are out of scope here.

## Required environment

For a plain operator API with your own (BYOC) certificate material:

```text
BASEHARBOR_API_TLS_CERT_FILE
BASEHARBOR_API_TLS_KEY_FILE
BASEHARBOR_API_OIDC_ISSUER
BASEHARBOR_API_OIDC_AUDIENCES
```

`BASEHARBOR_API_DATABASE_URL` is additionally required whenever the operator
API is enabled. The DSN must reach the Core database; `baha serve` verifies the
migration schema against the shipped migrations on every start and opens the
listener only when state and schema agree. The machine API requires a durable
operations directory (`BASEHARBOR_RUNTIME_OPERATIONS_DIR`). The managed-Core
defaults below derive both values from the installation.

`BASEHARBOR_API_OIDC_AUDIENCES` is a comma-separated list; the audience must
include the operator OIDC client the Core's own onboarding established, and the
issuer must match the verified operator realm, or startup is refused.

## Managed Core: minimal start

For an installation managed by `baha` on this host, the defaults are derived
from the READY Core state; no certificate variables and no issuer repetition
are needed:

```bash
BASEHARBOR_WORKSTATION_ENROLLMENT_ENABLED=true baha serve
```

`baha serve` then selects the owned Core Target as the enrollment authority,
binds the operator OIDC issuer and audience from the verified realm, enables
workstation enrollment, defaults the listener to the host's LAN address on port
`18443` and sets the machine operations directory under the Target state root.
An explicit `BASEHARBOR_API_LISTEN_ADDR` keeps its authority. Set
`BASEHARBOR_MANAGED_CORE_TLS=false` to disable only this defaulting.

To let the existing OpenBao service issuer produce the serving leaves instead of
BYOC files, set all of these; loopback or wildcard listeners and mismatched
advertised ports fail closed, and externally configured certificate material is
never overridden:

```bash
BASEHARBOR_MANAGED_CORE_TLS=true \
BASEHARBOR_API_LISTEN_ADDR=<lan-ip>:18443 \
BASEHARBOR_API_PUBLIC_URL=https://<lan-ip>:18443 \
BASEHARBOR_WORKSTATION_ENROLLMENT_ENABLED=true \
baha serve
```

The Core verifies its own OIDC issuer through the managed trust resolution that
`baha login` uses: `BASEHARBOR_OPERATOR_OIDC_CA_FILE` first, then the managed CA
the Core bootstrap wrote, then the system pool. The managed CA is additive;
TLS verification is never disabled.

## Routes

The server binds TLS 1.2+ only, listens by default on `127.0.0.1:8443` (override
with `BASEHARBOR_API_LISTEN_ADDR`), and serves:

| Route group | Condition | Purpose |
| --- | --- | --- |
| `GET /healthz` | always | process liveness, unauthenticated |
| `GET /readyz` | broker mode | dependency readiness, unauthenticated |
| `GET /api/v1/management/installation` | operator API | installation identity and tenant after operator authentication |
| `GET /api/v1/machine/discovery` | operator API | operations plus canonical `http` endpoint bindings |
| `POST /api/v1/machine/executions`, `GET /api/v1/machine/executions/{execution_id}` | operator API | admit an execution (`202` is admission only) and read it back |
| `GET /api/v1/machine/executions/{execution_id}/events` (SSE), `POST /api/v1/machine/streams/logs`, `POST /api/v1/machine/streams/exec` | operator API, capability-gated | execution events, log and exec streams |
| `POST/GET/DELETE /api/v1/machine/terminals…` | operator API | interactive terminal streams |
| `GET/POST/PUT /api/v1/machine/source-transfers…` | operator API | bounded source-transfer upload |
| `GET /api/v1/apps/…` | operator API | protected application-secret API |
| `POST /api/v1/connectors/enroll` | `BASEHARBOR_CONNECTOR_ENROLLMENT_ENABLED=true` | one-use connector bootstrap exchange, only anonymous machine-adjacent endpoint |
| `POST /api/v1/connectors/authorizations`, `/owner-context` (GET), `/renewal-authorizations`, `/status`, `/disconnect-plan`, `/disconnect` | connector enrollment on | Node Connector grants and lifecycle behind operator authentication |
| `POST /api/v1/workstations/enroll` | `BASEHARBOR_WORKSTATION_ENROLLMENT_ENABLED=true` | redeems one already-minted invitation, once |
| `POST /api/v1/workstations/bootstrap`, `POST /api/v1/workstations/renew` | workstation enrollment on | grant creation and mTLS device renewal |
| `GET /metrics` | runtime executor | OpenMetrics scrape endpoint for the executor's runtime resource manager |

The runtime executor exposes its own operator HTTP surface on `0.0.0.0:9443`
(override with `BASEHARBOR_RUNTIME_EXECUTOR_LISTEN_ADDR`), not on the Core operator
`127.0.0.1:8443` listener. Its `GET /metrics` serves OpenMetrics from the executor
observer behind the same TLS 1.2+ and client-certificate authentication as the
executor's `/internal/` and `/healthz` routes; a scrape must present a verified
executor leaf identity. This is a provider-internal operator endpoint, separate
from the Core operator API route table above.

Every `/api/` request passes TLS, bearer-OIDC verification, membership
resolution, tenant context, RBAC and ownership checks; plaintext HTTP never
reaches a handler and no downgrade happens. Enrolled workstation devices authenticate with
their mTLS leaf instead of an operator token. Connector outbound sessions are a
separate mTLS listener (`BASEHARBOR_CONNECTOR_LISTEN_ADDR` plus
`BASEHARBOR_CONNECTOR_TLS_{CERT,KEY,CA}_FILE`, Core SPIFFE leaf required);
they are not part of the operator HTTP surface.

## Point the CLI at it

On the developer workstation, redeem an invitation (see
[remote workstation enrollment](remote-workstation.md)), or register the
Core-owned remote Target and connection profile as described in
[Target commands](../cli/targets.md#remote-core-owned-dockerpodman):

```bash
baha target create vm-core --runtime-provider docker \
  --access vm-core-management --access-provider baseharbor-core \
  --reference vm-core --tenant-id <tenant-uuid>
BASEHARBOR_CONNECTION_PROFILE=<profile> baha use vm-core
baha --target vm-core status
```

`BASEHARBOR_CONNECTION_PROFILE` pins the HTTPS CA and the operator session must
be valid before `baha use`. The Core stays the authority for source admission,
consent and lifecycle.

## Verification and limits

The route and configuration claims above come from the code on `develop` and
were exercised against a real `baha serve` process twice: once with an on-host
PostgreSQL and a local HTTPS OIDC stub, once with PostgreSQL and the OIDC
provider in containers on a Docker network. Both used a self-signed leaf, the
shipped migration state and `BASEHARBOR_RUNTIME_OPERATIONS_DIR` set.

| Request | Result |
| --- | --- |
| `GET https://<addr>/healthz` | 200 `{"status":"ok"}` |
| `GET https://<addr>/readyz` | 200 `{"status":"ready"}` |
| `GET https://<addr>/api/v1/machine/discovery` | 401 `{"code":"unauthorized","message":"authentication required"}` |
| `GET https://<addr>/api/v1/management/installation` | 401, same envelope |
| `GET https://<addr>/api/v1/<unknown>` | 401, authentication before routing |
| `GET http://<addr>/healthz` | 400; no plaintext path exists |

A missing certificate/key pair, a missing database DSN and an unreachable
issuer stop the process before any listener. The schema check is exact: a
database that records anything other than the shipped migration list is refused
with `database has unexpected migration` or `database schema is not ready`, as
quoted in the sections above. An issuer chain unknown to the host resolves
through `BASEHARBOR_OPERATOR_OIDC_CA_FILE`, which both runs used. The startup on
the Core host remains:

```bash
BASEHARBOR_WORKSTATION_ENROLLMENT_ENABLED=true baha serve
```

The `baha up` and `baha onboard` invitation path remains the documented
workstation attachment; the OIDC/PostgreSQL configuration here is its server
side, not an extra step.

The native CI acceptance runs on both Docker and Podman. It starts an isolated
Core, issues a private invitation, imports it into fresh workstation state,
verifies authenticated remote CLI operations over the enrolled device
connection, and removes the test workload.
The runbook, workstation enrollment, and Core-side invocation are therefore
checked together on the exact CI source revision; this flow does not require a
separate physical-VM smoke test. Node Connector outbound sessions and the
Console browser flow are separate surfaces and remain outside this acceptance.

## See also

- [Control-plane runtime specification](../spec/control-plane-runtime.md)
- [Protected Machine HTTP v1](../spec/machine-http-v1.md)
- [Authentication and API errors](../reference/authentication-and-api-errors.md)
- [Automate supported operations](machine-operations.md)
