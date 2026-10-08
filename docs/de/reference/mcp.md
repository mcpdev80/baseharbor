# MCP-Referenz

BaseHarbor bietet Coding-Agenten eine bewusst begrenzte MCP-Schnittstelle.

Den lokalen MCP-Server starten:

```bash
baha mcp serve
```

Die verfügbaren Maschinenoperationen ermitteln:

```bash
baha agent describe -o json
```

## Semantische Tools

Die aktuelle vollständige Registry wird durch Quelltests mit der tatsächlichen MCP-Discovery abgeglichen. Die folgende Tabelle stammt aus typisierten Operationsmetadaten; manuell gepflegte unvollständige Tool-Listen sind nicht verbindlich.

<!-- BEGIN GENERATED MCP REGISTRY -->

| Tool | Safety | Policy required | Approval required |
| --- | --- | --- | --- |
| `baseharbor.installation.destroy` | `destructive` | false | true |
| `baseharbor.release.check` | `read_only` | false | false |
| `baseharbor.control-plane.status` | `read_only` | false | false |
| `baseharbor.control-plane.doctor` | `read_only` | false | false |
| `baseharbor.control-plane.up` | `mutating` | false | false |
| `baseharbor.control-plane.stop` | `mutating` | false | false |
| `baseharbor.control-plane.repair` | `mutating` | false | false |
| `baseharbor.control-plane.destroy` | `destructive` | false | true |
| `baseharbor.openbao.status` | `read_only` | false | false |
| `baseharbor.openbao.bootstrap` | `mutating` | false | false |
| `baseharbor.openbao.unseal` | `mutating` | false | false |
| `baseharbor.openbao.rotate` | `mutating` | false | false |
| `baseharbor.dev.domain` | `mutating` | false | false |
| `baseharbor.dev.credentials` | `mutating` | false | false |
| `baseharbor.app.environment` | `read_only` | false | false |
| `baseharbor.app.connection` | `read_only` | false | false |
| `baseharbor.connectivity.list` | `read_only` | false | false |
| `baseharbor.connectivity.connect` | `mutating` | false | false |
| `baseharbor.connectivity.disconnect` | `mutating` | false | false |
| `baseharbor.operator.identity` | `read_only` | false | false |
| `baseharbor.provider.init` | `mutating` | false | false |
| `baseharbor.provider.test` | `read_only` | false | false |
| `baseharbor.workspace.show` | `read_only` | false | false |
| `baseharbor.app.show` | `read_only` | false | false |
| `baseharbor.app.create` | `mutating` | false | false |
| `baseharbor.app.adopt` | `mutating` | false | false |
| `baseharbor.app.configure` | `mutating` | false | false |
| `baseharbor.runtime-identity.rotate` | `mutating` | false | true |
| `baseharbor.runtime-identity.revoke` | `destructive` | false | true |
| `baseharbor.tls.update` | `mutating` | false | false |
| `baseharbor.trust.status` | `read_only` | false | false |
| `baseharbor.trust.export` | `mutating` | false | false |
| `baseharbor.trust.install` | `mutating` | false | true |
| `baseharbor.app.stop` | `mutating` | false | false |
| `baseharbor.app.preflight` | `read_only` | false | false |
| `baseharbor.secret.list` | `read_only` | false | false |
| `baseharbor.secret.set` | `mutating` | false | false |
| `baseharbor.secret.delete` | `destructive` | false | true |
| `baseharbor.secret.tls-set` | `mutating` | false | false |
| `baseharbor.target.create` | `mutating` | false | false |
| `baseharbor.target.delete` | `mutating` | false | false |
| `baseharbor.stack.list` | `read_only` | false | false |
| `baseharbor.stack.show` | `read_only` | false | false |
| `baseharbor.stack.create` | `mutating` | false | false |
| `baseharbor.workspace.init` | `mutating` | false | false |
| `baseharbor.workspace.map` | `mutating` | false | false |
| `baseharbor.target` | `read_only` | false | false |
| `baseharbor.target.list` | `read_only` | false | false |
| `baseharbor.runtime.capabilities` | `read_only` | true | false |
| `baseharbor.runtime.list` | `read_only` | true | false |
| `baseharbor.runtime.inspect` | `read_only` | true | false |
| `baseharbor.runtime.metrics` | `read_only` | true | false |
| `baseharbor.runtime.start` | `mutating` | true | false |
| `baseharbor.runtime.stop` | `mutating` | true | false |
| `baseharbor.runtime.restart` | `mutating` | true | false |
| `baseharbor.app.list` | `read_only` | false | false |
| `baseharbor.inspect` | `read_only` | false | false |
| `baseharbor.workspace.list` | `read_only` | false | false |
| `baseharbor.workspace.resolve` | `read_only` | false | false |
| `baseharbor.workspace.status` | `read_only` | false | false |
| `baseharbor.workspace.update` | `mutating` | false | false |
| `baseharbor.app.new` | `mutating` | false | false |
| `baseharbor.plan` | `read_only` | false | false |
| `baseharbor.apply` | `mutating` | true | false |
| `baseharbor.status` | `read_only` | false | false |
| `baseharbor.doctor` | `read_only` | false | false |
| `baseharbor.observe` | `read_only` | false | false |
| `baseharbor.evidence` | `read_only` | false | false |
| `baseharbor.update` | `mutating` | true | false |
| `baseharbor.repair` | `mutating` | true | false |
| `baseharbor.backup` | `mutating` | false | false |
| `baseharbor.restore` | `mutating` | true | false |
| `baseharbor.destroy` | `destructive` | true | true |
| `baseharbor.policy.check` | `read_only` | true | false |
| `baseharbor.provider.list` | `read_only` | false | false |
| `baseharbor.provider.inspect` | `read_only` | false | false |
| `baseharbor.provider.verify` | `read_only` | false | false |
| `baseharbor.provider.add` | `mutating` | true | false |
| `baseharbor.provider.remove` | `destructive` | true | true |
| `baseharbor.organization.inspect` | `read_only` | false | false |
| `baseharbor.organization.check` | `read_only` | false | false |
| `baseharbor.organization.set` | `mutating` | true | false |
| `baseharbor.organization.update` | `mutating` | true | true |
| `baseharbor.policy.explain` | `read_only` | false | false |

