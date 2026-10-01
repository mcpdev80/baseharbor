# Application contract

The application contract describes what an application needs.

It stays portable across runtime/provider products.

```yaml
version: 1

app:
  id: 550e8400-e29b-41d4-a716-446655440000
  name: my-app
  environment: dev

services:
  sql:
    enabled: true

  key_value:
    enabled: true

  messaging_queue:
    enabled: true

  document_database:
    enabled: true

secrets:
  required:
    - name: APP_SECRET
```

## Stable application identity

`app.id` is the opaque stable `application_id` owned by the portable repository contract.

BaseHarbor onboarding flows generate it automatically.

Changing the application name, repository path or runtime realization does not change `application_id`.

Application name and environment remain readable selectors/display attributes; they are not durable deployment primary keys.

## Capability intent

The important rule is:

```text
application intent != provider configuration
```

The application may require SQL, cache, durable key-value, document database, queue/pub-sub/stream messaging, object storage, secrets, identity, exposure or telemetry without naming the provider product.

For example:

- `database.document` does not mean MongoDB is encoded in Application Intent;
- `messaging.queue` does not mean RabbitMQ is encoded in Application Intent;
- `database.key-value` remains distinct from reconstructable `cache.key-value` even when both use Valkey as a reference product.

Deployment context, Target selection, provider selection, placement, generated credentials, trust material and runtime-native resource names stay outside portable intent.

For exact fields, see [Manifest reference](../reference/manifest.md). For normative compatibility rules, see [Application Contract v1](../spec/application-contract-v1.md).
