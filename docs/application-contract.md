# Application contract

> BaseHarbor should hide operational complexity without hiding standard interfaces.

An application declares what backend capabilities and secret names it requires. BaseHarbor owns provisioning, isolation, readiness and lifecycle. The application should not need to know OpenBao AppRole names, KV v2 mount paths, Compose network names or BaseHarbor internal identifiers.

A second design test is equally important:

> The same application should still be runnable without BaseHarbor when another environment provides the same standard interfaces.

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
BASEHARBOR_BINDINGS=.baseharbor/apps/mailflow/runtime/bindings
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

`--path` prints the protected `application.env` path so an editor, IDE, process manager or normal dotenv loader can consume it directly.

## Lifecycle semantics

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

`baha app status` and `baha app doctor` report readiness metadata only:

```text
REQUIRED SECRET    PRESENT    USABLE
OPENAI_API_KEY     yes        yes
SMTP_PASSWORD      no         no
```

They never reveal secret values.

## Secret delivery remains separate from the requirement contract

The application declares what it needs, not how BaseHarbor internally stores or rotates it.

Runtime delivery may evolve independently and can use standard mechanisms such as:

- in-memory secret files
- environment injection where explicitly appropriate
- workload identity / native OpenBao access
- Kubernetes-native secret projection

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
```

This keeps BaseHarbor useful without making applications proprietary to BaseHarbor.
