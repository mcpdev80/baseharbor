# Application Contract v1

## Scope

This specification defines the stable portable application-intent boundary.

## Requirements

- Portable intent MUST describe application requirements rather than infrastructure product selection.
- Runtime-specific realization details MUST NOT be required in portable intent.
- Provider-specific configuration MUST NOT become portable intent unless it represents a demonstrated portable application semantic.
- Generated credentials and protected deployment state MUST NOT be stored as ordinary portable intent.
- Unsupported required semantics MUST fail before mutation.
- Compatible legacy input MAY be accepted when BaseHarbor can translate it deterministically to the canonical model.

## Identity

`app.name` is the stable logical application identity. Generated runtime/provider names MUST NOT replace it.

## Compatibility

Breaking changes to the public contract require explicit versioning and migration/deprecation behavior.
