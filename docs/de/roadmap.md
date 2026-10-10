# Fahrplan

Detaillierte Planung steht in GitHub Issues. Diese Seite zeigt die Produktrichtung; [Issue #155](https://github.com/mcpdev80/baseharbor/issues/155) ist die übergeordnete Roadmap.

## Aktueller Kandidat — v0.4.24

Implementierung und Pre-Release-Abnahme sind abgeschlossen; die Veröffentlichung steht noch aus.

- Aufgabenorientierte CLI und repositorybezogener Application-Lifecycle.
- Explizite Core-/Target-Auswahl und Rootless-Docker als Standard.
- Gemeinsame Provider ohne doppelte Server, mit ausdrücklich aktivierter HA.
- Abgesicherte Provider-Updates, PostgreSQL-/etcd-Recovery und klar begrenzte Upgrade-Pfade.
- Gemeinsame Console-/Node-Connector-Workflows und vollständige EN/DE-Dokumentation.

Die [Release Notes](releases/v0.4.24.md) beschreiben die wichtigsten Änderungen und Kompatibilitätshinweise. Detaillierter Umfang und Nachweise stehen in [PR #837](https://github.com/mcpdev80/baseharbor/pull/837).

## Vor dem v0.5-Freeze

### v0.4.25

Semantische Akzeptanz.

- Nachweis der semantischen Parität CLI/JSON/MCP;
- Nachweis der Laufzeit/Versorgergrenzen und des Eigentums;
- Nachweis, dass die Kompatibilität des Anbieters größer ist als die Fähigkeits-Namen-Matching;
- die endgültige Architektur und die Lebenszyklusakzeptanz vor dem Einfrieren durchführen.

## v0.5

Gefriert, versioniert und verhärtet die öffentlichen Aufträge.

Keine neue Plattform Primitive gehört in die v0.5 Linie.

## Später

### v0.6

Übertragbarkeit nach dem Einfrieren/Garantieverhärtung und Vorbereitungsarbeiten, die nicht
erfordern die Änderung der gefrorenen tragbaren Anwendung Semantik.

Die ursprüngliche portable-Verfügbarkeit Vertrag Arbeit wurde vorgezogen in v0.4.21
V0.6 darf also kein zweites Verfügbarkeitsmodell einführen.

### v0.7

Kubernetes Runtime-Implementierung hinter der gefrorenen Runtime Provider-Grenze.

Namespace-only-Operation bleibt das primäre Autorisierungsprofil.

### v0.8

Kubernetes Complete: voller Lebenszyklus, Fähigkeit/Anbieter, Erholung,
Beobachtungsfähigkeit, Update/Migration, Agent/DX und produktionshärtende Parität.

OpenShift, Enterprise und Cloud/Runtime-Erweiterung bauen auf dem eingefrorenen portablen
die Verträge zu ändern, anstatt sie zu ändern.
