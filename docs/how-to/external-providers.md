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

## Example: register an existing company SQL endpoint

Prerequisites: the service already exists, your deployment can reach it, and its operator has supplied the endpoint, trust material and any required protected credential reference. The hostname below is illustrative; replace it with your actual service:

```bash
baha provider add company-db \
  --provider-id company/postgresql \
  --kind company-postgresql \
  --capability database.sql \
  --endpoint postgres://db.company.example:5432/orders \
  --trust system
baha provider inspect company-db
baha provider verify company-db
```

Do not embed passwords in the endpoint. `--trust system` requires the endpoint's certificate to validate with system trust; it does not bypass TLS verification. Use the explicit trust/credential reference options from `baha provider add --help` when your company PKI or service authentication needs them.

Registration records the external provider and verification checks endpoint/trust. It does not migrate application data or automatically select this provider for every SQL request: configure placement through your effective provider/organization configuration and inspect `baha plan` in the application repository.

To remove only this registration when it is no longer needed:

```bash
baha provider remove company-db --yes
```

The externally operated database remains owned by its operator.