<!-- END GENERATED MCP REGISTRY -->

`baseharbor.target` liefert das wirksame Target und den aus dem Repository aufgelösten Application-/Environment-Kontext. Ein optionaler Target-Selektor ist zulässig; die Semantik entspricht `baha target -o json`.

Diese Tools führen BaseHarbor-Lifecycle-Operationen aus und sind keine Wrapper um CLI-Befehle.

`baseharbor.apply` bedeutet semantische Konvergenz statt einer vollständigen Abbildung aller menschenlesbaren CLI-Aliase.

`baseharbor.evidence` ist lesend und liefert dasselbe deterministische Evidence-Bundle wie `baha app evidence -o json`, einschließlich Recovery-Beiträgen und begrenzter Audit-Historie.

`baseharbor.destroy` ist destruktiv und verlangt eine ausdrückliche Zustimmung.

Backup und Restore akzeptieren einen lokalen Passwortdateipfad, der nur dem Eigentümer zugänglich ist. Klartextpasswörter werden nicht als MCP-Argumente entgegengenommen.

## Operator-Autorisierung

MCP definiert kein eigenes RBAC-Modell. Jedes mutierende oder destruktive Tool durchläuft vor Änderungen dieselbe BaseHarbor-Machine-Operator-Autorisierungsgrenze.

- Entwicklung verwendet ausdrücklich `trusted-local` als Akteurssemantik.
- Verwaltete Umgebungen verlangen eine gültige konfigurierte, authentifizierte Operator-Identität und lehnen fehlende oder ungültige Identitäten ab.
- Die Autorisierungsentscheidung erhält Maschinen-Sicherheitsmetadaten: `read_only`, `mutating`, `destructive`, Policy- und Freigabeanforderungen.
- Autorisierungs- und Audit-Metadaten enthalten niemals Bearer-/Refresh-Tokens, private Schlüssel oder vergleichbares Credential-Material.

