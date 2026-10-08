# Entwicklungsverlängerungsvertrag v1

## Anwendungsbereich

Diese Spezifikation definiert BaseHarbor Authoring-Time Entwicklungserweiterungen:

- Stack-Profile;
- Entwicklungspläne;
- Entwicklungsadapter.

Entwicklungserweiterungen erstellen oder validieren ökosystem-native Anwendungsquelle. Sie stellen keine Provider-Infrastruktur zur Verfügung und werden nicht zu Runtime-Provider-Instanzen.

```text
application requirement
!= development integration
!= capability provider
!= placement
!= runtime
!= delivery
```

## Bestehende Normen

BaseHarbor verwendet bestehende Formate und Ökosystemkonventionen, anstatt einen Rahmen zu definieren:

Standard / Konvention
| --- | --- |
Schema-Validierung, JSON-Schema 2020-12
Erweiterung Artifact-Distribution. OCI Bild / Distribution.
Gehe Abhängigkeiten, gehe Module, gehe zu!
JavaScript / Next.js Abhängigkeiten.json / npm ökosystem.
Python-Abhängigkeiten, pyproject.toml / Python-Verpackungskonventionen
Quarks-Abhängigkeiten) Maven / Quarks-Erweiterungskonventionen
• Telemetrie • Offene Telemetrie • OTLP
Provider-Verbindungs-Ausgangs-Service-Verbindlichkeits-Spezifikation, falls zutreffend
Portalprojektion - Backstage-Katalog `Component` Metadaten, optional und statisch

Devfile-Konzepte können Interoperabilität informieren, aber Devfile ist nicht der kanonische BaseHarbor-Entwicklungsplan und keine Core-Abhängigkeit.

## Angenommene BaseHarbor-Verträge

maschinenlesbare Behörde:

- `contracts/development/v1/stack-profile.schema.json`
- `contracts/development/v1/development-plan.schema.json`
- `contracts/extension/v1/extension-descriptor.schema.json`

### Stapelprofil

Ein Stack-Profil erstellt Metadaten.

Es MUSS NICHT zu portablen Anwendungsabsichten, Bereitstellungszustand, Laufzeitzustand oder Provider Runtime Registry-Zustand werden.

Ein Profil enthält stabile Entwicklungskomponenten und optionale Funktionseinstellungen. Mehrere Komponenten werden von v1 unterstützt.

Die Zusammensetzung ist deterministisch:

1. referenzierte Stammprofile werden in deklarierter Reihenfolge aufgelöst;
2. identische Stammwerte können zusammengeführt werden;
3. kollidierende Mutterwerte scheitern geschlossen;
4. das Kinderprofil kann die vererbten Werte explizit überschreiben;
5. die Zyklen sind geschlossen;
6. das effektive Profil hat deterministische Komponente und Fähigkeit Ordnung.

Eine Implementierungspräferenz bleibt eine Vorliebe. Sie MUSS im portablen Bewerbungsvertrag NICHT stillschweigend zur Anbieter-/Produktanforderung werden.

### Entwicklungsplan

Der Entwicklungsplan ist ein Authoring-Time-Plan und unterscheidet sich vom Runtime/Deployment-Plan.

Es kann Anforderungen an die Abhängigkeit von Ökosystemen, anwendungsverbindliche Erwartungen, Quell-/Konfigurationsaktionen, Erstellung von Konventionen, Gesundheitskonventionen und Telemetrieintegrationsaktionen enthalten.

Der Plan MUSS deterministisch für den gleichen Application Contract, effektive Stack Profile und Adapter-Versionen sein.

### Entwicklungsadapter

Ein Entwicklungsadapter MUSS diese Semantik aufdecken:

```text
Descriptor
Detect
Supports
Plan
Bootstrap
Validate
```

Ein Adapter MÜSST KEINE Infrastruktur für die Bereitstellung von Fähigkeiten bereitstellen, die Auflösung des Anbieters in der Erzeugung von Anwendungsquellen verbergen, eine erforderliche BaseHarbor-Anwendung SDK einführen oder ein Laufzeitprodukt in portabler Applikationsabsicht benötigen.

Generierte Anwendungen nutzen weiterhin normale Ökosystembibliotheken.

## Bezugsadapter

v0.4.18 definiert vier Referenzadapter:

```text
development/go
development/nextjs
development/python
development/quarkus
```

Die Referenz-Adapter sind Beispiele für den Vertrag, nicht privilegierte Core-Semantik. Ein Drittanbieter-Adapter verwendet die gleichen gemeinsamen Erweiterung Metadaten, Auflösung / Vertrauen Regeln und Konformität Erwartungen.

## Übereinstimmung

Ein konformer Adapter beweist: Descriptor, Detection, Contract Mapping, Development Planning, Bootstrap, Binding Portabilität, Capability Integration, Inspect Round-Trip, Evidence Validation, Idempotenz, Secret Safety, No provider leakage and MCP parity.

Die Umsetzung MUSS sich auf Folgendes konvergieren:

```text
Contract
  -> Development Plan
  -> ecosystem-native implementation
  -> normal BaseHarbor inspection
  -> evidence
  -> SATISFIED
```

MCP Parität wird durch die normale BaseHarbor Werkzeugmaschine überprüft `baseharbor.app.new`; keine Shell oder Runtime Escape Luke ist Teil des Adaptervertrages.

## Ausweitung der Verteilung und des Vertrauens

Anbieter Erweiterungen und Entwicklung Erweiterungen Aktien Verteilung Mechanik, nicht Lebenszyklus Semantik.

Gemeinsame Metadaten umfassen Erweiterungsfamilie, stabile Erweiterungs-ID, Implementierungsversion, Kompatibilität, Plattform-Kompatibilität, OCI-Referenz, unveränderliche Digest, JSON-Schema-Referenz und Signatur/SBOM/Attestation-Referenzen.

Die Auflösung ist fehlgeschlagen, wenn mehrere Kandidaten mehrdeutig sind. Vertrauenspolitik kann Digest, Signatur, SBOM und/oder Bescheinigung Metadaten erfordern, bevor eine Erweiterung akzeptiert wird.

Tags sind Entdeckungs-Aliasen. Unveränderlicher Digest ist maßgebliche Identität, wenn Artefakt-Distribution verwendet wird.

## Hintergrundgrenze

Backstage-Unterstützung ist eine emittierte Vertragsintegration, keine Entwicklungsadapter-Familie und keine Core-Abhängigkeit.

`catalog-info.yaml` Die Emission ist optional. Portal-Eigentümer sind explizit und Portal-Lebenszyklus wird nie aus BaseHarbor-Umgebung abgeleitet. Laufzeit, Ziel, Anbieterplatzierung und Anmeldeinformationen MÜSSEN NICHT in emittierte Katalog-Metadaten auslaufen.

Siehe ADR 0016 und `docs/how-to/backstage.md`.

## Auswirkungen auf die Vereinbarkeit

Diese Verträge fügen nur Autoren-Metadaten hinzu.

Sie ändern nicht bestehende portable Application Contract Semantics, Capability-Provider Platzierung Semantics oder Runtime-Provider Semantics. Brownfield `baha app init` und Grünfeld `baha app new` auf demselben Anwendungsauftrags- und Projektarchivinspektions-/Beweismodell konvergieren.
