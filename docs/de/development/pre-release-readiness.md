# Pre-Release-Bereitschaft

Die verbindliche vollständige Checkliste steht in [der zentralen Release-Abnahme](https://github.com/mcpdev80/baseharbor/blob/develop/docs/internal/pre-release-documentation-audit.md).

**Pre-Release-ready** bedeutet: Die gesamte Release-Vorbereitung und die
gezielte Abnahme sind abgeschlossen. Dazu gehören Core, Demo, Console,
Node-Connector, CLI/JSON/MCP/HTTP-Kompatibilität, Entwicklerabläufe, EN/DE-Doku,
Pages-Build und Navigation, Changelog, Release Notes, exakte Ecosystem-SHAs
sowie der versionsbezogene Gate- und Evidence-Plan. Grüne Core-Tests allein
reichen nicht. Fehlende, fehlgeschlagene, übersprungene oder ungeprüfte
Pflichtnachweise blockieren diesen Status.

**Release-ready** setzt zusätzlich den erfolgreichen vollständigen Pre-Release
voraus. Merge, Tagging, Veröffentlichung und anschließende Produkt-/Pages-Prüfung
sind spätere Schritte mit eigener Freigabe. Der echte Pages-Deploy erfolgt erst
nach Promotion; zuvor werden Kandidaten-Build und Workflow geprüft.

Vorhandene grüne Gates werden nur wiederverwendet, wenn der Evidence-Verifier
die unveränderten Eingaben bestätigt. Nur betroffene Gates erneut ausführen.
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
Pre-Release-Bereitschaft. Unverändert qualifizierte Eingaben über den vorhandenen
Evidence-Verifier wiederverwenden, statt die gesamte lokale Suite zu wiederholen.
