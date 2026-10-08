# Automation und Agents

CLI, JSON, MCP und geschütztes HTTP nutzen denselben Domänen-Lifecycle. Automation verwendet strukturierte Ergebnisse und Exit-Status, keine dekorierte menschliche Ausgabe.

Unterstützte JSON-Ausgabe erfolgt mit `-o json`/`--output json`. `--no-input` beziehungsweise `--non-interactive` verhindert Prompts, genehmigt aber keine Mutation und ersetzt keine fehlenden Entscheidungen oder Secrets.

Im Application-Repository:

```bash
baha --no-input app inspect . -o json > inspection.json
baha --no-input app plan -o json > plan.json
baha --no-input agent describe -o json > agent-tools.json
```

Jeden Exit-Status prüfen, bevor die Ausgabe weiterverwendet wird. `agent describe` beschreibt die tatsächlich unterstützte semantische Oberfläche.

## MCP

Der stdio-Server `baha mcp serve` stellt semantische Operationen bereit, keine generische Shell oder Docker-/Podman-Ausführung. In einem Client mit diesem Konfigurationsformat:

```json
{
  "mcpServers": {
    "baseharbor": {
      "command": "baha",
      "args": ["mcp", "serve"]
    }
  }
}
```

Der Client initialisiert MCP und ruft `tools/list` auf. CLI-Namen sind nicht automatisch MCP-Tool-Namen. `baha serve` ist eine separate lokale Service-Oberfläche; geschützte Netzwerkfreigabe benötigt ihre explizite TLS-/OIDC-Konfiguration.

Weiter: [Maschinenoperationen](../how-to/machine-operations.md), [MCP-Referenz (EN)](https://mcpdev80.github.io/baseharbor/reference/mcp/), [Machine Interface v1 (EN)](https://mcpdev80.github.io/baseharbor/spec/machine-interface-v1/).
