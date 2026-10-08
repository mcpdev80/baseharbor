# PostgreSQL für eine Order-API

Mit installiertem `baha` in einem übergeordneten Verzeichnis ohne `orders-api`:

```bash
baha app new orders-api --stack go --http --sql
cd orders-api
baha plan
baha up -e dev
baha app env --format json
```

`up` benötigt ein konfiguriertes Docker-/Podman-Target. Das Go-Scaffold ergänzt `pgx`, deklariert `DATABASE_URL`/`DATABASE_CA_FILE` und prüft die Verbindung beim Start. Laufzeitzugangsdaten kommen aus geschützten Bindungen; normale Env-Ausgabe maskiert sie.

## Eigene Daten abfragen

Nach erfolgreichem Deployment:

```bash
baha app psql
```

In dieser PostgreSQL-Sitzung:

```sql
CREATE TABLE IF NOT EXISTS orders (id integer PRIMARY KEY, total_cents integer NOT NULL);
INSERT INTO orders VALUES (42, 1990) ON CONFLICT (id) DO NOTHING;
SELECT id, total_cents FROM orders WHERE id = 42;
```

Mit `\q` beenden, anschließend `baha doctor`. Das prüft Application-Zugriff, keine Provider-Admin-Zugangsdaten. Ein bestehendes Repository wird mit Inspect/Init übernommen statt mit einem zweiten Scaffold überschrieben.

## Isolation

Shared PostgreSQL ist ein Target-eigener Provider mit getrennten Application-Datenbanken, Rollen und Zugangsdaten. Die Identität berücksichtigt Application, Umgebung und SQL-Instanz. Öffentlicher Datenbank-/Schema-Zugriff wird eingeschränkt und Cross-App-Zugriff negativ geprüft. `baseharbor_admin` bleibt ausschließlich beim Provider-Lifecycle.

Für bewusst dedizierten Placement existiert `BASEHARBOR_PROVIDER_POSTGRESQL_SCOPE=application`. Zusätzliche Instanzen können mehr Ressourcen benötigen; die Core-Capability bleibt verpflichtend.

Weiter: [Backup/Restore](backup-restore.md), [exakte SQL-Erklärung (EN)](https://mcpdev80.github.io/baseharbor/how-to/postgres/).
