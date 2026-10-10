# Menschliche CLI — v0.4.24

Die CLI ist **auf Aufgaben ausgerichtet**. Entwickler arbeiten überwiegend im aktuellen Application-Repository. Für Operatoren bleiben die expliziten Bereiche `app`, `target`, `provider`, `stack`, `config` und `mcp` erhalten.

## Häufige Aufgaben

| Ziel | Befehl |
| --- | --- |
| Neue Application oder anderes Objekt erstellen | `baha new` |
| Application ohne Dialog generieren | `baha new application NAME --stack go` |
| Repository einrichten / übernehmen | `baha init` |
| Application und benötigten Core starten | `baha up` |
| Application stoppen, persistente Daten erhalten | `baha down` |
| Zustand anzeigen | `baha status` |
| Contract und erkannte Ressourcen prüfen | `baha inspect` |
| Geplante Änderungen ansehen | `baha plan` |
| Fehler diagnostizieren | `baha doctor` |
| Verwaltete Applications auflisten | `baha list` |
| Sichern / wiederherstellen | `baha backup` / `baha restore` |
| Application-eigene Laufzeit entfernen | `baha destroy --yes` |

Außerhalb eines Application-Repositories gelten `down` und `destroy` für die ausgewählte Installation bzw. das Target. **`baha destroy --all` wirkt installationsweit** und muss separat geprüft werden. `baha down` löscht keine persistenten Application-Daten.

`baha --help` und `baha BEFEHL --help` zeigen zuerst typische Entwickleraufgaben; Spezialbefehle bleiben auffindbar.

## Targets anlegen

`baha new target` führt durch Name, Docker-/Podman-Runtime, Scope und Default-Auswahl. Der Dialog bietet `back` und `cancel` sowie eine abschließende Bestätigung. Für CI oder Remote-Targets dient `baha target create` mit explizitem Access-Provider und Referenz.

## Updates

`baha update --check` prüft veröffentlichte BaseHarbor-Releases und vergleicht die installierten eigenen Core-Provider mit dem ausgewählten unveränderlichen Release-Katalog. JSON zeigt `core_plan` und gegebenenfalls `core_inspection_error`; unbekannte Inventare gelten nicht als unterstützt.

`baha update --yes` aktualisiert BaseHarbor selbst und gleicht nur zugelassene Core-Provider-Änderungen nach geprüften Backup-, Ownership- und Recovery-Voraussetzungen ab. PostgreSQL-Major-Wechsel und nicht unterstützte HA-Transitions bleiben ausgeschlossen. v0.4.24 ist unveröffentlicht; die Prüfung eines verfügbaren Releases veröffentlicht diesen Kandidaten nicht.

Für Source-/Git-Updates der Application dient `baha app update`. Das ist eine andere Operation als ein BaseHarbor-/Core-Update. Die [Core-Provider-Update-Spezifikation](../spec/core-provider-update-v1.md) beschreibt die genauen Zulassungs- und Recovery-Grenzen.

## Automation und Maschinenclients

Menschenlesbare Terminalausgaben sind keine stabile API. Verwende `-o json`, sofern unterstützt, oder die typisierten MCP-/HTTP-Operationen. Die [CLI/Machine-Coverage-Übersicht (EN)](https://mcpdev80.github.io/baseharbor/reference/cli-machine-coverage/) dokumentiert Semantik und Ausnahmen.


## Ergänzende Befehlsreferenz

Die folgenden unveränderten Beispiele und Bezeichner entsprechen der englischen Referenz.



Technische Bezeichner: `baha COMMAND --help`.
