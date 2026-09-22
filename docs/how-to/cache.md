# Use cache

Declare the logical cache/key-value capability and let BaseHarbor resolve the provider for the selected environment.

Applications should use standard provider protocols and bindings rather than provider-specific BaseHarbor APIs.

Normal flow:

```bash
baha plan
baha up -e dev
baha doctor
```

Provider placement may be application-scoped, shared or external. That choice is deployment state, not business logic.

For exact fields, see [Manifest reference](../reference/manifest.md).
