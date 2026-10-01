# Durable key-value databases

`database.key-value/v1` is a durable data capability.

It is intentionally distinct from `cache.key-value/v1`.

| Capability | Primary meaning |
| --- | --- |
| `cache.key-value` | Reconstructable cache/state acceleration |
| `database.key-value` | Durable application data |

Valkey is the v0.4.19 reference provider for both contracts, but provider product reuse does not merge the semantics.

Durable key-value instances participate in normal provider placement, binding, readiness, ownership and recovery classification.
