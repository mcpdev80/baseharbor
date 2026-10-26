# Application runtime identity

Applications need a non-human identity whenever they use the BaseHarbor Application Runtime Broker at runtime. Managed OpenBao secrets are the first production use of this identity; runtime resources and asynchronous capability operations reuse the same boundary.

The runtime identity is infrastructure supplied by BaseHarbor. It is not application business configuration and therefore does not add a generic runtime-API switch to `baseharbor.yaml`.

## Workload contract

For a repository workload that requires the Application Runtime Broker, BaseHarbor injects:

```text
BASEHARBOR_RUNTIME_API_URL=https://baseharbor-runtime:8443
BASEHARBOR_RUNTIME_TOKEN_FILE=/run/secrets/baseharbor-runtime-token
BASEHARBOR_RUNTIME_CA_FILE=/run/secrets/baseharbor-runtime-ca
BASEHARBOR_RUNTIME_CLIENT_CERT_FILE=/run/secrets/baseharbor-runtime-client-cert
BASEHARBOR_RUNTIME_CLIENT_KEY_FILE=/run/secrets/baseharbor-runtime-client-key
```

Applications use ordinary HTTPS with the supplied mTLS identity and app-scoped runtime token. No mandatory BaseHarbor SDK is required.

The broker accepts only the application identity encoded as:

```text
spiffe://baseharbor/apps/<app>/<environment>
```

## Runtime API

Canonical application-bound runtime routes live below `/runtime/v1`.

Managed secret routes:

```text
POST   /runtime/v1/secrets
POST   /runtime/v1/secrets/resolve
PUT    /runtime/v1/secrets/resolve
DELETE /runtime/v1/secrets/resolve
```

Runtime resource and asynchronous operation routes use the same broker namespace. Existing app-qualified secret routes under `/runtime/v1/apps/{app}/...` remain compatibility aliases.

The runtime identity never authenticates to the normal operator API under `/api/v1/...`.

Each identity is scoped to exactly one application/environment. An identity issued for one application cannot be reused for another application's broker.

## Development API documentation

When a broker is required in `dev` or `development`, BaseHarbor exposes embedded Swagger/OpenAPI documentation on a separately allocated host-loopback-only URL. The documentation listener does not provide an authentication bypass into the mTLS runtime API.

Test/staging and production keep interactive documentation disabled by default.

## Rotation and revocation

Operators can invalidate or rotate runtime identity material without changing logical resource or secret references.

Existing secret references such as:

```text
baseharbor://secrets/dyn-0123456789abcdef0123456789abcdef
```

remain stable across identity rotation.

Applications that do not require any BaseHarbor runtime capability receive no Application Runtime Broker identity.
