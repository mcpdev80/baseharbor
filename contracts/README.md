# BaseHarbor machine-readable contracts

Portable contract schemas use JSON Schema 2020-12.

- `service/v1/` — provider-neutral service intent.
- `binding/v1/` — standards-aligned workload connection output.
- `provider/v1/` — provider catalog/distribution metadata.

These schemas are not provider product configuration. Provider-specific configuration uses a separate provider schema referenced by the provider descriptor.

Existing shipped v0.4 capability IDs remain compatible until their migration to the standards-first service model is explicitly accepted and tested.
