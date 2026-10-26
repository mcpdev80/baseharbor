# Store durable key-value data

Use `database.key-value/v1` for application data such as saved notification preferences. Use `cache.key-value/v1` for entries that can be rebuilt.

| Requirement | Capability flag | Generated Go bindings |
| --- | --- | --- |
| Reconstructable product cache | `--cache` | `REDIS_URL`, `REDIS_CA_FILE` |
| Saved user preferences | `--key-value` | `VALKEY_URL`, `VALKEY_CA_FILE` |

## Create a preferences service

With `baha` installed, run from a parent directory:

```bash
baha app new preferences-api --stack go --http --key-value
cd preferences-api
baha plan
baha up -e dev
baha app env --format json
```

`up` requires a configured local runtime Target. The scaffold declares durable key-value intent and prepares a `go-redis` client named `durableKV`. Protected bindings provide the URL and CA file; do not copy credentials into `baseharbor.yaml`.

For a persistent preference, add application code using your request context `ctx`:

```go
err := durableKV.Set(ctx, "user:42:notifications", "enabled", 0).Err()
```

A zero expiration preserves the entry until the application deletes it. Unlike a cache entry, this value belongs in the application's recovery plan. Verify readiness with `baha doctor`, then follow [backup and restore](backup-restore.md) before relying on recovery.

Valkey is the reference provider for both key-value contracts. Sharing a provider product does not make durable records disposable cache data.
