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

```json
{
  "name": "baseharbor.app.create",
  "arguments": {
    "intent": {
      "Version": 1,
      "ApplicationID": "32e1d8df-82a9-4c38-88f4-3b69b156fab8",
      "Name": "orders",
      "Environment": "dev",
      "Services": {"SQL": true}
    }
  }
}
```

MCP `baseharbor.secret.set` erhält `key` und `file`, keinen Klartextwert. Lesen erfolgt nach Autorisierung und Scope-Prüfung. Löschen benötigt ausdrückliches `approval`. Env-/Connection-Ergebnisse maskieren Credentials, auch unbekannte Provider-Felder.

## Trust und Lifecycle

`trust.status` und `trust.export` verwenden öffentliche CA-Daten. `trust.install` verändert den Ausführungshost und benötigt Genehmigung. Private Issuer-Schlüssel verlassen den Provider nicht.

Control-Plane-Up, Status, Doctor, Repair und Stop verwenden dieselben Core-Semantiken. Bootstrap-/Unseal-/Rotationspfade erhalten geschützte Recovery-Dateireferenzen statt Keys oder Tokens. Destruktion ist separat: aktiver Application-Besitz und der gesamte Scope werden geprüft, bevor eigene Ressourcen entfernt werden.

Für genaue MCP-Payloads, Workspace-Mapping und weitere Recipes: [kanonische Maschinenoperationen (EN)](https://mcpdev80.github.io/baseharbor/how-to/machine-operations/).


## Weitere technische Beispiele

```bash
git clone https://github.com/mcpdev80/baseharbor-demo.git
cd baseharbor-demo
git checkout 37c3b91287978351d0c23643ce9782c9de224a10
baha app inspect .
baha --no-input app init --quick --json
baha app preflight --json
```

```bash
baha --no-input app workspace init \
  --source backend=https://github.com/acme/api.git \
  --component api=backend -o json
baha --no-input app workspace map backend "$HOME/src/api" -o json
baha --no-input app workspace show -o json
```

```bash
chmod 600 /secure/orders-api-token
baha --no-input app secret set API_TOKEN --file /secure/orders-api-token --json
baha --no-input app secret list --json
```

```bash
baha --no-input trust status --json
baha --no-input trust export --output /tmp/baseharbor-public-ca.pem --json
baha --no-input trust install --yes --json
```

```go
report := provider.RunFull(ctx, target)
if err := provider.WriteJSON(os.Stdout, report); err != nil {
    os.Exit(2)
}
os.Exit(provider.ExitCode(report))
```


Technische Kennungen: `test`, `prod`, `baseharbor.app.show`, `{"name":"orders"}`, `baha app init orders --sql --json`, `repository`, `baseharbor.app.configure`, `hostname`, `tls_mode`, `certificate_directory`, `demo-app`, `exposure.http`, `baha app init`, `baseharbor.yaml`, `baseharbor.workspace.init`, `sources`, `components`, `baseharbor.workspace.map`, `path`, `{"key":"API_TOKEN","file":"/secure/orders-api-token"}`, `baseharbor.secret.delete`, `{"key":"API_TOKEN","approval":true}`, `secret.tls-set`, `certificate_file`, `private_key_file`, `app.environment`, `app.connection`, `approval=true`, `baseharbor.control-plane.up`, `postgres_port`, `openbao_port`, `recovery_file`, `control-plane.status`, `control-plane.doctor`, `control-plane.repair`, `control-plane.stop`, `openbao.bootstrap`, `openbao.unseal`, `openbao.rotate`, `openbao.status`, `control-plane.destroy`, `installation.destroy`.
