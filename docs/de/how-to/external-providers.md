# Externe/BYO-Provider verwenden

BaseHarbor besitzt Bindung und Registrierung, nicht den fremden Dienst. Entfernen einer Registrierung darf externe Datenbanken, Broker oder Identity-Dienste nicht zerstören.

TLS kann System-Vertrauen, eigene CA-Bundles, Zwischenketten, SAN-/Wildcard-Prüfung und erforderliche mTLS-Dateireferenzen verwenden. Private Schlüssel und Trust-Pfade bleiben Operator-/Deployment-Zustand, kein portabler Application Intent.

## Bestehenden SQL-Dienst registrieren

Der Dienst muss vorhanden und erreichbar sein; sein Operator liefert Endpunkt, Trust und geschützte Zugangsdatenreferenz. Ersetze den Beispielhost durch deinen tatsächlichen Dienst:

```bash
baha provider add company-db --provider-id company/postgresql --kind company-postgresql --capability database.sql --endpoint postgres://db.company.example:5432/orders --trust system
baha provider inspect company-db
baha provider verify company-db
```

Kein Passwort in den Endpunkt einbetten. `--trust system` verlangt ein gültiges System-Trust-Zertifikat, deaktiviert keine TLS-Prüfung. Eigene PKI/Auth nutzt die expliziten Referenzoptionen aus `baha provider add --help`.

Registrierung migriert keine Daten und wählt den Provider nicht automatisch für jede SQL-Anforderung. Effektive Provider-/Organisationskonfiguration und Application-Plan bestimmen Placement und Auswahl.

Nur die Referenz bewusst entfernen:

```bash
baha provider remove company-db --yes
```

Der externe Dienst bleibt Eigentum seines Operators. [Kanonische Anleitung (EN)](https://mcpdev80.github.io/baseharbor/how-to/external-providers/).
