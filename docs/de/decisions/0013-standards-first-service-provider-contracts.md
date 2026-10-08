# ADR 0013: Standard-First-Service- und Anbieterverträge

> Kompatibilitätsstatus: Voreinfrieren Legacy/Migration-Preservation Wortlaut unten wird ersetzt durch[ADR 0018](0018-public-contract-namespace-and-compatibility.md). Die historische Begründung bleibt erhalten; v0.4 trägt keine Vermächtnis- oder Migrationspflicht.

Status: angenommen

Geburtsdatum: 2026-09-23

## Kontext

BaseHarbor verwendet bereits mehrere offene Standards und ausgereifte Ökosystemmuster, aber einige Fähigkeiten und verbindliche Semantiken entwickelten sich lokal, bevor die Service/Provider-Grenze vollständig eingefroren wurde.

Die v0.5.x-Linie ist die Contract-Freeze-Strecke. Die Architektur braucht daher eine explizite Regel, um zu entscheiden, wann BaseHarbor einen bestehenden Standard übernimmt und wann sie BaseHarbor-spezifische Semantik definiert.

## Entscheidung

BaseHarbor ist der erste Standard.

- Gegründete offene Standards werden gegenüber eigenen Verträgen bevorzugt.
- Etablierte De-facto-Standards und Ökosystemkonventionen werden bevorzugt, wenn kein geeigneter formaler Standard existiert.
- BaseHarbor-spezifische Verträge definieren nur Semantik, die nicht unter eine bestehende Norm fällt.
- BaseHarbor-Erweiterungen sind explizit, versioniert und providerneutral.
- Anbieterspezifische Verträge bleiben hinter der Anbietergrenze.

Die ursprünglich angenommene Basis ist:

- JSON Schema 2020-12 für tragbare Schemata und Providerkonfiguration;
- Service-Bindung Spezifikation 1.1 bekannte Namen für Service-Anschluss-Ausgänge;
- Crossplane-Ressource/Provider/Reconciliation-Konzepte als Architekturleitfaden;
- gegebenenfalls Konzepte für den Lebenszyklus von Open Service Broker;
- gRPC/Protokollpuffer für externe Provider-Prozessgrenzen;
- OCI für die Verteilung von Artefakten und die Verdauung von Identität;
- OpenTelemetrie/OTLP für Beobachtungsdaten;
- OIDC/OAuth für die Identität;
- gegebenenfalls AsyncAPI und CloudEvents für Messaging/Event-Kontrakte;
- RESP als Kompatibilitätseigenschaft von Redis/Valkey-ähnlichen Cache-Anbietern;
- S3 API-Kompatibilität als Eigenschaft von Objekt-Speicher-Providern.

Service-Vertrag, Provider-Protokoll, Provider-Implementierungsversion, Produkt/Motor-Version und Artefakt-Verdauung sind separate Kompatibilitätsachsen.

Bestehende v0.4-Fähigkeits-IDs bleiben solange unterstützt, bis eine explizite Migration dokumentiert und getestet wird. Standards-First Alignment ist keine Berechtigung, umbenennen zu brechen.

## Folgen

- Service-Arten bleiben produktneutral.
- Protokoll-Kompatibilität wird nicht zur Dienstleistungsidentität.
- Laufzeit-Provider-Instance-Zustand bleibt getrennt von Provider-Katalog/Verteilung Metadaten.
- Neue Service-Arten/Anbieter erfordern vor der Umsetzung eine Norm-Audit.
- Die Konformität kann angenommene Standards getrennt von BaseHarbor-Erweiterungen testen.
- Zukünftige Provider-Repositorien können ohne Änderung der Anwendungsabsicht extrahiert werden.
