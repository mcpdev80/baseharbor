# Kommandozeile

`baha` ist die primäre menschliche Schnittstelle. Befehle sind nach Aufgabe geordnet und verwenden dieselbe Domänensemantik wie JSON, MCP und geschütztes HTTP.

Für den normalen Application-Ablauf: neue Anwendung mit `baha app new` oder bestehendes Repository mit `baha app init`, danach `baha up`, `baha status` und `baha doctor`. `baha app down` stoppt nur die Application; `baha down` betrifft die lokale Control Plane.

## Bereiche

- [Core-Workflow](core.md)
- [Applications](applications.md)
- [Targets](targets.md)
- [Provider](providers.md)
- [Development und Workspaces](development.md)
- [Organisation und Policy](organization-policy.md)
- [Security und Trust](security-trust.md)
- [Automation und Agents](automation-agents.md)
- [Shell und Bedienung](shell-ux.md)
- [Globale Optionen](global-options.md)

Die [vollständige Befehlsreferenz (EN)](https://mcpdev80.github.io/baseharbor/cli/command-index/) dokumentiert die genaue aktuelle Oberfläche. Namen und Flags werden nicht übersetzt.
