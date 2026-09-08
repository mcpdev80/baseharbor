# Application contract

BaseHarbor should hide operational complexity without hiding standard interfaces.

An application declares what backend capabilities and secret names it requires. BaseHarbor owns provisioning, isolation, readiness and lifecycle. The application should not need to know OpenBao AppRole names, KV v2 mount paths, Compose network names or BaseHarbor internal identifiers.

A second design test is equally important: the same application should still be runnable without BaseHarbor when another environment provides the same standard interfaces.

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

Declaring a required secret automatically enables the managed secrets capability.

## Lifecycle semantics

`plan` includes one read-only requirement action per required secret.

`preflight` validates the contract. When the managed application secret scope already exists it also checks whether each required secret is present and readable. Before the first `apply`, presence is reported as unknown because no secret scope exists yet; preflight does not mutate state merely to answer that question.

The first `apply` may materialize application runtime state and the isolated OpenBao scope. Before starting the workload it evaluates every required secret. Missing or unreadable required secrets stop the lifecycle at that boundary.

This makes the expected bootstrap flow explicit:

```text
baha app apply
    |
    +-- materialize runtime definition
    +-- materialize isolated secret scope
    +-- required secret missing -> STOP before workload start

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

`baha app status` and `baha app doctor` report only secret readiness metadata:

```text
REQUIRED SECRET    PRESENT    USABLE
OPENAI_API_KEY     yes        yes
SMTP_PASSWORD      no         no
```

They never reveal the secret value.

## Delivery is not part of the requirement contract

The application declares what it needs, not how BaseHarbor delivers it.

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
