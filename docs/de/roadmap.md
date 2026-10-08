# Fahrplan

Detaillierte Planung lebt in GitHub Issues. Diese Seite zeigt nur Produktrichtung.

Die maßgebliche hochrangige Planungsfrage ist #155. Release-spezifischer Umfang und
Die Akzeptanz bleibt in den verbundenen Release-Dach- und Kind-Problemen.

## Aktuelle — v0.4.22

Maschinenbediener Autorisierung, Erweiterung Vertrauen und Freigabebereitschaft.

- Genehmigung des transportneutralen Maschinenbetriebs;
- Durchsetzung der gemeinsamen Genehmigungsgrenze durch die MCP;
- Produktneutrale Erweiterung Vertrauen Metadaten und politische Grenze;
- öffentliche versionierte Anbieter/Extensions-Konformitäts-Artefakte;
- dokumentierte CLI/JSON/MCP-Abdeckung und praktische Automatisierungsbeispiele;
- konsistente Repository-Init- und Lifecycle-Vorflug, plus baubare Go-Starter;
- single-instance control-plane und PostgreSQL-Standards mit expliziter HA-Intention;
- eigentumssicheres Ziel/Anbieter-Reinigung und genaue Ressourcenplanung;
- Frühere Docker/Podman-Reisen, umsetzbare Fehlerdiagnose und selektive
  Wiederverwendung von Nachweisen für unveränderte Gate-Eingänge ohne schwächere Akzeptanzkriterien.

Die Umsetzung ist abgeschlossen in PR #790. Full Docker/Podman Annahme und alle 55
erforderliche Nachweise in Prelease-Lauf 37385682137 übergeben.

## Vor dem v0.5 Einfrieren

### v0.4.23

Readiness Policy, Public Namespace und Plattformvertrag einfrieren.

- explizite Vereinbarkeits-/Gefrierpolitik;
- Entscheidungen über den öffentlichen Namensraum und die Schema-Governance;
- die Einstufung der unterstützten/geprüften Plattform;
- Restlaufzeitneutrale Vertragsgrenzen;
- Maschine HTTP/Streaming und Target Access-Grenzen erforderlich, so dass zukünftige Konsole
  Arbeit nicht wieder öffnen gefrorenen Kern semantik.
- repository-unabhängige sichere Core Bootstrap mit obligatorischen SQL, Secrets und
  Identität, Fortsetzung der Erstanwendung und Standardeinstellungen für die Maschinenrolle;
- optional Konsole an einem ausgewählten Core in der gleichen Installation angebracht und
  Sicherheitsgrenze, mit gleichem HTTPS-Ursprung wie die Standardeinstellung;
- explizite geteilte/applikationsisolierte Platzierung und gemessene Kernressource
  Nachweise;
- vollständige Remote-Anwendung und Private-Consumer-Integration Qualifikation
  vor der Freigabe der Genehmigung.

Die Umsetzung ist in PR #809 im Gange.[draft release notes](releases/v0.4.23.md)
keine Freigabe oder Genehmigung vor der Freigabe beantragen.

### v0.4.24

Human CLI und Dokumentation Konsolidierung.

- kompakter, aufgabenorientierter menschlicher CLI;
- schrittweise Offenlegung für fortgeschrittene Namespaces/Operator-Namespaces;
- Aufgaben-/Kategorie-First-Dokumentations-Informationsarchitektur;
- vollständige kanonische Befehlsreferenz, die mit dem finalen CLI ausgerichtet ist.

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
