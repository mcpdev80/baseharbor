# Manifest reference

The application manifest is portable application intent.

## Top-level rule

Only application requirements belong here. Runtime/provider implementation details, generated credentials and protected deployment state do not.

## Core fields

| Field | Meaning |
|---|---|
| `version` | application contract version |
| `app.name` | stable logical application identity |
| `app.environment` | compatibility field where supported; deployment selection is otherwise external |
| `services` | declared application capability needs |
| `secrets.required` | required logical secret names |

Capability-specific exact fields are defined by the versioned schema/types and their normative capability specifications.

The schema/types are authoritative for structure. Markdown explains intent and compatibility but must not become a second manually maintained schema.
