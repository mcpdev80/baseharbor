# Unterstützte Operationen automatisieren

`baha agent describe -o json` und MCP `tools/list` zeigen tatsächliche IDs, Inputs, Sicherheit und Genehmigung. Dateireferenzen gelten auf dem Host, auf dem `baha mcp serve` läuft. Test/Prod benötigen den konfigurierten authentifizierten Operator.

## Intent erzeugen und lesen

```bash
baha --no-input app create orders --sql --json
baha --no-input app show orders --json
baha --no-input app list --json
```

Das erzeugt Intent, keine Container. MCP `baseharbor.app.create` erhält einen vollständigen typisierten `intent` mit eigener stabiler Application-UUID. Discovery ist maßgeblich für Property-Namen. Bestehender Intent wird nicht überschrieben.

Repository-Adoption verwendet `app init` beziehungsweise MCP `baseharbor.app.adopt`. Deployment-Inputs werden explizit konfiguriert; fehlende oder mehrdeutige Werte erzeugen Fehler statt Prompts. Quick Init bewahrt eine vorhandene Contract-Identität; Änderungen der Source bedeuten keine automatische Übernahme neuer Capabilities.

## Secrets geschützt liefern

Die Eingabedatei außerhalb von Git vorbereiten und schützen:

```bash
chmod 600 /secure/orders-api-token
baha --no-input app secret set API_TOKEN --file /secure/orders-api-token --json
baha --no-input app secret list --json
```

MCP `baseharbor.secret.set` erhält `key` und `file`, keinen Klartextwert. Lesen erfolgt nach Autorisierung und Scope-Prüfung. Löschen benötigt ausdrückliches `approval`. Env-/Connection-Ergebnisse maskieren Credentials, auch unbekannte Provider-Felder.

## Trust und Lifecycle

`trust.status` und `trust.export` verwenden öffentliche CA-Daten. `trust.install` verändert den Ausführungshost und benötigt Genehmigung. Private Issuer-Schlüssel verlassen den Provider nicht.

Control-Plane-Up, Status, Doctor, Repair und Stop verwenden dieselben Core-Semantiken. Bootstrap-/Unseal-/Rotationspfade erhalten geschützte Recovery-Dateireferenzen statt Keys oder Tokens. Destruktion ist separat: aktiver Application-Besitz und der gesamte Scope werden geprüft, bevor eigene Ressourcen entfernt werden.

Für genaue MCP-Payloads, Workspace-Mapping und weitere Recipes: [kanonische Maschinenoperationen (EN)](https://mcpdev80.github.io/baseharbor/how-to/machine-operations/).
