# Application contract

> BaseHarbor should hide operational complexity without hiding standard interfaces.

An application declares what backend capabilities and secret names it requires. BaseHarbor owns provisioning, isolation, readiness and lifecycle. The application should not need to know OpenBao AppRole names, KV v2 mount paths, Compose network names or BaseHarbor internal identifiers.

A second design test is equally important:

> The same application should still be runnable without BaseHarbor when another environment provides the same standard interfaces.

## Application identity and deployment context

`app.name` is the stable logical application identity. `app.environment` is deployment context, not part of the application's intrinsic identity.

The same logical application can therefore be instantiated as `dev`, `test`, `staging`, `production` or a customer-specific deployment without being redefined as a different application. In the current Compose implementation, the environment value is used for runtime isolation and naming. Future providers such as Kubernetes or OpenShift may realize the same logical requirements differently.

Provider-specific implementation details such as Compose project names, networks, host ports, volumes or OpenBao paths are not portable application requirements and must not become application dependencies.

## Portable application contract versus deployment state

`baseharbor.yaml` Manifest v1 is the supported repository-owned compatibility contract. BaseHarbor translates its portable application intent into the provider-neutral `PortableContract`; Compose-specific compatibility fields are not part of that portable view.

The current Compose deployment may also need operator/runtime inputs that are **not** portable application requirements. In v0.4 these remain stored separately in protected BaseHarbor runtime state and can include:

- the public FQDN used for the current deployment;
- the selected deployment TLS mode;
- the source/normalized files for an existing/BYOC certificate pair;
- automatically selected host-port fallbacks for configurable Compose publishers;
- generated Compose overrides and runtime identity material.

Those values must not be added to portable application intent merely because the Compose provider currently needs them. Future Kubernetes/OpenShift providers may realize the same application requirements through entirely different primitives.

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

A repository may explicitly declare an application-owned Compose workload without also requesting an artificial managed PostgreSQL or Valkey dependency. This remains a valid application shape when the workload is explicit.

BaseHarbor does not invent backend environment variables, backend networks, volumes or credentials for workload-only applications. A manifest with neither a managed capability nor an explicit workload remains invalid. Managed-secrets-only applications remain unsupported where the current runtime broker requires a materialized managed backend.

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

In v0.4.1 the protected runtime metadata additionally records the resolved logical capability, provider and workload binding. This metadata is for BaseHarbor control surfaces; applications continue to consume the same standard environment variables and files.

In v0.4.2 provider placement and lifecycle ownership are recorded separately in the protected provider registry. Shared, dedicated or external placement remains operator state and does not become application contract content.

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

The current Compose implementation supports an existing/BYOC certificate lifecycle for the repository deployment. `baha app tls update --check` is read-only; `baha app tls update` validates the source certificate/key pair and FQDN, refuses downgrades, installs owner-only normalized files, restarts the workload when required and verifies readiness.

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


## Managed HTTP exposure

Endpoint discovery and exposure are intentionally separate.

An application-owned Compose publisher remains owned by the application. BaseHarbor may discover, observe and verify it, but does not provision or delete it.

Managed exposure is explicit portable intent:

```yaml
workload:
  compose: compose.yaml
  services:
    - web

exposure:
  http:
    - name: public
      service: web
      port: 8080
      protocol: https
      visibility: public
```

The portable fields describe only the application requirement: logical exposure name, logical workload service, target port, HTTP/HTTPS transport and public/internal visibility. The concrete FQDN, published host port, certificate source, Compose network and reverse-proxy configuration remain deployment/provider state.

For v0.4.4 the Compose reference provider is Caddy. Managed HTTPS reuses the existing/BYOC deployment TLS state. Managed ACME, OpenBao PKI issuance and automatic certificate renewal remain future work.

`visibility: public` is the canonical default. `visibility: internal` restricts the current Compose reference binding to loopback/local reachability.

## Secure bindings and workload identity in v0.4.5

v0.4.5 adds a provider-neutral secure-binding model below the application manifest. It describes the security material required to connect a workload to a capability without placing provider internals or secret values into portable application intent.