## Regeln

- MCP verwendet denselben semantischen Lifecycle wie CLI/JSON.
- Dieses Release bietet MCP lokal über stdio.
- MCP bietet keine generische Shell.
- MCP erlaubt keine uneingeschränkte Docker-, Compose-, Podman- oder provider-native Ausführung.
- Operationen deklarieren lesende, mutierende oder destruktive Sicherheitssemantik.
- Autorisierung, Policy, Eigentümerschaft, Preflight, Reconciliation, Verifikation und geschützte Bindings bleiben verbindlich.
- Maschinenoperationen fragen nicht interaktiv nach Eingaben; ungelöste Entscheidungen erzeugen typisierte handlungsfähige Ergebnisse.
- Ausgaben sind secret-sicher.
- Runtime-spezifische Realisierungsdetails werden nicht zu MCP-Semantik.

Die maßgebliche Operations- und Sicherheitsdefinition ist der versionierte Machine Contract aus `baha agent describe -o json` und der typisierten Maschinen-Registry.

## Client-Berechtigungen

Ein sicherer Standard erlaubt Inspektion und Planung, während jedes mutierende Lifecycle-Tool eine ausdrückliche Client-Freigabe verlangt.

Manche Clients wandeln den Servernamen `baha` und MCP-Toolpunkte in durch Unterstriche getrennte Berechtigungsschlüssel um. Beispiel:

```json
{
  "permission": {
    "baha_*": "ask",
    "baha_baseharbor_target": "allow",
    "baha_baseharbor_status": "allow",
    "baha_baseharbor_workspace_resolve": "allow",
    "baha_baseharbor_workspace_status": "allow",
    "baha_baseharbor_inspect": "allow",
    "baha_baseharbor_plan": "allow",
    "baha_baseharbor_doctor": "allow",
    "baha_baseharbor_observe": "allow",
    "baha_baseharbor_evidence": "allow",
    "baha_baseharbor_policy_check": "allow",
    "baha_baseharbor_policy_explain": "allow"
  }
}
```

Die genaue Schreibweise ist clientspezifisch. Die Sicherheitsregel ist verbindlich: Lesen, Planen und Verifizieren können automatisch erlaubt sein. `apply`, `update`, `workspace.update`, `repair`, `backup`, `restore` und `destroy` bleiben freigabepflichtig.

`baseharbor.inspect` ist lesend, kann aber Repository-Pfade oder Git-URLs untersuchen. Umgebungen mit strengen Informationsgrenzen können deshalb `ask` verlangen, auch wenn kein Zustand verändert wird.

Für `baseharbor.destroy` ist zusätzlich die ausdrückliche destruktive BaseHarbor-Freigabe erforderlich. Eine Client-Berechtigung allein reicht nicht aus.

## Lifecycle-Timeouts

Mutierende Lifecycle-Operationen können länger als eine typische MCP-Anfrage dauern, insbesondere beim Laden oder Bauen von Images, bei Provider-Readiness und bei Backup/Restore.

Ein sehr kurzer Client-Timeout von 30 Sekunden ist ungeeignet. Für echte Application-Arbeit mindestens 180 Sekunden verwenden; als allgemeiner Ausgangspunkt sind 300 Sekunden empfohlen.

Beispiel:

```json
{
  "mcp": {
    "baha": {
      "timeout": 300000
    }
  }
}
```

Seit v0.4.15.1 läuft eine bereits akzeptierte mutierende Konvergenz unabhängig vom Abbruch der Client-Anfrage weiter, um halbfertige Mutationen durch Client-Timeouts zu verhindern. BaseHarbor begrenzt sie weiterhin auf eine eigene Lifecycle-Deadline von 30 Minuten. Ein längerer Client-Timeout bleibt sinnvoll, damit der Aufrufer das verifizierte Ergebnis statt einer späteren Statusabfrage erhält.

Sicherheitsmodell:

