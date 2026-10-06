# Protected Machine HTTP Interface v1

## Scope

This specification defines the authenticated HTTPS projection of the BaseHarbor Machine Interface used by Console and other web-capable machine clients.

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

## Discovery

`GET /api/v1/machine/discovery`

Returns the current machine contract, execution contract, semantic operations and negotiated HTTP capabilities.

Clients MUST discover operations rather than infer support from CLI spelling.

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

Operation input is not retained in ordinary execution metadata.

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
