# Target Access Provider Contract

Status: normative for the pre-v0.5 Target model.

## Hard boundary

```text
Runtime Provider != Target Access Provider
```

A Runtime Provider defines runtime-specific realization and observation semantics.
A Target Access Provider defines how BaseHarbor Core reaches a Target and
transports already-selected, already-authorized bounded operations.

Neither axis substitutes for the other.

## Target model

The existing Target model remains authoritative:

```text
Target
├── Runtime
│   └── provider
└── Access
    └── reference -> AccessDefinition
        ├── provider
        ├── reference
        └── native-context (optional)
```

The effective projection exposes:

```text
name
runtime_provider
access_provider
access_reference
scope
```

Runtime and Access providers are independent.

Valid examples include:

```text
runtime=docker      access.provider=local
runtime=podman      access.provider=local
runtime=docker      access.provider=baseharbor-node-connector
runtime=podman      access.provider=baseharbor-node-connector
runtime=kubernetes  access.provider=native-api
runtime=openshift   access.provider=native-api
```

## Responsibilities

A Target Access Provider may own:

- connection establishment;
- authenticated encrypted transport;
- endpoint and peer identity;
- structured capability discovery;
- bounded request/response transport;
- log/event stream transport;
- bounded resource-scoped exec transport;
- native platform context references;
- connection health.

A Target Access Provider must not own:

- portable Application Intent;
- Application or Deployment desired state;
- Runtime Provider realization semantics;
- provider placement;
- environment policy;
- operator authorization policy;
- HA semantics;
- secret source-of-truth semantics;
- BaseHarbor reconciliation decisions.

## Core call direction

```text
CLI / Console / MCP / HTTP
          |
          v
     BaseHarbor Core
          |
          v
 Runtime semantics
          |
          v
 Target Access Provider
          |
          v
 concrete target
```

Clients never call a connector or other Access Provider directly.

## Local access

`access.provider=local` means BaseHarbor executes the already-selected Runtime
Provider operation on the local machine.

Local access is independent from the runtime:

```text
docker + local
podman + local
```

The legacy CLI `--provider` flag is only an alias for
`--runtime-provider`. It does not imply the Access Provider.

## BaseHarbor Node Connector

`access.provider=baseharbor-node-connector` is an optional remote Target Access
implementation for Docker/Podman/VM/bare-metal scenarios.

The connector:

- receives bounded typed operations selected by Core/Runtime Provider;
- does not receive portable Application Intent;
- does not own policy, placement or reconciliation;
- has no generic host-shell/runtime-command/workspace-command API;
- stages transferred deployment artifacts under an explicit protected staging
  root;
- uses mutually authenticated TLS with verified peer identity;
- negotiates structured capabilities after authentication.

The connector can evolve and release independently from BaseHarbor Core while
conforming to the versioned Target Access contract.

## Native API access

`access.provider=native-api` represents platform-native authenticated access
such as Kubernetes/OpenShift API contexts.

Kubernetes/OpenShift do not require the BaseHarbor Node Connector merely to
satisfy the Target Access model.

## Security

Every non-local Access Provider fails closed.

Required baseline:

- authenticated encrypted transport;
- no plaintext or opportunistic fallback;
- verified endpoint/peer identity;
- credentials referenced through existing credential/trust contracts;
- no credentials embedded in portable Application Intent;
- rotation/renewal without changing Target/Application identity;
- expired, revoked and not-yet-valid identities rejected;
- capability negotiation only after authentication;
- security-relevant connection failures remain auditable without credential
  leakage.

## Capability discovery

Access capabilities are structured and provider-neutral. Consumers must not
branch on provider names where capability discovery answers the question.

Examples:

```text
connect
stream
exec-transport
peer-identity
native-context
```

Only demonstrated BaseHarbor requirements enter the public contract.

## Compatibility

This decoupling is pre-freeze architecture. The historical invariant

```text
access.provider == runtime.provider
```

is not a compatibility promise and is removed rather than emulated.

Portable Application Intent remains unchanged.

## Canonical Connector wire artifact

