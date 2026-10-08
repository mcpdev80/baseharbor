# BaseHarbor mit Backstage verwenden

Backstage bleibt Portal, Repository-Veröffentlichung und Catalog-Layer. BaseHarbor liefert Application Intent, Planung und Lifecycle-Evidence über seine bestehenden Verträge.

Eine Scaffolder Custom Action kann strukturierte CLI-Ausgabe verwenden:

```bash
baha app new catalog-api --stack go --all --emit-backstage --backstage-owner platform-team --output json
```

Für MCP-fähige Integration wird `baseharbor.app.new` nach Discovery mit semantischen Inputs wie `name`, `stack`, `capabilities`, `emit_backstage` und `backstage_owner` aufgerufen. Menschliche Ausgabe nicht parsen.

## Catalog-Metadaten

Auf ausdrücklichen Auftrag wird Application-eigenes `catalog-info.yaml` erzeugt: Component-Identität, expliziter Portal-Owner, expliziter Lifecycle mit Default `experimental` und Contract-Referenz. BaseHarbor-Besitz definiert keinen Backstage-Owner; Umgebung definiert keinen Backstage-Lifecycle.

Target, Runtime, Placement, Zugangsdaten und generierter Deployment-Zustand landen nicht in Catalog-Metadaten. Nach Veröffentlichung konfiguriert das Portal normale Catalog-Ingestion; die Datei gehört dem Repository und wird nicht dauerhaft vom Core reconciliert.

## Grenzen

Das Portal besitzt Login/RBAC, Plugins, Repository-Credentials, Veröffentlichung und Ausführungs-Policy. BaseHarbor erhält keine Portal-Credentials und ruft keine Catalog-API auf. Evidence verwendet `baseharbor.evidence/v1` mit Soll-, erzwungenem, beobachtetem und verifiziertem Zustand, kein Backstage-Sonderformat.

Core enthält keinen Backstage-SDK, Node-Runtime, Template-Interpreter oder `${{ ... }}`-Evaluator. [Integrationsvertrag (EN)](https://mcpdev80.github.io/baseharbor/how-to/backstage/).
