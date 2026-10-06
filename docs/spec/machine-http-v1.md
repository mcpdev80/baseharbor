# Protected Machine HTTP Interface v1

## Scope

This specification defines the authenticated HTTPS projection of the BaseHarbor Machine Interface used by Console and other web-capable machine clients.

The public envelope schema is
`contracts/machine/v1/control.schema.json`. Its `discovery`, `execute_request`,
`execution`, `event` and `error_result` definitions describe the serialized Core
records. `control.golden.json` contains explicitly synthetic examples generated
from Core's types and registries by
`go run ./scripts/tools/machine-control-fixtures`. Consumers pin both files to an
immutable public Core commit and retain their SHA-256 digests. Envelope validation
does not replace operation-specific input validation, authentication or policy.

The HTTP interface is a transport over the same BaseHarbor semantic operations used by CLI, JSON and MCP. It MUST NOT introduce a second lifecycle, desired-state store, authorization model or runtime abstraction.

## Security boundary

All machine HTTP endpoints:

- MUST be served only by the BaseHarbor TLS management listener;
- MUST reject plaintext HTTP with no downgrade fallback;
- MUST pass through the existing authenticated operator HTTP middleware;
- MUST bind the verified transport principal into the shared Machine Operator Authorization boundary;
- MUST preserve operation safety, policy and confirmation semantics;
- MUST keep ordinary responses, execution metadata and event metadata secret-safe.

Browser clients use memory-only bearer authentication against the configured
same-origin HTTPS Core endpoint. Origin-bearing requests from a different
authority are rejected; Core does not authenticate browser cookies. Console
transports must reject foreign discovered destinations and redirects before
forwarding a credential. SSE therefore uses authenticated fetch streaming,
not a cookie-only EventSource assumption.

Each admitted event/log/exec stream ends at the earlier of verified token expiry
and five minutes. Provider streams close on request cancellation/deadline; socket
writes are bounded to 30 seconds, clamped to the session deadline. Reconnection
requires authentication and authorization again. This does not promise instant
revocation of identity-provider group changes within an issued token's lifetime.

Transport authentication and operator authorization are separate requirements. A valid TLS connection does not by itself authorize an operation.

## Console installation topology

The default is one optional Console connected directly to one selected Core in
one BaseHarbor installation and security boundary. Serve Console and protected
Core HTTP on the same HTTPS origin. Core remains the authority for lifecycle,
policy, identities and secrets. The Console owns no parallel authority or
installation state. Core checks browser Origin against the destination HTTPS
authority, including its port; forwarding headers cannot authorize a foreign
Origin, and duplicate Origin headers are rejected.

The v0.4.23 Console rejects a configured foreign Core before authentication and
pins all discovered destinations to the selected Core. It does not provide a
central multi-Core backend, Dev-to-Prod forwarding or installation federation.
Cross-origin operation requires a separate explicit trusted-origin design; it
is not enabled by CORS headers or redirects in this release. A future client-side
installation selector must authenticate directly to each newly selected Core
and discard the previous installation's credentials and streams.

## Discovery

`GET /api/v1/machine/discovery`

Returns the current machine contract, execution contract, semantic operations and negotiated HTTP capabilities.

Clients MUST discover operations rather than infer support from CLI spelling.

HTTP discovery lists only the operations implemented by the selected HTTP
executor. It takes their descriptors and safety metadata from the shared Core
registry. The same support registry selects execution handlers and rejects
unimplemented operations before creating an execution. A semantic operation
available through another projection does not imply HTTP support.

HTTP discovery also returns an `http` map of named endpoint bindings, each with
`href`, `method` and optional `protocol`. The same Core registry installs the
routes and produces these descriptors. Browsers bootstrap only the documented
discovery endpoint, then use these bindings for execution and streams. Named
`{execution_id}` and `{stream_id}` placeholders accept validated resource IDs.
Every resolved destination remains pinned to the configured HTTPS Core origin;
discovery never authorizes a redirect, credential-bearing URL or foreign origin.

## Executions

`POST /api/v1/machine/executions`

Request:

```json
{
  "operation_id": "status",
  "context": {
    "application": "demo",
    "environment": "prod",
    "target": "prod-eu"
  },
  "input": {}
}
```

`context.environment` is mandatory. Context selectors are the authorization scope. Operation input MUST NOT override an explicitly authorized application, environment, target, workspace or runtime resource.

