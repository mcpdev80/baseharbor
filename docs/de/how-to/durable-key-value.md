# Dauerhafte Key-Value-Daten

`database.key-value/v1` ist für gespeicherte Application-Daten wie Benachrichtigungseinstellungen; `cache.key-value/v1` für wiederaufbaubare Einträge.

| Bedarf | Flag | Go-Bindungen |
| --- | --- | --- |
| Cache | `--cache` | `REDIS_URL`, `REDIS_CA_FILE` |
| Dauerhafte Daten | `--key-value` | `VALKEY_URL`, `VALKEY_CA_FILE` |

In einem übergeordneten Verzeichnis mit `baha` und konfiguriertem Runtime-Target:

```bash
baha app new preferences-api --stack go --http --key-value
cd preferences-api
baha plan
baha up -e dev
baha app env --format json
```

Das Scaffold bereitet den Client `durableKV` vor. URLs, Zugangsdaten und CA kommen aus geschützten Bindungen, nicht aus eingechecktem Application Intent.

Im eigenen Handler mit Request-Kontext `ctx`:

```go
err := durableKV.Set(ctx, "user:42:notifications", "enabled", 0).Err()
```

Null-Ablaufzeit erhält den Eintrag bis zum Löschen. Fehler behandeln und diese Daten in den Recovery-Plan aufnehmen. Valkey als gemeinsames Referenzprodukt macht dauerhafte Daten nicht zu disposable Cache.

Mit `baha doctor` prüfen; danach [Backup/Restore](backup-restore.md). [Kanonische Anleitung (EN)](https://mcpdev80.github.io/baseharbor/how-to/durable-key-value/).
