# Standardmatrix

Diese Matrix ist die erste Basisbasis für v0.5-Standards. Sie erfasst, was BaseHarbor annimmt, was noch eine BaseHarbor-Erweiterung bleibt und wo bestehende v0.4-Verträge eine Angleichung erfordern, anstatt blinden Austausch.

Bereich: Standard / Muster Aktuelle Version / Status: Klasse: Governance / Lizenz: Reife Direkte BaseHarbor-Nutzung: Gap / BaseHarbor-Erweiterung: Aktuelle Aktion:
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
Schema-Schema-Schema-Strategie 2020-12-Standard öffnen JSON-Schema-Community; offene Spezifikation sehr hoch Portable Service-Schemas und Provider-Konfigurationsvalidierung Lebenszyklus, Eigentümer und Provider-Ausführung sind außerhalb des Schema-Scope-Bereichs. **ADOPT**-
Service-Anbindung: Service-Bindung: Spezifikation: 1.1.0. Öffne Spezifikation: servicebinding.io community: Apache-2.0. reif in Cloud-native Ökosysteme: Bekannte Bindungsnamen:`type `, ` provider `, ` host `, ` port `, ` uri `, ` username `, ` password `, ` certificates `, ` private-key`• Identity Refs, Autorisierung, Rotation/Revokation, Eigentum und Verifizierung von Metadaten, **ALIGN**
Provider/Resourcen-Architektur-Modell: Crossplane ressource/provider/reconciliation model: v2.4 docs line: Establiertes Architekturmuster: CNCF-Ökosystem; Apache-2.0-Projekt: High-Beobachteter Zustand, Provider-Abstraktion, externe Realisierungs- und Aussöhnungskonzepte: Importieren Sie keine Kubernetes CRDs/finalizers/namespaces in portable Intent **KEEP / ALIGN**
Open Service Broker API-Datei v2.17 familie • Open API / Architekturmuster • Open Service Broker community • Apache-2.0 · Reifer, aber schmaler als BaseHarbor · Katalog, Plan, Bereitstellung, Update, Bind, Entbind, Deprovision und Async-Operation Konzepte • BaseHarbor fügt Versöhnung, Platzierung, Eigentum, Verifikation, Backup/Restore und Laufzeit-Separierung hinzu **KEEP / ALIGN**
Provider-Distribution: OCI Bild / Distribution / Laufzeit: Bild 1.1.1, Distribution 1.1.1, Laufzeit: 1.3.0 standard öffnen: Open Container Initiative / Linux Foundation; Apache-2.0 , sehr hohe Provider-Artefakte, Digest-Identität, Indexe/Plattformen, registry-neutrale Distribution: BaseHarbor Katalog-Metadaten und Kompatibilitätsregeln: **KEEP** .
Beobachtbarkeit: OpenTelemetry / OTLP-OTel 1.61.0, OTLP 1.11.0-Standard öffnen: CNCF; Apache-2.0-Sehr hohe Spuren, Metriken, Protokolle Transport und semantische Konventionen.Nur BaseHarbor-Intent/Policy/Provider-Auflösung **KEEP / ALIGN**
OpenID Connect / OAuth.OIDC Core 1.0 Errata 2- standard öffnen OpenID Foundation- sehr hohe Entdeckung, Emittent, Scopes/Claims, Standard-Authentifizierung/Token-Semantik, BaseHarbor Autorisierung/Politik und Provider-Lebenszyklus. **ADOPT**
Beschreibung des Messaging-Verfahrens: AsyncAPI.3.1.0. Öffnen Sie den Standard: AsyncAPI-Initiative; Apache-2.0. Hochkanal/Operation/Message API-Beschreibung: Broker Provisioning, Placement und Lifecycle. **ADOPT, wenn Messaging-Schiffe**.
Event-Hüllkurve: Event-Hüllkurve: CloudEvents: 1.0.2 kompatibel mit 1.0. Offene Norm: CNCF; Apache-2.0. High-Event-Hüllkurve: Event-Semantik: Broker-spezifische Lieferung: Semantik: **ADOPT, falls zutreffend**.
.Cache-Protokoll . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . .
Objektspeicher-API-API-API-API-API-API-Familie mit aktueller AWS-S3-API-Familie, De-facto API-Standard, AWS-definiert, weit verbreitet, sehr hohe Kompatibilitäts-Eigenschaft für S3-kompatible Anbieter, Generische Objektspeicher-Service-Semantik und Nicht-S3-Anbieter, **ALIGN**
Kein akzeptierter neutraler Servicevertrag Kein ausreichender Standard Nicht fragmentiertes Ökosystem Mittel / fragmentiert Wiederverwendung generischer Schema / verbindliche Standards wo möglich Minimal BaseHarbor Vektor Semantik nur **BAHA EXTENSION**

## Analyse des Dienstleistungsgefälles in der Familie

. . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . .
| --- | --- | --- | --- | --- |
| `sql` Service Binding; Provider/Ressourcenmuster; Enginekonventionen; logische Datenbankressource, Lebenszyklus, Provider-Auflösung; enginspezifische Felder bewegen sich hinter dem Provider; minimum portable SQL Semantik und erforderliches Feature-Subset.
| `cache` Service Binding; RESP für kompatible Anbieter; generische Cache-Intent; Beenden Sie die Behandlung von Redis/Valkey-Namen als generische Service-Identität.
| `object-storage ` Service-Bindung; S3 als de-facto API. logisches Objekt Ressource / Bucket Semantik.`object-storage.s3/v1` wird Kompatibility-mapped, nicht universelle Objekt-Speicher-Identität-Provider-neutrale Objekt-Speicher-Semantik-Betrachtung-
| `secrets` Service-Bindung für geliefertes Verbindungsmaterial Erforderliche geheime Namen, undurchsichtige Refs, Lebenszyklus/Sicherheitsmodell, ggf. Standard-Ausgabenamen. Rotation/Widerruf/Autorisierung/Referenzsemantik.
| `messaging` AsyncAPI, CloudEvents; Provider-Protokolle wie AMQP/MQTT/Kafka-Generische Messaging-Intent-Standards von der ersten Implementierung verwenden; Lebenszyklus, Platzierung und erforderliche semantische Subsets.
| `vector` Kein kompletter neutraler Vertrag, nichts produktspezifisches, Audit APIs vor dem Hinzufügen von Feldern, kleinste nachgewiesene gemeinsame Semantik.
| `observability`• OpenTelemetrie/OTLP-Intention, Richtlinie, Provider/Backend-Auswahl • Vermeidung paralleler Baha-Telemetriedrahtsemantik • Platzierung/Eigentum/Verifikation •
| `identity` OIDC/OAuth-Provider-Auswahl und -Richtlinie keine proprietäre Login/Token-Semantik. BaseHarbor-Lebenszyklus-/Autorisierungspolitik.

## Aktuelle v0.4 Klassifikation

### BEWAHREN

- Anbieterneutrale Anwendungsabsicht;
- Lebenszyklus des Vertrags über die Integration von Anbietern;
- gRPC/Protocol Buffers Prozessgrenze;
- OCI-Verteilungsrichtung,
- gewünschtes/beobachtetes/rekonziles/verifiziertes Modell;
- Vermittlung und Eigentum des Anbieters;
- die Konformität des Anbieters;
- `telemetry.otlp/v1` Verwendung von Standard-OpenTelemetrie-Variablen.

### ALIGN

- `secure-binding/v1`: BaseHarbor Security / Lifecycle-Erweiterungen zu halten, verwenden Service Binding 1.1 Namen für Standard-Verbindung Ausgänge;
- `database.sql/v1 `: Semantik verschickt halten, Karte zu Service Art` sql`;
- `cache.key-value/v1 `: Semantik verschickt halten, Karte zu Service Art` cache`, bei Bedarf getrennt RESP melden;
- `object-storage.s3/v1 `: ausgelieferte v0.4 Kompatibilität, Karte an` object-storage`zuzüglich S3-Kompatibilität;
- `metrics/v1 `, ` logs/v1 `, ` traces/v1`: halten Anwendung / Plattform Absicht, wo nützlich, aber OpenTelemetrie bleibt maßgeblich für Telemetrie Daten Semantik.

### ERLÄUTERUNG

Kein ausgelieferter v0.4 Vertrag ist derzeit für Blindersatz geplant. Jeder Ersatz erfordert eine explizite Kompatibilitätsmigration.

### BAHA ERWEITERUNG

- Anbieterplatzierung und Lebenszyklus-Eigentum;
- Aussöhnung/Verifikationssemantik;
- Anbieterkatalog-Metadaten über OCI;
- Metadaten über den Sicherheitslebenszyklus, die nicht von der Dienstbindung erfasst werden;
- minimale Vektor-Service-Semantik, wenn/wenn implementiert.

## Anforderungen an die Einfrierung

Vor dem Einfrieren des v0.5.x-Vertrags:

1. jeder eingefrorene Dienstleistungsauftrag hat JSON Schema 2020-12;
2. Standard-Service Binding-Namen sind kanonisch für Anschlussausgänge;
3. Service-Art, Protokoll-Kompatibilität, Provider-Implementierung und Produkt-Version unterscheiden sich;
4. Die Metadaten des Providerkatalogs unterscheiden sich vom Status der Laufzeitanbieter-Instanz;
5. vorhandene v0.4 Fähigkeits-IDs haben Kompatibilitäts-Mappings dokumentiert;
6. Konformitätsprüfungen validieren angenommene Normen und BaseHarbor-Erweiterungen getrennt;
7. Es ist kein anbieterspezifischer Produktvertrag durch portable Anwendungsabsicht erforderlich.
