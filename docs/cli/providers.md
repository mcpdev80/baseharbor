# Provider commands

The provider area covers provider authoring and provider lifecycle concepts.

## Provider authoring

```text
baha provider init
baha provider test
```

Provider extensions implement versioned BaseHarbor contracts rather than embedding product names into Application Intent.

## Capability provider model

v0.4.19 includes bundled/reference providers for:

- PostgreSQL — `database.sql`;
- Valkey — `cache.key-value` and `database.key-value` as distinct logical contracts;
- MongoDB — `database.document`;
- RabbitMQ — queue, pub/sub and stream messaging;
- SeaweedFS — `object-storage.s3`;
- OpenBao — secrets;
- Keycloak — OIDC identity;
- observability reference providers.

## External / BYO providers

Existing infrastructure can be registered/referenced without transferring ownership to BaseHarbor.

External provider removal removes the BaseHarbor binding/reference and must not destroy foreign infrastructure.

See [External / BYO providers](../how-to/external-providers.md).
