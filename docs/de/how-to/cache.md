# Produktdaten cachen

Cache-Daten müssen aus ihrer maßgeblichen Datenquelle rekonstruierbar sein. Ein leerer Cache darf die Anwendung verlangsamen, keine Geschäftsdaten verlieren.

Mit installiertem `baha`, ohne vorhandenen Ordner `catalog-api`:

```bash
baha app new catalog-api --stack go --http --cache
cd catalog-api
baha plan
baha up -e dev
baha app env --format json
```

Ein konfiguriertes Runtime-Target ist Voraussetzung für `up`. Das Scaffold ergänzt `go-redis`, prüft Verbindung und verwendet `REDIS_URL`/`REDIS_CA_FILE` aus geschützten Runtime-Bindungen. Normale Ausgabe maskiert Zugangsdaten.

In deinem Handler mit Request-Kontext `ctx` und initialisiertem Client `cache`:

```go
err := cache.Set(ctx, "product:42", `{"name":"Coffee mug"}`, 5*time.Minute).Err()
```

Fehler prüfen. Bei `cache.Get`-Miss aus der autoritativen Datenquelle laden und den Cache auffüllen. Fachliche Endpunkte erzeugt das Scaffold nicht. `baha doctor` prüft die Capability.

Für nicht rekonstruierbare Daten verwende [dauerhaften Key-Value-Speicher](durable-key-value.md). [Kanonisches Beispiel (EN)](https://mcpdev80.github.io/baseharbor/how-to/cache/).