```text
understand / plan / verify
        ↓
mutation requires approval
        ↓
destruction requires explicit BaseHarbor approval too
```

## Development- und Workspace-Tools

`baseharbor.app.new` ist die semantische Greenfield-Erzeugung. Sie verwendet denselben Application Contract, Stack Profile, Development Plan und dasselbe Adaptermodell wie die CLI, ohne eine generische Shell anzubieten.

Die Ausgabe enthält `validation_scope: repository_capability_evidence` und `build_verified: false`. Ein erfolgreich inspiziertes Repository beweist weder den Build noch die Runtime-Bereitschaft.

`baseharbor.workspace.resolve` löst lesend die kanonischen Component-/Source-Identitäten anhand der lokalen XDG-Workspace-Zuordnung auf.

`baseharbor.workspace.status` zeigt denselben Git-Zustand je Repository wie die CLI. Optionales Fetch aktualisiert nur die Remote-Tracking-Daten; es verändert keine ausgecheckten Commits.

`baseharbor.workspace.update` verwendet dieselbe abgesicherte native Git-Logik wie `baha app workspace update`: Nur saubere, nicht divergierte Branches mit konfiguriertem Upstream dürfen Fast-Forward ausführen. MCP umgeht weder Dirty-, Diverged-, Detached- noch Missing-Upstream-Sperren. `check=true` prüft nur den geplanten Ablauf.

## CLI-Abdeckung

Die [CLI-/Maschinen-Matrix](cli-machine-coverage.md) erfasst sämtliche unterstützten sichtbaren Befehle und Aliase. Unklassifizierte neue Befehle führen zum Fehler im generierten Inventory-Test. Host-, Interaktiv- und Transportausnahmen werden mit Begründung und sicheren Alternativen erfasst. `workspace.init` und `workspace.map` akzeptieren typisierte Eingaben und nutzen dieselben Operationen wie die nicht interaktiven CLI-/JSON-Pfade.

## Typisierte Operationsbeispiele

Die [Rezepte für Maschinenoperationen](../how-to/machine-operations.md) beschreiben Application-Erzeugung, Workspace-Mapping, geschützte Secrets, Trust-Freigaben und Provider-Conformance. Verbindliche Eingaben und erforderliche Felder stammen aus dem tatsächlichen MCP-`tools/list`-Schema. CLI-JSON und MCP verwenden gemeinsame Operationen; rohe Zugangsdaten und beliebige Runtime-Kommandos bleiben ausgeschlossen.

## Explizite Verfügbarkeit der Control Plane

`baseharbor.control-plane.up` richtet den verpflichtenden SQL-/Secrets-/Identity-Core ohne Application-Repository ein. `machine_role` wählt mit `development` oder `deployment` passende Vorgaben; `ha` ist ein Boolean mit Standard `false`. Weitere Eingaben sind Target, Ports und ein geschützter Recovery-Dateipfad. Der Lifecycle wählt bei neuen Targets die Standardtopologie und verweigert unvereinbare HA-Wünsche, wenn bereits ein Single-Server-Zustand besteht. `control-plane.status` meldet tatsächliche Mitglieder und Verfügbarkeit; Einzelserverbetrieb verspricht keinen Failover.

```json
{"name":"baseharbor.control-plane.up","arguments":{"target":"local","ha":false}}
```

## Node- und Trust-Werkzeuge

Die typisierten Node-Operationen `baseharbor.node.add`, `baseharbor.node.connect`, `baseharbor.node.list`, `baseharbor.node.status` und `baseharbor.node.disconnect` bilden denselben sicheren Enrollment- und Lifecycle-Vertrag wie die CLI ab. Enrollment-Geheimnisse werden nicht als MCP-Ergebnis zurückgegeben; `node.add` liefert ausschließlich den Pfad zur owner-only Enrollment-Datei. `node.connect` und `node.disconnect` benötigen ausdrückliche Freigabe.

`baseharbor.trust.uninstall` entfernt ausschließlich von BaseHarbor verwaltetes Host-Trust-Material und bewahrt fremde Zertifikate auf.
