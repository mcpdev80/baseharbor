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
  "capability": "object-storage.s3/v1",
  "name": "user-4711"
}
```

The application does not request SeaweedFS, AWS S3, Ceph RGW or another concrete product. For `object-storage.s3/v1`, the current Compose reference path resolves to SeaweedFS behind the provider boundary.

## Authorization

Repository inspection and runtime authorization are deliberately separate. Source evidence such as an S3 `CreateBucket` call may indicate that `runtime.create` is needed, but it never grants the operation.

A runtime request is accepted only when the authenticated workload identity is explicitly authorized for the requested capability and operation. Knowing a resource identifier is never sufficient authorization.

## Idempotency and asynchronous operations

Every mutating runtime request requires an `Idempotency-Key`.

If an application retries after a timeout or lost response, the same logical request must resolve to the same operation/resource outcome rather than creating duplicates.

The API executes mutations asynchronously. Create and delete return an operation identity with `pending`, `running`, `succeeded` or `failed` state. Operation state is persisted by the per-application broker and unfinished work is reconciled after broker restart.

## Bindings and secrets

`GET /runtime/v1/resources/{resourceId}/binding` is the explicit authenticated binding endpoint. For `object-storage.s3/v1` it returns the S3 endpoint, bucket, region and resource-scoped credentials required by a native S3 client.

The credentials are intentionally **not** stored in asynchronous operation state, ordinary resource metadata, logs, metrics or `baseharbor.yaml`. They remain in protected executor state and are disclosed only through the application-scoped mTLS + runtime-token protected binding request. Provider-global administrator credentials never leave the Runtime Provider Executor.

## Compatibility

The Runtime Resource API is versioned independently from individual capability specifications. Capability-specific parameters belong to the relevant capability specification; provider-specific fields are not portable runtime request parameters. Breaking API changes require a new API version.


## Runtime provider execution path

For the first executable runtime capability, the request path is:

```text
authorized workload service
        | mTLS + runtime token
        v
Application Runtime Broker
        | mTLS / SPIFFE application identity
        v
shared Runtime Provider Executor
        | provider-admin boundary
        v
object-storage.s3/v1 provider
```

The broker and executor do not mount a Docker/Podman socket. The shared executor has no host-published port and communicates with brokers only on the internal `baseharbor-runtime-control` network. Runtime S3 workloads receive provider-network access only when that specific workload service is explicitly authorized for the S3 runtime capability.
