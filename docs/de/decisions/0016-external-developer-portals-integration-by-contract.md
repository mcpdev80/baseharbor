# ADR 0016: Externe Entwicklerportale integrieren durch emittierte Verträge

## Status

Akzeptiert für v0.4.18.

## Kontext

BaseHarbor besitzt portable Application Intention, Lifecycle Evidence und maschinenlesbare Application Semantics. Externe Entwicklerportale besitzen Katalogingestion, Gerüst UX, Repository-Veröffentlichung und portalspezifische Anmeldeinformationen.

Die Koppelung von BaseHarbor Core an ein Portal würde Verantwortlichkeiten duplizieren und die Verfügbarkeit von Portalen zu einem Teil der Laufzeitarchitektur von BaseHarbor machen.

## Entscheidung

Externe Entwicklerportale integrieren sich mit BaseHarbor durch emittierte, versionierte Verträge und optionale ökosystem-native Repository-Metadaten.

Backstage ist der erste Referenzverbraucher. BaseHarbor kann eine statische emittieren `catalog-info.yaml` Projektion, wenn explizit angefordert, aber Core hängt nicht von Backstage ab.

Die emittierten Katalog-Metadaten:

- nur aus BaseHarbor-eigenen Auftragsdaten abgeleitet werden;
- ist deterministisch und statisch;
- schließt nicht das Eigentum der Organisation ab;
- enthält keine Repository-Publishing-Anmeldeinformationen;
- enthält keine Backstage-Template-Ausdrücke;
- benötigt kein Live-Portal.

BaseHarbor Core nicht:

- abhängig vom Backstage-SDK oder Node-Laufzeit;
- Aufruf der API Backstage Catalog;
- Implementierung eines Backstage-Katalog-Clients;
- Parse oder Ausführung `template.yaml`;
- Bewertung `${{ ... }}` Ausdrücke;
- Einführung eines generischen Gerüsts/Action-Motors;
- Repositories im Namen eines Portals veröffentlichen.

Die gleiche architektonische Grenze gilt für Port und andere externe Entwicklerportale.

## Folgen

Die Portalintegration bleibt optional und default-off.

Die kanonische Quelle der Wahrheit bleibt der BaseHarbor Application Contract, Maschinenverträge und Beweismodell. Portal Metadaten ist eine Projektion und kann ohne Änderung der tragbaren Anwendungssemantik regeneriert werden.

Ein Portal kann BaseHarbor-Ausgaben verbrauchen, ohne dass BaseHarbor Portalanmeldeinformationen oder eine portalspezifische Laufzeitabhängigkeit benötigt.
