# BaseHarbor machine-readable contracts

Machine HTTP semantic envelopes are shipped in `machine/v1/control.schema.json`,
with Core-generated synthetic examples in `machine/v1/control.golden.json`.
These cover discovery, execution requests, execution state, events and typed
error results. Consumers pin immutable public Core commits and content digests;
fixture agreement is a source check, never runtime, authentication or release
evidence.

Portable contract schemas use JSON Schema 2020-12.

- `service/v1/` — provider-neutral service intent.
- `binding/v1/` — standards-aligned workload connection output.
- `provider/v1/` — provider catalog/distribution metadata.

These schemas are not provider product configuration. Provider-specific configuration uses a separate provider schema referenced by the provider descriptor.

All v0.4 contracts are pre-freeze versioned drafts. See [compatibility policy](../COMPATIBILITY.md) and [public register](../docs/reference/public-contracts.md).

Schema IDs use the controlled repository namespace in ADR 0018. The embedded `contracts.SchemaRegistry` verifies and resolves all packaged schemas offline; consumers pin the repository commit for external acquisition. No obsolete domain aliases are retained.

Navigation and Runtime Explorer result shapes are generated from actual Core
semantic types in `machine/v1/read-models.schema.json`; synthetic examples are
in `machine/v1/read-models.golden.json`. Regenerate with
`go run ./scripts/tools/machine-read-models -schema` and without `-schema` for
examples. Drift tests compare both packaged artifacts to the actual types.
`runtime.list` permits `null` for an empty Core inventory. Target configuration
is not a connectivity/health observation; consumers must not invent these fields.

The same generated read-model artifact also includes Runtime Explorer capability
records, authenticated stream descriptors and actual Core terminal event/input
examples. Terminal binary data uses standard JSON base64. The dedicated shipped
terminal schemas remain the normative frame/bounds contract; generated example
regressions validate against both their actual Core types and these schemas.
