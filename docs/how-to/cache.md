# Cache product lookups

Use a reconstructable cache for frequently requested product data. The database remains the source of truth; an empty cache must only slow the application down.

## Create a Go API with cache bindings

From a parent directory with no `catalog-api` folder, with `baha` installed:

```bash
baha app new catalog-api --stack go --http --cache
cd catalog-api
baha plan
baha up -e dev
baha app env --format json
```

A configured local Docker or Podman Target is required for `up`. The generated `.env.example` contains `REDIS_URL` and `REDIS_CA_FILE`; BaseHarbor supplies their protected runtime values. The scaffold uses `go-redis` and verifies connectivity at startup. Normal `app env` output masks credentials.

## Add cache behavior

In the generated Go application, use the initialized `cache` client for a short-lived product entry:

```go
err := cache.Set(ctx, "product:42", `{"name":"Coffee mug"}`, 5*time.Minute).Err()
```

Here `ctx` is your request context. Read with `cache.Get(ctx, "product:42")`; on a miss, load the product from its authoritative store and repopulate the cache. This application logic is yours to add; the scaffold does not implement a product catalog.

Run `baha doctor` to verify the cache capability after deployment. For data that must survive recovery, use [durable key-value](durable-key-value.md) instead.
