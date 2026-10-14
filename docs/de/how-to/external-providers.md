# Externe/BYO-Provider verwenden

BaseHarbor besitzt Bindung und Registrierung, nicht den fremden Dienst. Entfernen einer Registrierung darf externe Datenbanken, Broker oder Identity-Dienste nicht zerstören.

TLS kann System-Vertrauen, eigene CA-Bundles, Zwischenketten, SAN-/Wildcard-Prüfung und erforderliche mTLS-Dateireferenzen verwenden. Private Schlüssel und Trust-Pfade bleiben Operator-/Deployment-Zustand, kein portabler Application Intent.

## Bestehenden SQL-Dienst registrieren

Der Dienst muss vorhanden und erreichbar sein; sein Operator liefert Endpunkt, Trust und geschützte Zugangsdatenreferenz. Ersetze den Beispielhost durch deinen tatsächlichen Dienst:

```text
BaseHarbor owns the binding/reference
BaseHarbor does not own the foreign service
```

Endpunkte müssen absolute URLs ohne Benutzerinformationen, Query-Parameter oder
Fragmente sein. Zugangsdaten und TLS-Material werden über geschützte
Zugangsdaten-/Trust-Referenzen bereitgestellt. Zugangsdatenreferenzen brauchen
eine Ressourcen-URI; Trust-Referenzen dürfen auch Dateipfade sein. Referenzen
dürfen kein Inline-Material, Benutzerinformationen, Query-Parameter oder
Fragmente enthalten. `--trust system` verlangt ein
gültiges System-Trust-Zertifikat, deaktiviert keine TLS-Prüfung. Eigene PKI/Auth
nutzt die expliziten Referenzoptionen aus `baha provider add --help`.

Registrierung migriert keine Daten und wählt den Provider nicht automatisch für jede SQL-Anforderung. Effektive Provider-/Organisationskonfiguration und Application-Plan bestimmen Placement und Auswahl.

Nur die Referenz bewusst entfernen:

```bash
baha provider add company-db \
  --provider-id company/postgresql \
  --kind company-postgresql \
  --capability database.sql \
  --endpoint postgres://db.company.example:5432/orders \
  --trust system
baha provider inspect company-db
baha provider verify company-db
```

Der externe Dienst bleibt Eigentum seines Operators. [Kanonische Anleitung (EN)](https://mcpdev80.github.io/baseharbor/how-to/external-providers/).

Metadaten der Registrierung sind öffentlich. API-/Zugriffsschlüssel, Autorisierungsheader, Passwörter, Tokens und andere Zugangsdaten gehören in geschützte Credential-Referenzen; entsprechende Metadatenschlüssel werden unabhängig von Groß-/Kleinschreibung und Trennzeichen abgewiesen.


## Weitere technische Beispiele

```bash
baha provider remove company-db --yes
```


Technische Kennungen: `baha provider --help`, `baha plan`.

Für externes OIDC bindet die Vorbereitung vertrauliche Eingaben und Trust-Material an ihre aktuellen Dateien. Werden Eingaben vor dem Binding geändert, ersetzt oder umgeleitet, ist ein neuer Plan erforderlich; bestehende Lieferprojektionen bleiben bei diesem Fehler unverändert. HTTPS-Discovery und Workload-Projektion verwenden dieselben erfassten Trust-Bytes. Öffentliche CA-Zertifikate dürfen lesbar bleiben; Schreibrechte für Gruppe oder Andere werden vor Discovery und Lieferung abgelehnt. Vertrauliche Zugangsdaten bleiben nur für den Owner lesbar. Das prüft Materialkonsistenz, nicht Benutzer-Login, Session-Migration oder eine vollständige Providerübergabe.

Für externes OTLP werden Authentifizierungsheader bei der Vorbereitung erfasst und über die geschützten Deployment- und OpenTelemetry-Workload-Umgebungen geliefert. Die Verifikation verwendet dasselbe Material statt Zugangsdaten aus der aktuellen Prozessumgebung. Reapply ohne Header-Eingabe behält die Authentifizierung nur beim selben Empfänger. Ein anderer Empfänger erfordert eine explizite Header-Auswahl; ein explizit leeres `BASEHARBOR_OTLP_HEADERS` wählt anonymen Zugriff. Client-Identitäten des verwalteten Collectors bleiben zur Wiederherstellung erhalten und werden nicht an einen externen Empfänger geliefert. Empfänger-Weiterleitungen dürfen Authentifizierungsdaten nicht weiterreichen. Diese Prüfungen belegen den begrenzten Lieferpfad, nicht Pufferung, alle Signale oder vollständige Aufbewahrung.