A secure binding can describe:

- workload identity;
- opaque credential references;
- trust/CA references;
- least-privilege authorization metadata;
- opaque required-secret references;
- whether renewal, rotation and revocation are supported;
- machine-readable security diagnostics.

The current managed-secrets path maps its existing runtime identity to the SPIFFE subject `spiffe://baseharbor/apps/<application>/<environment>`. The binding contains only logical references. OpenBao AppRole names, RoleIDs, SecretIDs, KV paths, policy names, private keys and secret values remain protected provider/runtime state.

This is intentionally below Manifest v1. Applications still declare only secret names and capability requirements; they do not configure SPIFFE, OpenBao, certificate files or BaseHarbor credential references.

Secure-binding metadata is validated before provider preflight. Invalid or ambiguous references therefore fail before provider mutation.


## S3-compatible object storage in v0.4.6

Object storage is explicit portable application intent. Applications declare logical bucket identities, not a storage product:

```yaml
services:
  object_storage:
    buckets:
      attachments: {}
      exports: {}
```

The deterministic CLI path is equivalent:

```bash
baha app init mailflow \
  --s3-bucket attachments \
  --s3-bucket exports
```

A single default bucket can also be requested with `--s3`.

Each logical bucket maps to `object-storage.s3/v1`. SeaweedFS is the current shared Compose reference provider, but `baseharbor.yaml` contains no SeaweedFS image, port, physical bucket name, IAM user or credential value. Those details remain provider/deployment state.

For the preferred bucket, BaseHarbor materializes standard S3/AWS-compatible variables:

```text
S3_ENDPOINT=http://...
S3_BUCKET=...
S3_REGION=us-east-1
AWS_ENDPOINT_URL=http://...
AWS_REGION=us-east-1
AWS_ACCESS_KEY_ID=...
AWS_SECRET_ACCESS_KEY=...
```

Named buckets additionally receive `S3_<NAME>_ENDPOINT`, `S3_<NAME>_BUCKET`, `S3_<NAME>_REGION`, `S3_<NAME>_ACCESS_KEY_ID` and `S3_<NAME>_SECRET_ACCESS_KEY`.

File bindings are written below `bindings/object-storage-s3/<bucket>/` and contain endpoint, bucket, region, access key and secret key. Credential files are owner-only, and normal `baha app env` output masks access/secret keys.

The current SeaweedFS provider creates a separate bucket-scoped identity for every logical bucket and verifies readiness with a real authenticated SigV4 Put/Get flow. A shared provider instance therefore does not imply shared authorization between applications or buckets.

Repository workloads use the provider's internal Compose endpoint over the BaseHarbor-owned object-storage integration network; host-side consumers use the protected loopback endpoint. Neither address is portable application identity.

Backup/restore does **not** yet capture object contents. v0.4.6 therefore refuses `baha app backup` and `baha app restore` for applications containing managed object storage instead of producing or accepting an incomplete recovery unit.

## OTLP telemetry export in v0.4.7

Applications may opt into provider-neutral OTLP export:

```yaml
workload:
  compose: compose.yaml
  services:
    - api
    - worker

telemetry:
  otlp:
    signals:
      - traces
      - metrics
```

The selected workload services receive standard OpenTelemetry environment variables. They do not receive a BaseHarbor-specific telemetry API and do not name the Collector, Tempo, Prometheus, Loki or Grafana.

The current v1 binding uses OTLP HTTP/protobuf export. BaseHarbor may satisfy it through the shared managed OpenTelemetry Collector or an external OTLP destination selected in deployment state. Provider endpoint addresses and authorization material are not portable application intent.

An external destination can be supplied by deployment environment using `BASEHARBOR_OTLP_ENDPOINT`. Optional sensitive OTLP headers use `BASEHARBOR_OTLP_HEADERS` and are injected only at the trusted workload/provider boundary; they are not written into `baseharbor.yaml`.

Requesting OTLP transport alone never provisions Prometheus, Loki, Tempo or Grafana.
