# Application contract

> BaseHarbor should hide operational complexity without hiding standard interfaces.

An application declares what backend capabilities and secret names it requires. BaseHarbor owns provisioning, isolation, readiness and lifecycle. The application should not need to know OpenBao AppRole names, KV v2 mount paths, Compose network names or BaseHarbor internal identifiers.

A second design test is equally important:

> The same application should still be runnable without BaseHarbor when another environment provides the same standard interfaces.

## Application identity and deployment context

`app.name` is the stable logical application identity. `app.environment` is deployment context, not part of the application's intrinsic identity.

The same logical application can therefore be instantiated as `dev`, `test`, `staging`, `production` or a customer-specific deployment without being redefined as a different application. In v0.3.0 Compose uses the environment value for runtime isolation and naming. Future providers such as Kubernetes or OpenShift may realize the same logical requirements differently.

Provider-specific implementation details such as Compose project names, networks, host ports, volumes or OpenBao paths are not portable application requirements and must not become application dependencies.

## Portable application contract versus deployment state

`baseharbor.yaml` is the portable, repository-owned desired-state contract. It describes application requirements that should survive a future change of runtime/provider.

The current Compose deployment may also need operator/runtime inputs that are **not** portable application requirements. In v0.3 these are stored separately in protected BaseHarbor runtime state and can include:

- the public FQDN used for the current deployment;
- the selected deployment TLS mode;
- the source/normalized files for an existing/BYOC certificate pair;
- automatically selected host-port fallbacks for configurable Compose publishers;
- generated Compose overrides and runtime identity material.

Those values must not be copied into the portable manifest merely because the Compose provider currently needs them. Future Kubernetes/OpenShift providers may realize the same application requirements through entirely different primitives.

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

The CLI can create the same declaration without editing YAML by hand:

```bash
baha app create mailflow \
  --postgres-instance primary \
  --postgres-instance analytics \
  --redis-instance cache \
  --redis-instance sessions
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

Bindings for multiple named instances are nested by stable instance identity:

```text
bindings/
├── postgres/
│   ├── primary/
│   │   ├── host
│   │   ├── port
│   │   ├── database
│   │   ├── username
│   │   ├── password
│   │   └── uri
│   └── analytics/
│       └── ...
└── valkey/
    ├── cache/
    │   └── ...
    └── sessions/
        └── ...
```

Multiple logical instances are not an HA mechanism. A `primary` PostgreSQL instance with future high availability remains one logical service with one stable application-facing endpoint while BaseHarbor manages the replicated topology behind it. See `docs/decisions/0001-service-instances-and-ha-intent.md`.

## Workload-only applications

A repository may explicitly declare an application-owned Compose workload without also requesting an artificial managed PostgreSQL or Valkey dependency. This is a valid v0.3 application shape when the workload is explicit.

BaseHarbor does not invent backend environment variables, backend networks, volumes or credentials for workload-only applications. A manifest with neither a managed capability nor an explicit workload remains invalid. Managed-secrets-only applications remain unsupported where the current v0.3 runtime broker requires a materialized managed backend.

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

BaseHarbor also materializes file bindings:

```text
bindings/
├── metadata.json
├── postgres/
│   ├── host
│   ├── port
│   ├── database
│   ├── username
│   ├── password
│   └── uri
└── valkey/
    ├── host
    ├── port
    ├── password
    └── uri
```

Directories are owner-only and binding files are written with owner-only permissions. Service ports are bound to `127.0.0.1`, never to all host interfaces by default.

The contract intentionally supports both common consumption styles:

```text
DATABASE_URL
```

or:

```text
<bindings>/postgres/password
```

No application code has to call BaseHarbor to retrieve either form.

## CLI convenience is optional

`baha app env` and the trusted-local developer-access commands exist for convenience; they are not runtime dependencies.

```bash
baha app env mailflow
baha app env mailflow --format json
baha app env mailflow --format yaml
baha app env mailflow --format shell
baha app env mailflow --path
baha app psql
baha app valkey
baha app logs
```

Credential-bearing service URLs are masked by default. Printing them requires an explicit operation such as `--reveal` where supported.

`--path` prints the protected `application.env` path so an editor, IDE, process manager or normal dotenv loader can consume it directly.

## Lifecycle and readiness semantics

`plan` includes a requirement action for every required secret.

`preflight` is read-only. When an application secret scope already exists it reports whether each required secret is present and usable. Before the first `apply`, presence is reported as unknown rather than materializing state during preflight.

The first `apply` may materialize the runtime definition and isolated OpenBao scope. Before workload start it evaluates every required secret. Missing or unreadable required secrets stop the lifecycle at that boundary.

```text
baha app apply
    |
    +-- materialize runtime definition
    +-- materialize isolated secret scope
    +-- required secret missing/unusable -> STOP before workload start

baha app secret set ...
    |
    v
baha app apply / baha app up
    |
    +-- required secrets present and usable
    +-- start workload
    +-- verify runtime
```

`baha app up` always fails closed before workload start when a required secret is missing or unusable.

For repository Compose workloads, readiness also includes selected service state/health and conventional application-owned HTTP/HTTPS publishers. A running container is not automatically READY. Redirects count as reachable web exposure; 5xx or unreachable endpoints do not. Hostname-bound local HTTPS uses the configured deployment FQDN as HTTP Host/TLS ServerName while BaseHarbor still dials the local published socket.

`baha app show`, `status` and `doctor` report readiness metadata only and never reveal secret values.

## Deployment TLS is not an application dependency

v0.3 supports an existing/BYOC certificate lifecycle for the current repository Compose deployment. `baha app tls update --check` is read-only; `baha app tls update` validates the source certificate/key pair and FQDN, refuses downgrades, installs owner-only normalized files, restarts the workload when required and verifies readiness.

This does not turn certificate source directories, Compose TLS files or Caddy details into portable application requirements. BaseHarbor-managed ACME issuance, OpenBao PKI issuance, automatic rotation and provider-neutral `tls.certificate` intent remain future work.

## Secret delivery remains separate from the requirement contract

The application declares what it needs, not how BaseHarbor internally stores or rotates it.

Runtime delivery may evolve independently and can use standard mechanisms such as:

- in-memory secret files
- environment injection where explicitly appropriate
- workload identity / native OpenBao access
- Kubernetes/OpenShift-native secret projection

The selected runtime provider owns that delivery decision. The `secrets.required` contract remains stable.

## Secrets are not general application configuration

BaseHarbor deliberately distinguishes secret/infrastructure inputs from normal application configuration.

Typical secret or infrastructure inputs:

```text
DATABASE_PASSWORD
S3_SECRET_KEY
OPENAI_API_KEY
SMTP_PASSWORD
```

Typical application configuration that should remain owned by the application:

```text
BACKFILL_BATCH_SIZE
DEFAULT_LANGUAGE
CLASSIFICATION_THRESHOLD
```

BaseHarbor must not become a universal configuration framework for application business behavior.

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
Compose-generated host ports
certificate source directories
```

This keeps BaseHarbor useful without making applications proprietary to BaseHarbor.
