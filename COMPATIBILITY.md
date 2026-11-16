# Compatibility policy

BaseHarbor v0.4 is pre-freeze. A contract carrying `v1` is a versioned draft,
not a promise that its structure has already been frozen. Compatibility and
migration guarantees begin only when the relevant contract is explicitly
accepted for the v0.5 freeze.

Before that freeze, incorrect interfaces and state formats may be replaced.
No legacy mode, obsolete alias or migration implementation is required.
Each change must still update its producers, consumers, examples and tests
together and preserve authorization, ownership and secret-safety invariants.

## Contract evolution

| Change | Rule |
| --- | --- |
| Add an optional field | Consumers may ignore unknown optional fields; required identity, version and state must still validate. |
| Remove/rename a field, change meaning, requiredness or type | Breaking change; update affected contracts and consumers together. After freeze, use a new major contract version. |
| Add enum/event/operation values | Negotiate support. Unknown state or safety values must fail closed, not become success. |
| Change a result or typed error | Result/error schemas and parity checks change together; actionable cause and safe next step remain required. |
| Change process exit behavior | CLI exit status is a public surface: zero is successful completion, nonzero is failure. Exact mappings belong to the CLI implementation/reference. |
| Deprecate a frozen contract | Publish the replacement, affected versions, removal boundary and tested upgrade/recovery path before removal. No such frozen deprecation promise is active in v0.4. |
| Change persisted state or recovery format | Validate its version and ownership before use. Pre-freeze invalid state may require explicit recreation; never silently reinterpret it. |

Runtime, capability, delivery, workload source, Target Access and extension
versions are independent. A provider product version does not change portable
application intent or authorize an operation.

The [public contract register](docs/reference/public-contracts.md) maps each
surface to its authoritative artifact. The [namespace decision](docs/decisions/0018-public-contract-namespace-and-compatibility.md)
defines schema identity and offline resolution. The [platform matrix](docs/reference/platform-support.md)
separates delivered runtime evidence from compilation and unproven platforms.

Transport/schema conformance alone does not prove live Console or remote
Connector lifecycle support. Those require independent exact-input integration
evidence. A successful fixture test cannot satisfy a runtime requirement.
