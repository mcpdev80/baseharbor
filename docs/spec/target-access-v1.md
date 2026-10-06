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

## Connector enrollment implementation status

The scoped enrollment boundary and PostgreSQL one-use store are implemented;
see [ADR 0020](../decisions/0020-scoped-connector-enrollment.md). Signatures use
the existing protected OpenBao authority and client-owned CSR keys. Bootstrap
authorization binds tenant, Target, node, runtime and nonce and expires within
ten minutes. Certificate TTL is bounded to 24 hours. Consumption commits before
signing, so a failed or interrupted issuance requires a fresh authorization.

This boundary does not yet advertise a usable remote Target. The operator and
bootstrap endpoints, live node admission, renewal/revocation, outbound session binding
and real runtime qualification remain required. Explicit non-local access fails
closed while its execution adapter is unavailable; it never selects the local
runtime as a transport fallback.
