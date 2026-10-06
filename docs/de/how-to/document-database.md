# Produktdokumente speichern

`database.document/v1` passt zu verschachtelten oder variablen Datensätzen. MongoDB ist Referenzprovider, kein Produktname im portablen Application Intent.

Mit `baha` und konfiguriertem Runtime-Target:

```bash
baha app new catalog-documents --stack go --http --document-db
cd catalog-documents
baha plan
baha up -e dev
baha app env --format json
```

Das Go-Scaffold liefert MongoDB-Treiber, TLS-konfigurierten `mongoClient` und Bindungsnamen `MONGODB_URL`/`MONGODB_CA_FILE`. Normale Ausgabe maskiert die URL-Zugangsdaten.

Für deine Application-Datenbank `databaseName`, Request-Kontext `ctx` und Import `go.mongodb.org/mongo-driver/v2/bson`:

```go
products := mongoClient.Database(databaseName).Collection("products")
_, err := products.InsertOne(ctx, bson.M{
    "sku": "mug-42",
    "name": "Coffee mug",
    "attributes": bson.M{"capacity_ml": 350, "color": "blue"},
})
```

Fehler behandeln; keine fremde Application-Datenbank wählen. Dieses Fachverhalten ergänzt du selbst. `baha doctor` prüft die Capability; dauerhafte Dokumente gehören in [Backup/Restore](backup-restore.md).

[Kanonische Anleitung (EN)](https://mcpdev80.github.io/baseharbor/how-to/document-database/).
