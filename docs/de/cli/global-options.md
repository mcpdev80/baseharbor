# Globale Optionen

Diese Optionen werden vor der Befehlsausführung verarbeitet.

| Option | Wirkung |
| --- | --- |
| `--target NAME` / `--target=NAME` | Target-Auflösung überschreiben |
| `-q`, `--quiet`, `--silent` | Knappe Ausgabe |
| `-v`, `--verbose` | Diagnoseausgabe |
| `--no-color` | Farben deaktivieren |
| `--plain` | Schlichte Ausgabe |
| `--no-input`, `--non-interactive` | Keine Eingabeprompts |
| `--version` | BaseHarbor-Version anzeigen |

Quiet und Verbose schließen sich aus. Automation wählt Target und Umgebung bei Mehrdeutigkeit ausdrücklich, verwendet strukturierte JSON-/MCP-Ergebnisse und prüft Exit-Status. Nicht interaktiv bedeutet keine automatische Genehmigung.

[Exakte Referenz (EN)](https://mcpdev80.github.io/baseharbor/cli/global-options/).