Accepted operations return HTTP `202` with an `Execution v1` resource and a `Location` header.

The execution model exposes:

- stable `execution_id`;
- semantic `operation_id`;
- stable actor reference;
- bounded operation context;
- `pending | running | succeeded | failed | cancelled`;
- structured progress;
- structured result or typed error;
- start/finish timestamps.

Operation input is not retained in ordinary execution metadata. The semantic
executor rejects unknown fields, duplicate keys at every depth, non-object input,
invalid UTF-8, nesting beyond 64 levels and trailing JSON values without
including submitted field names or values in its validation failure.

## Execution status and events

`GET /api/v1/machine/executions/{execution_id}`

Returns the structured execution resource.

`GET /api/v1/machine/executions/{execution_id}/events`

Returns semantic Server-Sent Events using the Machine Event v1 envelope.

The initial event vocabulary includes:

- `operation.started`;
- `operation.progress`;
- `operation.succeeded`;
- `operation.failed`;
- `operation.cancelled`;
- application/provider/target/runtime-resource state events for compatible future producers.

Execution metadata and event streams are bound to the authenticated execution actor. One authenticated operator MUST NOT read another operator's execution stream merely by knowing the execution identifier.

## Navigation operations

The semantic machine registry includes structured navigation operations required by Console.

`app.list` is target-scoped and returns secret-safe deployment summaries containing stable application/deployment identities, target/environment, runtime provider, observed state/readiness and source availability.

`target.list` returns secret-safe Target summaries containing runtime provider, access reference, scope and selector state.

Provider listing/inspection and organization inspection use their existing machine operations. `workspace.list`, `workspace.resolve` and `workspace.status` provide the structured workspace navigation surface.

These are semantic machine operations and therefore remain available consistently to MCP and HTTP.

## Runtime logs

`POST /api/v1/machine/streams/logs`

The request uses Stream Contract v1 and MUST identify:

- application/environment/target context where applicable;
- runtime `resource_kind`;
- stable runtime `resource_id`;
- bounded historical selection such as `since` or `tail` where supported.

The request is authorized through the shared Machine Operator Authorization boundary before a Runtime Explorer adapter is invoked.

When the active runtime implementation does not advertise log streaming, BaseHarbor returns typed `unsupported_operation`; it MUST NOT fall back to runtime-native unauthenticated ports or shell commands.

Stream response metadata identifies the stream, actor, Target and runtime resource without embedding credentials.

Successful admission flushes the response headers even when the producer is idle.
Each output chunk is flushed without waiting for EOF. Client cancellation and
credential expiry terminate the observation; a follow client MUST NOT silently
replay the request or treat transport EOF as successful application execution.

## Runtime exec / terminal boundary

`POST /api/v1/machine/streams/exec`

Exec uses the same authenticated TLS and authorization boundary and requires:

- explicit runtime resource kind/id;
- an explicit bounded command;
- a Runtime Explorer capability permitting exec for that resource.

Container/pod exec does not imply host shell access.

The v1 boundary is intentionally capability-gated until a Runtime Explorer implementation supplies an explicit compatible session transport. BaseHarbor MUST return typed `unsupported_operation` rather than inventing a generic shell or CLI passthrough.

## Lifecycle parity

The HTTP semantic executor directly reuses the existing BaseHarbor Core functions and result models for application, workspace, provider, organization and lifecycle operations.

Implementations MUST NOT:

- invoke `baha` as a subprocess;
- parse human CLI output;
- bypass shared authorization/policy/ownership/preflight/reconciliation/verification;
- introduce Console-specific desired state;
- permit operation input to escape its authorized context.

## Relationship to Runtime Explorer and Target Access

Runtime-resource discovery and concrete log/exec implementations are supplied by the provider-neutral Runtime Explorer contract.

Remote execution or observation reaches a Target only through the Target Access Provider boundary selected by Core. Console and HTTP clients MUST NOT connect directly to a Target Access implementation.

## Interactive container terminal

`POST /api/v1/machine/terminals` admits a bounded terminal and returns a
`StreamDescriptor`. The request uses `StreamRequest` with `tty: true`, explicit
container kind/stable id, environment, Target, argv and `rows`/`columns` in 1–512.
The selected Runtime Explorer checks resource ownership and environment before
calling the typed runtime terminal primitive. A platform or managed resource is
required. Local Linux Docker/Podman backends supply a PTY; remote terminal
qualification remains part of the remote integration requirements.