The language-neutral transport records are defined in
[wire.schema.json](https://github.com/mcpdev80/baseharbor/blob/HEAD/contracts/targetaccess/v1/wire.schema.json), with
[synthetic golden records](https://github.com/mcpdev80/baseharbor/blob/HEAD/contracts/targetaccess/v1/wire.golden.json) and
[session semantics](https://github.com/mcpdev80/baseharbor/blob/HEAD/contracts/targetaccess/README.md).
Core packages and resolves them offline. Consumers acquire an immutable Core
commit, reject duplicate keys and unknown fields, and verify the same fixtures.

Requests carry the Core execution correlation and an absolute UTC deadline.
Typed payloads do not admit a generic host command. Terminal admission cannot
resume; binary chunks, argv and staged artifact payloads have explicit bounds.
A schema match establishes structure only. Exact authenticated peer/scope,
durable side-effect admission, cancellation and filesystem confinement must
also be enforced by the live session and runtime implementation.

## Connector enrollment implementation status

The scoped enrollment boundary and PostgreSQL one-use store are implemented;
see [ADR 0020](../decisions/0020-scoped-connector-enrollment.md). Signatures use
the existing protected OpenBao authority and client-owned CSR keys. Bootstrap
authorization binds tenant, Target, node, runtime and nonce and expires within
ten minutes. Certificate TTL is bounded to 24 hours. Consumption commits before
signing, so a failed or interrupted issuance requires a fresh authorization.

The operator and bootstrap HTTPS handlers use the canonical wire records and
one-use Authority. Grant creation requires an authenticated Editor membership,
Core-owned Target configuration and effective Target policy. The bootstrap
exchange accepts only its scoped bearer authorization and client-owned CSR;
it does not require operator OIDC or return a private key.

Enrollment is explicitly enabled on the operator Core with
`BASEHARBOR_CONNECTOR_ENROLLMENT_ENABLED=true` and
`BASEHARBOR_CONNECTOR_AUTHORITY_TARGET=<local-core-target>`. Startup requires
operator OIDC, PostgreSQL and the initialized/unsealed protected OpenBao
authority. The signing Target is fixed at startup and must use canonical local
access with Docker or Podman. A request cannot choose another signing authority.

| HTTPS endpoint | Authorization and result |
| --- | --- |
| `POST /api/v1/connectors/authorizations` | Operator OIDC and resolved tenant `create` permission; returns only the protected `token`, `nonce`, `expires_at` projection. |
| `POST /api/v1/connectors/enroll` | One scoped `Authorization: Bearer` credential plus canonical enrollment request; returns the signed client certificate and public trust bundle. |

Grant input is `target_id`, `node_id`, `environment`, `lifetime_seconds`
(1–600) and `certificate_ttl_seconds` (1–86400). The trusted Core Target
configuration supplies `tenant-id` and runtime; the request supplies neither.
Its access definition uses `baseharbor-node-connector` and a stable node-id
`reference` matching the requested node. Unbound Targets, foreign tenants,
unsupported runtimes and policy denials fail closed. Both endpoints require
HTTPS, bounded whole-body JSON and same-origin browser requests, and use
`Cache-Control: no-store`. Tokens belong in the owner-only bootstrap
authorization file, never ordinary execution metadata or shell arguments.

## Outbound session admission

The operator Core may enable its separate outbound-session listener with
`BASEHARBOR_CONNECTOR_LISTEN_ADDR`, `BASEHARBOR_CONNECTOR_TLS_CERT_FILE`,
`BASEHARBOR_CONNECTOR_TLS_KEY_FILE` and `BASEHARBOR_CONNECTOR_TLS_CA_FILE`.
Enrollment, operator OIDC and PostgreSQL remain required. The server certificate
has one Core SPIFFE URI and server-auth usage; Connector certificates have
client-auth usage and the exact persisted tenant/Target/node/runtime identity.
TLS 1.3 verifies the chain before canonical Hello and active certificate admission.

The pool admits at most 64 connections and four sessions per enrolled scope.
Live capabilities are requested from the authenticated peer and bound to its
exact Hello identity. Static access descriptors do not prove live support.
Runtime Explorer projects inventory and bounded container operations through
that transport; existing Core ownership and tenant decisions remain authoritative.
Remote lifecycle realization, metrics and live runtime qualification still
need complete adapters and exact-source runtime qualification.

Follow logs and interactive terminals exclusively consume one admitted session.
The canonical stream open and each event carry the exact stream ID and Core
correlation. Output and terminal input have independent contiguous sequences;
foreign scope, gaps and malformed frames retire the connection. Output blocks
are bounded to 16 KiB and use reader backpressure. Terminal argv, size and input
are typed; ownership and environment are checked by Core before stream open.
Exit codes are propagated. Slow readers, token/session expiry and disconnect
close the transport; an interactive process is never transparently reconnected.
These source-level guarantees still require real Docker/rootless Podman and
authenticated browser qualification at the final candidate.

One control operation executes per connection. Every invocation rechecks
certificate admission and preserves Core execution correlation. No operation is
automatically replayed after cancellation, disconnect or ambiguous completion.
The interrupted transport is retired; callers reconcile observed state before a
new mutation. A missing or foreign session never selects a local runtime.

Sessions expire at the earlier of five minutes, peer certificate expiry and
Core lifetime. Persisted admission is rechecked every five seconds with a
two-second lookup deadline, including idle and active sessions. Revocation or
registry unavailability closes the transport within that check bound. New
handshakes reload server identity material and node CA trust, supporting a
controlled overlap bundle; changing the selected Core identity requires restart.
Real OpenBao/rotation/runtime and authenticated private evidence remain required;
these source primitives are not a v0.4.23 release approval.
