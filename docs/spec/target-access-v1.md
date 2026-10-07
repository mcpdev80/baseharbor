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
| `POST /api/v1/connectors/renewal-authorizations` | Protected operator identity, tenant membership and update permission; creates a one-use authorization pinned to the node's current active certificate. |

Grant input is `target_id`, `node_id`, `environment`, `lifetime_seconds`
(1–600) and `certificate_ttl_seconds` (1–86400). The trusted Core Target
configuration supplies `tenant-id` and runtime; the request supplies neither.
Its access definition uses `baseharbor-node-connector` and a stable node-id
`reference` matching the requested node. Unbound Targets, foreign tenants,
unsupported runtimes and policy denials fail closed. These endpoints require
HTTPS, bounded whole-body JSON and same-origin browser requests, and use
`Cache-Control: no-store`. Tokens belong in the owner-only bootstrap
authorization file, never ordinary execution metadata or shell arguments.

Certificate renewal uses the same authorization input and canonical CSR exchange.
Initial enrollment never replaces an issued identity. The renewal store locks
the existing node and binds its grant to the current certificate serial. A
revoked, expired or concurrently replaced certificate invalidates the grant;
revocation during signing also prevents new certificate admission. Certificate
material is returned only after the replacement and one predecessor are committed
atomically. The predecessor remains admissible for at most five minutes and no
longer than its original expiry. A subsequent renewal retires any earlier overlap.
Revoking a predecessor preserves the current replacement; revoking the current
identity denies both. The renewal tables and guards require migration `0007`;
rolling it back preserves earlier enrollment/admission and fails schema readiness.
The source implementation still requires real PostgreSQL/OpenBao and live
Connector rotation evidence before release qualification.

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
The Core project realization adapter binds every staged project to the exact
live tenant/Target/node/runtime identity. A staging receipt must preserve the
bundle ID, each source digest and every relative filename under one confined,
immutable object directory. Staged bytes are copied before dispatch; caller
changes cannot alter a later Quadlet realization. Compose apply/repair/destroy
accept only files from that validated project; Podman uses its owned native
Quadlet lifecycle. Each operation rechecks negotiated live capabilities,
retains Core correlation and a bounded deadline, and performs one dispatch.
A disconnect, substituted receipt or missing runtime exit status fails closed;
there is no automatic mutation replay or local execution fallback.

Project service observations independently inspect every selected container and
verify its immutable ID and exact native project/service labels. Names and a
running state alone never prove application readiness. Backend verification
uses a bounded command in one unambiguously owned running service, preserving
the existing SQL/cache TLS checks and container-local credential references.
Missing exit status, changed ownership or ambiguous replicas deny execution;
remote diagnostics are not returned as error details. Bundle identifiers follow
the Node's bounded immutable publication contract.

Generated provider bind files retain their Core-approved permissions so native
unprivileged services can read TLS material. Environments remain owner-only;
readable bind files remain below owner-only staging/object/parent directories.
The Node's versioned immutable publication records both digests and file modes,
and revalidates the protected ancestors and exact modes before execution.
Published Quadlet apply may additionally select `project_directory`, bound to
one immutable `bundles/.object-<32 lowercase hex>` publication. The selected
unit's content must match its committed bytes. Explicit
`@BASEHARBOR_BUNDLE@/` references are permitted only for read-only file volumes
and owner-only container environment files; the Node resolves them beneath its
own bundle root. Core host paths and build instructions are not portable inputs.
Node-native storage/registry process configuration belongs to the Node.

The Core graph adapter validates all selected unit names before mutation,
publishes network/volume definitions before starting containers and removes
containers before resource definitions. Native resource name collisions require
an owned realization receipt and matching project/service labels before repair;
foreign resources are preserved. Ordinary removal keeps provider data volumes.
These graph/provider primitives do not qualify the complete Application engine.

Writable, privileged, substituted or changed modes fail closed. Protected Core
receipts bind restoration to these same source modes. Ordinary Compose destroy
retains data; volume removal requires an explicit Core-owned reset decision.

Operator API startup binds one local installation even when Connector enrollment
is disabled. Enrollment and outbound sessions use that same startup selection;
per-application runtime brokers do not become installation authorities.
Remote application prerequisites select that bound local installation authority,
independently of the execution node. The
request must retain its verified tenant/identity and owned Target. It cannot
select another Core through application preferences or bootstrap a second Core
on the node. Live SQL/Secrets/Identity readiness of the bound installation is
still required; a cached state does not establish readiness.
The startup-bound API also rejects an application preference for another local
installation before reading its Core state or attempting bootstrap. Standalone
local CLI selection remains independent; API requests cannot replace the bound
installation by selecting a different local Target.
Explicit Core bootstrap uses the same startup binding and refuses an execution
node before creating installation state. An enrolled node cannot become another
Core through the machine setup operation.