The creator's verified issuer/subject owns the session, including in `dev`.

`terminal.ready` confirms transport admission, not readiness of the requested
program. Clients attach input and terminal protocol replies before rendering
initial output. The transport preserves input bytes; the container's terminal
owns line editing and signal interpretation.
Every control request requires bearer authentication and reuses the shared
operator boundary. There is no terminal cookie, query token, host-command API or
browser-to-runtime connection.

| Endpoint | Semantics |
| --- | --- |
| `GET /api/v1/machine/terminals/{stream_id}/events` | One SSE output attachment; `terminal.ready`, base64 `terminal.output`, `terminal.exit` with exit code. |
| `POST /api/v1/machine/terminals/{stream_id}/input` | A `TerminalInput` frame containing base64 bytes or resize dimensions. |
| `DELETE /api/v1/machine/terminals/{stream_id}` | Close the transport and its runtime CLI process. |

Input sequence starts at one and must increment exactly. A sequence is consumed
before the side effect; an ambiguous write closes the session and is never
replayed. Output sequence is independent. `Last-Event-ID`, a second attachment
and foreign actors are rejected. SSE EOF without `terminal.exit` is a transport
failure, not successful command completion.

Sessions expire at the earlier of the admitted token expiry and five minutes.
Unattached sessions close after ten seconds. Capacity is bounded to sixteen
sessions globally and four per actor. Output reads one 16 KiB chunk at a time;
socket writes are deadline-bound, input writes have a five-second limit. Output
disconnect or cancellation closes the runtime transport. Runtime-side process
termination and cleanup still require the real Docker/Podman qualification;
these source tests alone do not establish that a container command is terminated
by every runtime's exec disconnect behavior.

Wire schemas are packaged in `contracts/machine/v1/terminal-*.schema.json` and
resolve through the offline public registry. Terminal bytes and argv are never
used as audit/log fields; the descriptor supplies safe actor/resource context.

## Resolved tenant permissions

When the protected API middleware resolves a tenant membership, machine
authorization uses Core's existing RBAC service: Viewer can perform read-only
operations, while Editor can also mutate and delete. Unknown roles, missing
membership identifiers and unknown safety classes deny. An authenticated
request in `dev` retains this check; it does not become a trusted local operator.
This role check supplements effective policy and resource ownership. It does
not establish tenant isolation for an adapter's inventory or runtime resources.

### Navigation result consumption

`app.list`, `target.list`, `workspace.list` and `runtime.list` use the generated
[read-model schema](https://github.com/mcpdev80/baseharbor/blob/HEAD/contracts/machine/v1/read-models.schema.json) and
[synthetic examples](https://github.com/mcpdev80/baseharbor/blob/HEAD/contracts/machine/v1/read-models.golden.json). The
source is Core's semantic Go types, shared across CLI JSON, MCP and HTTP. The
consumer validates the result only after a correlated successful execution.
Empty Runtime Explorer inventory can be `null`. Configured Targets do not
report connection health, and deployment observations do not imply live runtime
health. Unknown result fields or incompatible versions require a supported
consumer update; a live error must never select fixture data.

Terminal consumer examples are emitted from actual `machine.StreamDescriptor`,
`machine.TerminalEvent` and `machine.TerminalInput` records in the generated
read-model artifact. Runtime capability examples use Core's actual
`runtimeexplorer.CapabilitySet`. These examples are explicitly synthetic and
provide decoding/conformance checks only; they do not qualify an authenticated
browser or a real PTY/runtime journey.

## Managed trust rotation

`openbao.rotate` is advertised only when the HTTP semantic executor implements
it. Select an explicit installation target and environment and send
`{"approval":true}`. Missing approval, mismatched input target and application
or workspace selectors are rejected before runtime access. Core selects the
protected recovery file from its own installation configuration; HTTP input
cannot supply a recovery path or recovery keys.

The executor calls the same credential and service-CA rotation used by CLI/MCP,
then returns only initialized/unsealed/manager-ready flags. Replacement
verification and retirement remain Core responsibilities. Protected regular
recovery material is validated before mutation. Failure or interrupted
observation must not be treated as completion or automatically replayed.
Execution observation remains bounded by the existing authenticated five-minute
HTTP/session lifetime; inspect the existing execution if observation ends.
