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

Für externes OTLP wählt `BASEHARBOR_PROVIDER_EXTERNAL_OTLP_EXTERNAL_REFERENCE` einen registrierten Verweis. Ein Alias wird zum tatsächlichen HTTPS-Endpunkt ohne Zugangsdaten aufgelöst; ein widersprüchliches `BASEHARBOR_OTLP_ENDPOINT` wird abgelehnt. Ein geschützter JSON-Credential-Verweis liefert entweder ein Token oder ein Benutzername/Passwort-Paar. Explizite Header müssen mit der Registrierung übereinstimmen. Ohne Credential-Verweis wählen explizite Header die Authentifizierung; ein explizit leeres `BASEHARBOR_OTLP_HEADERS` wählt anonymen Zugriff. Weggelassene Header dürfen geschützte Zugangsdaten nur für dieselbe angewendete Registrierung und denselben Empfänger wiederverwenden. Eine neue Registrierung erbt keine Zugangsdaten einer anderen Registrierung.

Explizite CA- und Client-Zertifikat/Schlüssel-Verweise liefern Empfängervertrauen und optionale mTLS-Identität. Credential- und Schlüsselquellen bleiben privat, stabil und unverändert; öffentliches Vertrauensmaterial darf ausschließlich Zertifikate enthalten. Zertifikatsverzeichnisse erfordern derzeit explizite unterstützte Dateiverweise. Der Adapter prüft Eigentum und Bytes der vorbereiteten Quellen vor der Lieferung sowie vor/nach der tatsächlichen Verifikation, vergleicht geschützte Runtime-, Host- und Workload-Lieferungen und exportiert einen echten Protobuf-Trace mit ausgewählter Authentifizierung und TLS-Identität. Weiterleitungen dürfen keine Zugangsdaten weiterreichen. Verwaltete Collector-Identitäten bleiben zur Wiederherstellung erhalten und werden nicht implizit an externe Empfänger geliefert. Dies belegt weder Pufferung, jedes Signal, Materiallieferung an Remote-Hosts noch vollständige Aufbewahrung.

Wird ein bestehender externer OTLP-Empfänger oder seine ausgewählte Authentifizierung, sein Vertrauensmaterial oder seine Client-Identität geändert, entsteht auch bei gleicher registrierter Provider-Instanz-ID ein gemeinsamer Folgenplan. Unverifizierte Liefer- oder Aufbewahrungsfolgen zeigen **Achtung** und erfordern zwei aktuelle, an den Plan gebundene Bestätigungen. Geändertes Material macht frühere Zustimmung ungültig. Technische Material-/HTTPS-Prüfungen folgen nach diesen Bestätigungen vor jeder Mutation und bewahren bei Fehlern das bisherige Binding. Diese Zustimmung erlaubt keine Datenlöschung.

Die OIDC-Lieferung an Anwendungen löst die ausgewählte registrierte Referenz zum tatsächlichen HTTPS-Issuer auf. Die Registration-ID wird mit `BASEHARBOR_PROVIDER_EXTERNAL_OIDC_EXTERNAL_REFERENCE` gewählt und muss keine Issuer-URL sein. Die vorhandene globale Registrierung wird rein lesend in die kanonische Target-Registry projiziert und erst durch normales Apply gespeichert; fremde oder umgeleitete Targets können sie nicht übernehmen. Die registrierten `client-id`-Metadaten, eine lokale `file://`-Credential-Referenz und eine CA-Dateireferenz liefern das Material für den unterstützten Adapter. Widersprüchliche Prozess-Eingaben, nicht unterstützte Credential-Stores, mTLS oder Zertifikatsverzeichnis-Lieferung scheitern nach Folgenbestätigung an der technischen Zulassung. Registrierte Referenzen werden vor und nach Discovery erneut geprüft. Discovery lehnt eingebettete URL-Zugangsdaten und Credential-Query-Parameter ab, ohne deren Werte auszugeben. Issuer-/Client-/Credential-/Trust-Wechsel und die Freigabe einer externen App-Referenz erfordern die aktuelle Folgenbestätigung; fremde Infrastruktur und ursprüngliche Credential-Dateien bleiben erhalten. HTTPS-Quelltests authentifizieren den gelieferten Client bei getrennten Quell-/Zielempfängern und belegen keine Browser-Anmeldung, OAuth-Session-Migration oder native Abnahme aller Provider.

Der gemeinsame Credential-/Trust-Lesepfad akzeptiert unterstützte absolute lokale Dateien und `file://`-Referenzen. Zugangsdaten und private Schlüssel benötigen eine ausschließlich eigene private Datei; öffentliche CA-Dateien dürfen lesbar, aber nicht durch Gruppe oder andere beschreibbar sein. Verknüpfte Dateien/Elternpfade, übergroße Eingaben und geänderte Lesevorgänge scheitern vor der Nutzung, ohne abgelehnte Werte auszugeben oder Quelldateien zu verändern. Die Zertifikatsverzeichnis-Erkennung prüft dieselben Regeln; der mTLS-Verifier verarbeitet die erfassten geschützten Zertifikats-/Schlüsselbytes. Die OIDC-Registrierungsprüfung verlangt den exakten Issuer und gültige HTTPS-Discovery-Endpunkte, lehnt Endpunkte mit Zugangsdaten ab, begrenzt die Antwort und folgt keinen Weiterleitungen. Erfolgreiche Discovery belegt keine Client-Authentifizierung oder Session-Fortführung.

Verwaltete Keycloak-Platzierungswechsel und Binding-Wechsel zwischen verwaltetem und externem OIDC verwenden nach aktueller Folgenbestätigung die vorhandenen Liefer- und Retirement-Adapter. Ursprüngliche Realms, Benutzer, Datenbank-Volumes und Credential-Eigentümer bleiben erhalten; Zielkonten und Sessions werden nicht automatisch migriert. Retirement entfernt nur Identity-Routen der alten Realisierung und bewahrt bereits verifizierte Ziel- und unabhängige Workload-Routen. Diese Quellprüfungen und begrenzten nativen Fixtures schließen das Provider-Paket nicht ab.
