# Application Runtime Broker

The **Application Runtime Broker** is BaseHarbor's per-application runtime control surface.

Managed secrets through OpenBao are the first production runtime capability implemented through the broker. Runtime resources and asynchronous operations use the same architectural boundary.

~~~text
application
    |
    | mTLS + app-scoped runtime identity
    v
Application Runtime Broker
    |
    +-- /runtime/v1/secrets
    +-- /runtime/v1/resources
    +-- /runtime/v1/operations/{id}
    +-- /runtime/v1/capabilities
    |
    v
capability/provider execution
~~~

## One broker per application

The broker is bound to one application/environment identity:

~~~text
spiffe://baseharbor/apps/<app>/<environment>
~~~

Existing app-qualified secret routes remain compatibility aliases. Canonical bound routes do not repeat the application name because mTLS already fixes the application identity.

## Secrets are a runtime module

Applications using dynamic managed secrets already require the broker today.

Canonical secret routes:

~~~text
POST   /runtime/v1/secrets
POST   /runtime/v1/secrets/resolve
PUT    /runtime/v1/secrets/resolve
DELETE /runtime/v1/secrets/resolve
~~~

OpenBao remains behind the broker and its credentials remain app-scoped.

## Runtime resources and operations

The same broker namespace is used for application-time resources:

~~~text
POST   /runtime/v1/resources
GET    /runtime/v1/resources/{resourceId}
DELETE /runtime/v1/resources/{resourceId}
GET    /runtime/v1/resources/{resourceId}/binding
GET    /runtime/v1/operations/{operationId}
GET    /runtime/v1/capabilities
~~~

A capability is only advertised as supported when an authorized provider execution path exists. Source-code detection never grants runtime authorization.

For `object-storage.s3/v1`, the broker delegates provider mutation to one shared **Runtime Provider Executor**. The executor authenticates the broker through the existing BaseHarbor workload SPIFFE identity, owns the provider-global administrative boundary, and performs bucket/IAM mutations against the selected S3 provider. The application and broker never receive provider-global credentials.

## Development Swagger / OpenAPI

When the broker is required in a dev or development environment, BaseHarbor enables interactive runtime API documentation by default.

The docs listener is separate from the mTLS runtime API and is published only on host loopback:

~~~text
http://127.0.0.1:<allocated-port>/
~~~

The port is allocated once per application and stored in owner-only BaseHarbor state. baha app apply, baha app up and baha app status report the URL.

The canonical contract remains spec/runtime-api/v1/openapi.yaml. Swagger UI assets are embedded in the BaseHarbor runtime image, so the developer docs do not require a public CDN.

| Environment | Interactive docs |
| --- | --- |
| dev / development | enabled by default |
| test / staging | disabled by default |
| prod / production | disabled by default |

Operators can explicitly override deployment policy with BASEHARBOR_RUNTIME_DOCS_ENABLED=true or false. This setting is not portable application intent and does not belong in baseharbor.yaml.

## Security boundary

The broker continues to run without a Docker/Podman socket and without provider-global administrator credentials. The Runtime Provider Executor also has no Docker/Podman socket, has no host-published port, and is reachable only from broker containers through the internal `baseharbor-runtime-control` network.

For runtime-only applications the per-app broker owns the deterministic backend network used by authorized workload services to reach `baseharbor-runtime`. When the app already has managed PostgreSQL/Valkey runtime services, that existing backend network remains authoritative. Runtime S3 provider-network attachment is service-specific: only services explicitly listed in the runtime permission receive it.

The development docs listener exposes documentation only. It does not bypass the authenticated runtime API and does not expose application credentials, OpenBao credentials, runtime bearer tokens or secure bindings.
