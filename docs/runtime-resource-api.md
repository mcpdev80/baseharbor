# Runtime Resource API

BaseHarbor's Runtime Resource API is the application-facing control surface for requesting logical resources while an application is already running.

The application continues to use standard protocols for the resource itself. The Runtime Resource API is only for lifecycle requests such as creating, looking up or deleting a logical resource.

## Canonical contract

The versioned OpenAPI 3.1 contract is:

`spec/runtime-api/v1/openapi.yaml`

OpenAPI is the normative application-facing API contract. Swagger UI, Scalar, Redoc or another renderer is only a presentation layer over that contract.

Applications use HTTP/JSON against the Runtime Resource API. Provider integrations remain separate and continue to use the BaseHarbor provider contract and its gRPC/Protocol Buffers direction.

## Environment boundary for interactive API documentation

| Environment | Interactive Swagger/Scalar-style UI |
| --- | --- |
| Development | available by default |
| Test / staging | disabled by default; explicit platform/operator opt-in |
| Production | disabled by default; explicit platform/operator opt-in |

This is deployment/platform policy, not application intent. `baseharbor.yaml` must not gain a portable field such as `swagger: true`.

The OpenAPI contract itself exists in every environment even when interactive documentation is disabled.

Enabling an interactive documentation UI never weakens API authentication or authorization and must not expose credentials, tokens or secure bindings.

## Resource lifecycle

The initial v1 contract defines:

```text
GET    /runtime/v1/capabilities
POST   /runtime/v1/resources
GET    /runtime/v1/resources/{resourceId}
DELETE /runtime/v1/resources/{resourceId}
GET    /runtime/v1/resources/{resourceId}/binding
GET    /runtime/v1/operations/{operationId}
```

A create request is provider-neutral:

```json
{
  "capability": "object-storage.s3",
  "name": "user-4711"
}
```

The application does not request SeaweedFS, AWS S3, Ceph RGW or another concrete product.

## Authorization

Repository inspection and runtime authorization are deliberately separate. Source evidence such as an S3 `CreateBucket` call may indicate that `runtime.create` is needed, but it never grants the operation.

A runtime request is accepted only when the authenticated workload identity is explicitly authorized for the requested capability and operation. Knowing a resource identifier is never sufficient authorization.

## Idempotency and asynchronous operations

Every mutating runtime request requires an `Idempotency-Key`.

If an application retries after a timeout or lost response, the same logical request must resolve to the same operation/resource outcome rather than creating duplicates.

The API also supports asynchronous provider work. A request may return a resource in `provisioning` state with an operation identity that is queried through `GET /runtime/v1/operations/{operationId}`.

## Bindings and secrets

`GET /runtime/v1/resources/{resourceId}/binding` exposes provider-neutral binding metadata.

Secret material is not returned through ordinary metadata fields. Credentials remain behind BaseHarbor's secure-binding/runtime-identity boundary.

## Compatibility

The Runtime Resource API is versioned independently from individual capability specifications. Capability-specific parameters belong to the relevant capability specification; provider-specific fields are not portable runtime request parameters. Breaking API changes require a new API version.
