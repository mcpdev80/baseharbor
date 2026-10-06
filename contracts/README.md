# BaseHarbor machine-readable contracts

Portable contract schemas use JSON Schema 2020-12.

- `service/v1/` — provider-neutral service intent.
- `binding/v1/` — standards-aligned workload connection output.
- `provider/v1/` — provider catalog/distribution metadata.

These schemas are not provider product configuration. Provider-specific configuration uses a separate provider schema referenced by the provider descriptor.

All v0.4 contracts are pre-freeze versioned drafts. See [compatibility policy](../COMPATIBILITY.md) and [public register](../docs/reference/public-contracts.md).

Schema IDs use the controlled repository namespace in ADR 0018. The embedded `contracts.SchemaRegistry` verifies and resolves all packaged schemas offline; consumers pin the repository commit for external acquisition. No obsolete domain aliases are retained.
