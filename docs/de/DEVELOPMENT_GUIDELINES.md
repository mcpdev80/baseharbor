# Leitlinien für die Entwicklung von BaseHarbor

Diese Regeln gelten für Code, Tests, Dokumentation und Anbieter-/Laufzeitarbeit.

## Prinzip der Projekttechnik

> **AI-generiert, menschlich spezifiziert, maschinenverifiziert.**
> **KI-generiert, menschlich spezifiziert, maschinell verifiziert.**

Dies ist eine konkrete technische Regel:

- **Menschenspezifische:** Architektur, Absicht, Einschränkungen, Sicherheitsgrenzen und Akzeptanzkriterien sind bewusste menschliche Entscheidungen.
- **AI-generiert:** KI kann Code, Tests, Dokumentation und repetitive Integrationsarbeit aus diesen Spezifikationen implementieren.
- **Machine-verified:** Korrektheit wird durch deterministische Validierung, Tests, statische Überprüfungen, Laufzeitverifizierung und Freigabenachweise statt Vertrauen in generierte Ausgabe allein ermittelt.

AI-generierte Änderungen umgehen nicht überprüfbare Verträge, Eigentumsregeln, Überprüfung oder Freigabetore.

## 1. Kleinstes richtiges Design

Nur das umsetzen, was die aktuelle Anforderung benötigt. Vermeiden Sie spekulative Rahmenbedingungen, unabhängige Refaktors und produktspezifische Shortcuts.

## 2. Sicher und ausfallen geschlossen

Unüberprüfbares Eigentum, fehlende Genehmigung, ungültiger Zustand, nicht unterstützte Semantik und unüberprüfbare Sicherheitsannahmen sind kein Erfolg.

## 3. Verträge, nicht Erzeugnisse

Anwendungen beschreiben Fähigkeiten. Anbieter/Laufzeit-Produktauswahlen bleiben hinter BaseHarbor-Grenzen.

## 4. Standards zuerst

BaseHarbor MUSS etablierte offene Standards gegenüber proprietären Verträgen bevorzugen.

- De-facto-Standards und etablierte Ökosystemkonventionen SOLLTEN dort verwendet werden, wo kein geeigneter formaler Standard existiert.
- Baha-spezifische Verträge MÜSSEN nur Semantik definieren, die vernünftigerweise nicht durch einen bestehenden Standard repräsentiert werden kann.
- Baha-Erweiterungen MÜSSEN klar getrennt, versioniert und Provider-neutral sein.
- Anbieterprodukte MÜSSEN keine produktspezifischen Verträge in den BaseHarbor-Kern ziehen.
- Jeder neue Service-Art oder Fähigkeitsanbieter MUSS dokumentieren: Bestehende Standards, angenommene Standards, Abweichungen, Baha-Erweiterungen und Kompatibilität Auswirkungen vor der Implementierung.

## 5. Anbieterachsen getrennt halten

```text
runtime != capability != delivery
```

Verstecken Sie nicht eine Anbieterachse innerhalb einer anderen.

## 6. Geheimnisse geben nie normale Datenpfade ein

Legen Sie Passwörter, Token, private Schlüssel, geheime Werte oder kridentielle URLs nicht durch Protokolle, Fehler, Metriken-Etiketten, Audit-Datensätze, Maschinenausgabe oder übergebene Manifeste aus.

## 7. Eine maßgebliche Quelle der Wahrheit

Halten Sie nicht den gleichen detaillierten Vertrag manuell an mehreren Stellen.

Bevorzugen Sie gegebenenfalls Schema/OpenAPI/Protobuf/typed Definitionen.

## 8. Mutationen folgen einem Lebenszyklus

```text
plan -> preflight -> apply -> verify
```

Planung und Preflight mutieren nicht. Melden Sie keinen Erfolg, bevor die erforderliche Überprüfung erfolgreich ist.

## 9. CLI, JSON und MCP teilen sich einen semantischen Kern

Die Präsentation unterscheidet sich. Domain-Verhalten nicht.

Keine Schnittstelle darf Politik, Eigentum, Versöhnung, Verifizierung oder Beweise umgehen.

## 10. Prüfungen beweisen Invarianten

Testen Sie Sicherheit, Eigentum, Kompatibilität, Ausfall und Recovery-Verhalten, nicht nur Happy-path-Funktionsausgabe.

Verwenden Sie zuerst die lokale/repository-Validierung. Nutzen Sie GitHub-Aktionen, wenn dies vom Release-Gate erforderlich ist.

## 11. Änderungen im Rahmen behalten

Refaktor nicht unabhängig Code in der gleichen Änderung. Bewahren Sie aktuelle Architektur, es sei denn, die Aufgabe zeigt ein fehlendes primitiv.

## 12. Halten Sie die Quelle menschlich lesbar

Quelle Layout ist Teil des Beitragszahlervertrages.

- Eine nicht-testete Go-Quelldatei MUSS bei oder unter 800 Zeilen bleiben. Durch Verantwortung teilen, bevor mehr Verhalten hinzugefügt wird.
- Eine Go-Funktion oder Methode MUSS bei oder unter 200 Zeilen bleiben. Lange Orchestrierung sollte zu benannten Lebenszyklusphasen oder Domänenhelfern delegieren.
- Nicht aufteilen Code nur, um eine Zahl zu erfüllen: Dateinamen und Hilfsgrenzen MUSS reale Verantwortlichkeiten beschreiben.
- Tests, generierte Artefakte, Referenzdokumentation und historisches Freigabematerial unterliegen nicht diesen Quellcode-Grenzen.
- Der normale Quellqualitäts-Workflow und Pre-Release-Gate erzwingen diese Grenzen mit `scripts/source-readability-audit.sh`.

## 13. Zusammengeführte Zweigniederlassungen löschen

Zweige sind temporäre Arbeitsflächen, nicht langlebige Geschichte.

- Ein Feature, Fix, Release-Preparation oder Validierungszweig MUSS gelöscht werden, nachdem seine Änderungen zusammengeführt oder anderweitig vollständig in den Zielzweig integriert wurden.
- Vorübergehend `runtime-validation/*` Zweige MÜSSEN entfernt werden, sobald das Validierungsergebnis nicht mehr benötigt wird.
- Langlebige Branchen beschränken sich auf explizit bezeichnete Integration oder gepflegte Arbeitsspuren wie z.B.`main `, ` develop`und absichtlich beibehaltene aktive Merkmals-/Prüfzweige.
- Vergewissern Sie sich vor dem Löschen eines abweichenden Zweigs, dass alle erforderlichen Änderungen bereits im Zielzweig vorhanden sind oder absichtlich obsolet sind.

## Dokumentation

Folgen[Documentation style](STYLE_GUIDE.md).

Human docs erklären. Referenz aufgezählt. Specs definieren. ADRs bewahren rationale. GitHub Issues planen zukünftige Arbeit. Releases bewahren Geschichte.

Jedes Release läuft das komplette[pre-release audit](internal/pre-release-documentation-audit.md). Es deckt Release-Scope-Probleme, BaseHarbor-Implementierung, Verträge, EN/DE-Dokumentation, Roadmap/Staleness, Changelog/Release-Notes, die externen `baseharbor-demo`, GitHub Pages, Exact-Candidate Pre-Release-Evidenz, Promotion, Veröffentlichung und Post-Release-Verifikation.