Protected Core deployment state can retain a versioned project receipt containing
only the exact node Scope, immutable directory, bundle ID and source commitments.
Restoration requires the original protected source bytes and live scope binding;
foreign, changed or incomplete state fails before dispatch. Restoration itself
does not stage another bundle or replay a mutation. It is not an operator input
and does not establish full Application reconciliation or backend readiness.

The managed backend file projection checks the exact Core-generated definition
before compiling a native project name. It copies the runtime environment and
referenced file mounts/secret files from one protected directory; unrelated
installation files are excluded. Modified definitions, symbolic links and
unconfined paths fail closed. This preparation preserves SQL/cache TLS material;
workload delivery, remote placement and the complete Application engine require
their separate integration proof.

This project adapter carries already authorized runtime decisions. It does not
replace Core Application planning, provider placement, secret authority or
persisted reconciliation. Complete remote Application integration and
exact-source native qualification remain required before release approval.

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
Core lifetime. Persisted admission and the current node CA bundle are rechecked
every five seconds with a two-second lookup deadline, including idle and active
sessions, and before every dispatch/stream open. The peer chain is verified
against freshly loaded trust; a cached successful handshake does not preserve
an authority removed from the overlap bundle. Revocation, trust reload failure,
retired CA or registry unavailability closes the transport within that bound. New
handshakes reload server identity material and node CA trust, supporting a
controlled overlap bundle; changing the selected Core identity requires restart.
Real OpenBao/rotation/runtime and authenticated private evidence remain required;
these source primitives are not a v0.4.23 release approval.

### Managed Core server signing

The managed OpenBao authority configures a separate `baseharbor-core` CSR role for
`spiffe://baseharbor/platform/core/*`. It enables server authentication only;
`baseharbor-nodes` remains client authentication only. The internal
`ServiceIssuer.SignCoreCSR` boundary checks the signed leaf against the submitted
Core key, exact URI identity, lifetime and sole server-auth usage. It returns
public certificate material and never a private key. Node enrollment cannot call
this boundary to obtain a Core identity.

Core server CSRs can additionally carry up to eight explicitly authorized,
canonical DNS names selected by the internal Core caller. The request's approved
names must match its signed CSR, and the returned certificate must preserve
exactly that set. This allows normal TLS server-name verification alongside the
Core URI identity. Wildcards, IP SANs, duplicates and additional names are denied;
node signing still refuses all DNS SANs and cannot grant server-name authority.

This provides an issuance boundary for Core-owned keys. It does not by itself
install or rotate the listener's configured files, coordinate trust overlap, or
qualify a production OpenBao rotation. Those deployment and end-to-end checks
remain required before pre-release approval.

Managed certificate revocation accepts the positive hexadecimal serial from an
X.509 leaf as well as the issuer's colon-separated form. Core normalizes it to
OpenBao's certificate storage key before revocation; malformed, zero or
oversized serials fail before authenticating to the issuer. A persisted node
revocation independently denies existing dispatch and fresh session admission.

## Source-bound Quadlet completion observation

`runtime.quadlet.verify-completion` is an authenticated, read-only capability for
Linux Podman with a live user systemd manager. Its payload requires `name`
(a `.container` unit), the exact unresolved `content`, and `project_directory`
(an immutable `bundles/.object-<hex>` publication). It cannot select host paths,
start units or supply an `enable` flag. Normal peer, Target and capability checks
apply; the observation does not admit or replay a mutation.

The Node verifies the published bundle, current unit and native resource ownership,
resolved source digest, protected activation receipt and current Linux boot.
Native successful process exit must follow that source's recorded activation
and have settled unit state. Failed, unstarted, altered, removed and stale units
fail closed. An absent container alone is never successful completion evidence.

Success returns `name`, `project_directory`, `content_sha256` (SHA-256 of the
unresolved request content) and `completed: true`. Core revalidates all four
fields against its scoped immutable project before accepting the observation.
This primitive does not itself qualify the complete remote Application lifecycle
or authorize enabling unqualified completion-dependency realization.

## Completion-aware remote graph sequencing

Core's explicit completion-aware projector marks the generated init units and
keeps dependency ordering while removing native implicit activation of those
units. It also removes the generated `ExecStartPre` shell state heuristic.
The ordinary projector continues to refuse completion dependencies.

The completion-aware applier validates the whole graph and capability set before
publication, publishes all units without starting containers, and activates them
in dependency order. Each init unit must pass the source-bound observation before
its dependent starts. Waiting is bounded by the caller and a two-minute graph
limit; only observations repeat. Failed or interrupted mutations are never
replayed. These project mechanics do not establish full Application qualification.

For a completion-dependent Quadlet graph, Core sends published `runtime.quadlet.apply`
requests with `enable: true` and `autostart: false`. The node starts the exact owned
unit explicitly but clears its automatic target activation. After a node boot,
Core must reapply the graph and verify successful init completion on that boot
before starting the dependent. No automatic node start substitutes for this
verification. `autostart` is optional for ordinary apply, and requires an immutable
`project_directory` when present. Native evidence checks target membership; it
does not claim that a machine reboot was performed. Full Application engine
qualification remains separate.
