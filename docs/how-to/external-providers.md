# Use External / BYO providers

External/BYO providers let BaseHarbor bind existing infrastructure without claiming ownership of it.

## Ownership rule

```text
BaseHarbor owns the binding/reference
BaseHarbor does not own the foreign service
```

Removing the BaseHarbor provider registration must not destroy the external database, broker, identity service or other foreign infrastructure.

## Supported v0.4.19 direction

External placement is supported across the provider model, including representative data-provider paths such as PostgreSQL, Valkey, RabbitMQ and MongoDB.

TLS/trust configuration can use:

- system trust;
- custom CA/root bundles;
- intermediate chains;
- SAN/wildcard certificate validation;
- mTLS client certificate/key references where required;
- directory-based certificate discovery when unambiguous.

Private-key material and trust paths remain operator/deployment state, not portable Application Intent.

Use `baha provider --help` and the effective provider/organization configuration commands to inspect the exact current command surface.
