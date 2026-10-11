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
