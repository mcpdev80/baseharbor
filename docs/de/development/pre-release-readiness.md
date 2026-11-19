# Pre-Release-Bereitschaft

Die verbindliche vollständige Checkliste steht in [der zentralen Release-Abnahme](https://github.com/mcpdev80/baseharbor/blob/develop/docs/internal/pre-release-documentation-audit.md).

**Pre-Release-ready** bedeutet: Die gesamte Release-Vorbereitung und die
gezielte Abnahme für den aktiven Scope der Version sind abgeschlossen. Für
v0.4.28 sind das Core, Demo, das gelieferte CLI und die optionale
Perses-Provider-Oberfläche: Entwicklerabläufe vom Arbeitsplatz zu Core auf
unterstützten Docker- und Podman-Laufzeiten, aktuelle
CLI/JSON/MCP/HTTP-Kompatibilität, EN/DE-Doku, Pages-Kandidaten-Build und
Navigation, Changelog, Release Notes, unveränderliche Core-/Demo-SHAs sowie
der versionsbezogene Testplan. Die optionale Perses-Oberfläche gehört nicht
zum Pflicht-Core-Bundle; wenn sie nicht gewählt ist, ist `not configured` der
Normalzustand und kein Fehler oder Readiness-Problem, und kein
Release-Nachweis darf behaupten, eine Perses-Instanz sei implementiert oder
bereitgestellt. Console und Node Connector sind archivierte Snapshots und als
aktive Abnehmer außerhalb des öffentlichen Scopes dieser Version; das
schwächt ihre archivierten Quellen, Tests und Nachweise nicht. Grüne Core-Tests allein
reichen nicht. Fehlende, fehlgeschlagene, übersprungene oder ungeprüfte
Pflichtnachweise blockieren diesen Status.

**Release-ready** setzt zusätzlich den erfolgreichen vollständigen Pre-Release
voraus. Merge, Tagging, Veröffentlichung und anschließende Produkt-/Pages-Prüfung
sind spätere Schritte mit eigener Freigabe. Der echte Pages-Deploy erfolgt erst
nach Promotion; zuvor werden Kandidaten-Build und Workflow geprüft.

Die Vorbereitung allein erlaubt keinen Pre-Release-Start.

## Parallele lokale Prüfungen

Unabhängige lokale Prüfungen laufen gemeinsam auf einer isolierten Kopie des
aktuellen Codes, einschließlich vorgemerkter, weiterer und unversionierter Änderungen:

```bash
python scripts/local-qualification.py --jobs 4 --race-package ./internal/application --race-package ./contracts
```

Betroffene Race-Pakete werden mit wiederholtem `--race-package` gewählt; `--go`
wählt ein installiertes Go-Programm. Der Runner teilt Compiler-Caches, begrenzt
parallele Prüfungen und protokolliert Commit/Tree sowie Status, Dauer und Log jeder
Prüfung im ausgegebenen Ergebnisverzeichnis. Fehler, Abbruch und Zeitüberschreitung
lassen die Gesamtprüfung fehlschlagen. Änderungen nach der Kopie verändern den
geprüften Stand nicht. Go-Tests, Vet, CLI-Build, Skript-Tests, Doku-Audit,
Lesbarkeit und strikte EN/DE-Builds laufen unabhängig. Runtime-Abnahmevariablen
werden ausgeschlossen: Native Docker-/Podman-Gates bleiben separat auf ihren
exakten Eingaben qualifiziert. Lokaler Erfolg ist keine Ecosystem- oder
Pre-Release-Bereitschaft. Betroffene Prüfungen werden direkt ausgeführt; lokale Logs sind Diagnoseausgaben.

Die erforderlichen Prüfungen laufen direkt über normale Workflow-Abhängigkeiten. Gate-Manifeste, Evidence-Wiederverwendung und erneute Prüfungen abgeschlossener Läufe entfallen. Vorbereitung genehmigt keinen Pre-Release. Der einzige CI-Vermerk enthält Merge-Commit-SHA und PR-Nummer.
