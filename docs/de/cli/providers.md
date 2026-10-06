# Provider-Befehle

Provider-Erweiterungen implementieren versionierte Verträge. Produktnamen werden nicht Bestandteil von Application Intent.

Referenzprodukte sind PostgreSQL für SQL, Valkey für getrennte Cache-/dauerhafte Key-Value-Verträge, MongoDB für Dokumentdatenbanken, RabbitMQ für Messaging, SeaweedFS für S3, OpenBao für Secrets, Keycloak für OIDC und weitere Observability-Provider.

## Erweiterung erzeugen und prüfen

In einem Verzeichnis ohne `company-sql-provider`:

```bash
baha provider init company/sql --path ./company-sql-provider
baha provider test ./company-sql-provider -o json
```

Das Scaffold erzeugt Deskriptor und Implementierungsskelett. Vertrag und Operationen müssen implementiert sein, bevor der Provider produktionsbereit ist. Vertragsprüfungen beweisen keine Erreichbarkeit oder Berechtigung einer beliebigen Datenbank.

Nach Registrierung eines vorhandenen Dienstes:

```bash
baha provider list
baha provider inspect company-db
baha provider verify company-db
```

Externe Registrierung überträgt keinen Infrastruktur-Besitz. Entfernen löst nur die Bindung/Referenz.

Weiter: [Provider-Modell](../explanation/providers.md), [externe Provider](../how-to/external-providers.md), [exakte Befehle (EN)](https://mcpdev80.github.io/baseharbor/cli/providers/).
