# Application contract

> BaseHarbor should hide operational complexity without hiding standard interfaces.

An application declares what backend capabilities and secret names it requires. BaseHarbor owns provisioning, isolation, readiness and lifecycle. The application should not need to know OpenBao AppRole names, KV v2 mount paths, Compose network names or BaseHarbor internal identifiers.

A second design test is equally important:

> The same application should still be runnable without BaseHarbor when another environment provides the same standard interfaces.

## Application identity and deployment context

`app.name` is the stable logical application identity. `app.environment` is deployment context, not part of the application's intrinsic identity.

The same logical application can therefore be instantiated as `dev`, `test`, `staging`, `production` or a customer-specific deployment without being redefined as a different application. In v0.2.0 Compose uses the environment value for runtime isolation and naming. Future providers such as Kubernetes or OpenShift may realize the same logical requirements differently.

Provider-specific implementation details such as Compose project names, networks, host ports, volumes or OpenBao paths are not portable application requirements and must not become application dependencies.

## Required secrets

Required secrets are part of the declarative application contract:

```yaml
version: 1

app:
  name: mailflow
  environment: production

services:
  postgres:
    enabled: true
  redis:
    enabled: true
  secrets:
    enabled: true

secrets:
  required:
    - name: OPENAI_API_KEY
    - name: SMTP_PASSWORD
```

The contract stores secret names only. Secret values never belong in the manifest.

`baha app create` can declare the same contract directly:

```bash
baha app create mailflow \
  --environment production \
  --postgres \
  --redis \
  --require-secret OPENAI_API_KEY \
  --require-secret SMTP_PASSWORD
```

Declaring a required secret automatically enables managed secrets.

## Multiple instances of the same service

The common case stays deliberately small:

```yaml
services:
  postgres:
    enabled: true
  redis:
    enabled: true
```

Each enabled service above resolves to one logical instance named `default`.

When an application actually needs several independent services of the same type, it uses stable logical names instead of a numeric count:

```yaml
services:
  postgres:
    instances:
      primary: {}
      analytics: {}

  redis:
    instances:
      cache: {}
      sessions: {}

  secrets:
    enabled: false
```

Every named instance gets its own credentials, loopback port, volume, lifecycle identity and binding directory. Adding a second instance does not rotate an existing instance.

Named PostgreSQL instances expose variables such as:

```text
DATABASE_PRIMARY_URL=postgresql://...
DATABASE_ANALYTICS_URL=postgresql://...
```

Named Redis/Valkey instances expose variables such as:

```text
REDIS_CACHE_URL=redis://...
REDIS_SESSIONS_URL=redis://...
VALKEY_CACHE_URL=redis://...
VALKEY_SESSIONS_URL=redis://...
```

A single instance still receives the conventional generic aliases. With multiple instances, `default` is preferred for the generic alias. For PostgreSQL, `primary` is also accepted as the preferred generic target when no `default` instance exists. If there is no unambiguous preferred instance, BaseHarbor emits only named variables instead of guessing.

Multiple logical instances are not an HA mechanism. A `primary` PostgreSQL instance with future high availability remains one logical service with one stable application-facing endpoint while BaseHarbor manages the replicated topology behind it. See `docs/decisions/0001-service-instances-and-ha-intent.md`.

## Native runtime contract

Applications do not log in to BaseHarbor and do not require the `baha` process, a BaseHarbor SDK or a proprietary protocol at runtime.

When `baha app apply` materializes PostgreSQL or Valkey, BaseHarbor publishes the managed service on an automatically allocated **loopback-only** host port and generates normal application-facing connection information.

The generated owner-only dotenv contract is:

```text
.baseharbor/apps/<app>/runtime/application.env
```

For PostgreSQL and Valkey it contains standard variables such as:

```text
BASEHARBOR_APP_NAME=mailflow
BASEHARBOR_ENVIRONMENT=dev
BASEHARBOR_BINDINGS=/absolute/path/to/.baseharbor/apps/mailflow/runtime/bindings
DATABASE_URL=postgresql://...
REDIS_URL=redis://...
VALKEY_URL=redis://...
```

A developer may point normal framework or IDE dotenv support at that file. The application itself only sees standard environment variables and native service protocols.

BaseHarbor also materializes owner-only file bindings. Service ports are bound to `127.0.0.1`, never to all host interfaces by default.

## CLI convenience is optional

`baha app env` exists for developer convenience and inspection; it is not a runtime dependency.

```bash
baha app env mailflow
baha app env mailflow --format json
baha app env mailflow --format yaml
baha app env mailflow --format shell
baha app env mailflow --path
```

Credential-bearing service URLs are masked by default. Printing them requires an explicit operation:

```bash
baha app env mailflow --reveal
```

## Lifecycle semantics

`plan` includes a requirement action for every required secret. `preflight` is read-only. Before workload start, `apply` and `up` evaluate all required secrets and fail closed when a required value is missing or unusable.

`baha app status` and `baha app doctor` report readiness metadata only and never reveal secret values.

## Secret delivery remains separate from the requirement contract

The application declares what it needs, not how BaseHarbor internally stores or rotates it.

Runtime delivery may evolve independently and can use standard mechanisms such as:

- in-memory secret files
- environment injection where explicitly appropriate
- workload identity / native OpenBao access
- Kubernetes/OpenShift-native secret projection

The selected runtime provider owns that delivery decision. The `secrets.required` contract remains stable.

## Secrets are not general application configuration

BaseHarbor deliberately distinguishes secret/infrastructure inputs from normal application configuration. Business settings such as batch sizes, language or classification thresholds remain application-owned.

## Stable application-facing interfaces

Applications should consume normal ecosystem interfaces such as:

```text
DATABASE_URL
REDIS_URL
secret file / standard secret mechanism
S3_ENDPOINT
OIDC_ISSUER
```

Applications should not depend on:

```text
OpenBao AppRole names
OpenBao KV-v2 internal paths
BaseHarbor policy names
Compose network names
BaseHarbor internal IDs
```

This keeps BaseHarbor useful without making applications proprietary to BaseHarbor.
