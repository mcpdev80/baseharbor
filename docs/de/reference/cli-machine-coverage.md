# CLI- und Maschinen-Abdeckung

Diese Matrix wird aus dem tatsächlichen Befehlsbaum und der typisierten Operations-Registry erzeugt. Jede Operation ist semantisch, ein Alias, eine Darstellung oder eine ausdrücklich begründete Host-/Transport-Ausnahme. Optionale und zusammengesetzte Modi stehen in der jeweiligen Zeile; unbekannte Produktbefehle lassen den Inventory-Test fehlschlagen.

Die aktuellen Daten liefert `baha agent describe -o json` über `cli_coverage`. Nach Änderungen am Befehlsbaum wird die kanonische englische Referenz durch `BASEHARBOR_UPDATE_MACHINE_DOCS=1 go test ./cmd/baha -run TestMachineDocumentationTracksRegistryAndCommandTree` aktualisiert. Normale Testläufe weisen Drift zurück.

Die technische Registry und ihre Befehle, Flags, Operationskennungen und maschinenlesbaren Ergebnisse werden hier unverändert wiedergegeben. Die Klassifizierung und Begründungen sind Erläuterungen dazu.

| Befehl | Einordnung | CLI-JSON / strukturiertes Ergebnis | Operation / Tool | Begründung |
| --- | --- | --- | --- | --- |
| `baha` | Darstellung |  |  | Befehlsgruppe; unterstützten Unterbefehl aufrufen. |
| `baha agent` | Darstellung |  |  | Befehlsgruppe; unterstützten Unterbefehl aufrufen. |
| `baha agent describe` | Darstellung |  |  | Machine registry/discovery itself, rather than a product mutation. |
| `baha app` | Darstellung |  |  | Befehlsgruppe; unterstützten Unterbefehl aufrufen. |
| `baha app apply` | semantisch | CLI -o json / typisiertes MCP-Ergebnis | baseharbor.apply |  |
| `baha app backup` | semantisch | CLI -o json / typisiertes MCP-Ergebnis | baseharbor.backup |  |
| `baha app create` | semantisch | CLI -o json / typisiertes MCP-Ergebnis | baseharbor.app.create |  |
| `baha app creds` | semantisch | CLI -o json / typisiertes MCP-Ergebnis | baseharbor.app.connection | MCP exposes connection metadata without passwords or credential-bearing URIs; CLI credential reveal is operator-local and excluded from MCP. |
| `baha app destroy` | semantisch | CLI -o json / typisiertes MCP-Ergebnis | baseharbor.destroy |  |
| `baha app doctor` | semantisch | CLI -o json / typisiertes MCP-Ergebnis | baseharbor.doctor |  |
| `baha app down` | semantisch | CLI -o json / typisiertes MCP-Ergebnis | baseharbor.app.stop |  |
| `baha app env` | semantisch | CLI --format json (masked) / typed MCP result | baseharbor.app.environment | MCP exposes masked values only. --reveal and loading the protected local dotenv file are execution-host credential access and are explicitly excluded modes. |
| `baha app evidence` | semantisch | CLI -o json / typisiertes MCP-Ergebnis | baseharbor.evidence |  |
| `baha app exec` | ausgeschlossen |  |  | Arbitrary process execution is intentionally excluded: #787 forbids generic exec passthrough; use typed lifecycle and observation tools. |
| `baha app init` | semantisch | CLI -o json / typisiertes MCP-Ergebnis | baseharbor.app.adopt | New repository intent uses app.adopt; an existing repository uses app.configure. Interactive suggestions are presentation only. |
| `baha app inspect` | semantisch | CLI -o json / typisiertes MCP-Ergebnis | baseharbor.inspect |  |
| `baha app list` | semantisch | CLI -o json / typisiertes MCP-Ergebnis | baseharbor.app.list |  |
| `baha app logs` | ausgeschlossen |  |  | Raw/follow runtime streams can include application secrets and require a streaming transport; status, doctor and evidence provide bounded secret-safe observation. |
| `baha app new` | semantisch | CLI -o json / typisiertes MCP-Ergebnis | baseharbor.app.new |  |
| `baha app plan` | semantisch | CLI -o json / typisiertes MCP-Ergebnis | baseharbor.plan |  |
| `baha app preflight` | semantisch | CLI -o json / typisiertes MCP-Ergebnis | baseharbor.app.preflight |  |
| `baha app psql` | ausgeschlossen |  |  | Interactive database terminal and raw SQL passthrough are excluded; app.connection supplies secret-safe connection metadata for an operator-owned client. |
| `baha app redis` | ausgeschlossen |  |  | Interactive database terminal and arbitrary command passthrough are excluded; app.connection supplies secret-safe connection metadata for an operator-owned client. |
| `baha app restore` | semantisch | CLI -o json / typisiertes MCP-Ergebnis | baseharbor.restore |  |
| `baha app runtime-identity` | Darstellung |  |  | Befehlsgruppe; unterstützten Unterbefehl aufrufen. |
| `baha app runtime-identity revoke` | semantisch | CLI -o json / typisiertes MCP-Ergebnis | baseharbor.runtime-identity.revoke |  |
| `baha app runtime-identity rotate` | semantisch | CLI -o json / typisiertes MCP-Ergebnis | baseharbor.runtime-identity.rotate |  |
| `baha app secret` | Darstellung |  |  | Befehlsgruppe; unterstützten Unterbefehl aufrufen. |
| `baha app secret delete` | semantisch | CLI -o json / typisiertes MCP-Ergebnis | baseharbor.secret.delete |  |
| `baha app secret list` | semantisch | CLI -o json / typisiertes MCP-Ergebnis | baseharbor.secret.list |  |
| `baha app secret set` | semantisch | CLI -o json / typisiertes MCP-Ergebnis | baseharbor.secret.set |  |
| `baha app secret tls-set` | semantisch | CLI -o json / typisiertes MCP-Ergebnis | baseharbor.secret.tls-set |  |
| `baha app shell` | ausgeschlossen |  |  | Interactive container terminal is host/TTY dependent and would grant arbitrary exec; use typed lifecycle and app.environment instead. |
| `baha app show` | semantisch | CLI -o json / typisiertes MCP-Ergebnis | baseharbor.app.show |  |
| `baha app status` | semantisch | CLI -o json / typisiertes MCP-Ergebnis | baseharbor.status |  |
| `baha app tls` | Darstellung |  |  | Befehlsgruppe; unterstützten Unterbefehl aufrufen. |
| `baha app tls update` | semantisch | CLI -o json / typisiertes MCP-Ergebnis | baseharbor.tls.update |  |
| `baha app up` | semantisch | CLI -o json / typisiertes MCP-Ergebnis | baseharbor.apply |  |
| `baha app update` | semantisch | CLI -o json / typisiertes MCP-Ergebnis | baseharbor.update |  |
| `baha app valkey` | alias |  |  | Canonical command: baha app redis; shares its coverage classification. |
| `baha app workspace` | Darstellung |  |  | Interactive presentation combining workspace.init and workspace.map; clients invoke those typed operations explicitly. |
| `baha app workspace init` | semantisch | CLI -o json / typisiertes MCP-Ergebnis | baseharbor.workspace.init |  |
| `baha app workspace map` | semantisch | CLI -o json / typisiertes MCP-Ergebnis | baseharbor.workspace.map |  |
| `baha app workspace resolve` | semantisch | CLI -o json / typisiertes MCP-Ergebnis | baseharbor.workspace.resolve |  |
| `baha app workspace show` | semantisch | CLI -o json / typisiertes MCP-Ergebnis | baseharbor.workspace.show |  |
| `baha app workspace status` | semantisch | CLI -o json / typisiertes MCP-Ergebnis | baseharbor.workspace.status |  |
| `baha app workspace update` | semantisch | CLI -o json / typisiertes MCP-Ergebnis | baseharbor.workspace.update |  |
| `baha backup` | semantisch | CLI -o json / typisiertes MCP-Ergebnis | baseharbor.backup |  |
| `baha completion` | Darstellung |  |  | Shell completion emits client-local shell text; semantic discovery is agent describe. |
| `baha config` | Darstellung |  |  | Befehlsgruppe; unterstützten Unterbefehl aufrufen. |
| `baha config organization` | Darstellung |  |  | Befehlsgruppe; unterstützten Unterbefehl aufrufen. |
| `baha config organization check` | semantisch | CLI -o json / typisiertes MCP-Ergebnis | baseharbor.organization.check |  |
| `baha config organization set` | semantisch | CLI -o json / typisiertes MCP-Ergebnis | baseharbor.organization.set |  |
| `baha config organization show` | semantisch | CLI -o json / typisiertes MCP-Ergebnis | baseharbor.organization.inspect |  |
| `baha config organization update` | semantisch | CLI -o json / typisiertes MCP-Ergebnis | baseharbor.organization.update |  |
| `baha config prompt` | Darstellung |  |  | Client-local shell prompt display preferences; explicit target arguments and target inspection provide semantic context. |
| `baha connect` | semantisch | CLI -o json / typisiertes MCP-Ergebnis | baseharbor.connectivity.connect |  |
| `baha connections` | semantisch | CLI -o json / typisiertes MCP-Ergebnis | baseharbor.connectivity.list |  |
| `baha destroy` | semantisch | CLI -o json / typisiertes MCP-Ergebnis | baseharbor.control-plane.destroy | Control-plane mode maps here; repository application mode uses destroy. --all maps to installation.destroy after authorization of every discovered deployment. All destruction retains ownership checks and explicit approval. |
| `baha dev` | Darstellung |  |  | Befehlsgruppe; unterstützten Unterbefehl aufrufen. |
| `baha dev credentials` | semantisch | CLI -o json / typisiertes MCP-Ergebnis | baseharbor.dev.credentials | Ensure/reset/configuration use the shared authority. MCP returns username and protected file reference; CLI password display is an excluded operator-local mode. |
| `baha dev domain` | semantisch | CLI -o json / typisiertes MCP-Ergebnis | baseharbor.dev.domain |  |
| `baha disconnect` | semantisch | CLI -o json / typisiertes MCP-Ergebnis | baseharbor.connectivity.disconnect |  |
| `baha doctor` | semantisch | CLI -o json / typisiertes MCP-Ergebnis | baseharbor.control-plane.doctor | Control-plane mode maps here; --fix uses control-plane.repair and rechecks readiness. Repository mode uses doctor or repair. |
| `baha down` | semantisch | CLI -o json / typisiertes MCP-Ergebnis | baseharbor.control-plane.stop | Control-plane mode maps here; repository application mode uses app.stop. |
| `baha init` | semantisch | CLI -o json / typisiertes MCP-Ergebnis | baseharbor.app.adopt |  |
| `baha inspect` | semantisch | CLI -o json / typisiertes MCP-Ergebnis | baseharbor.inspect |  |
| `baha list` | semantisch | CLI -o json / typisiertes MCP-Ergebnis | baseharbor.app.list |  |
| `baha login` | ausgeschlossen |  |  | Authorization Code/PKCE browser handoff establishes the operator's local protected session before semantic requests; operator.identity verifies that boundary without returning tokens. |
| `baha logout` | ausgeschlossen |  |  | Removes a client-local protected login session; performed by the authentication client, outside server product actions. operator.identity reports the resulting boundary. |
| `baha mcp` | Darstellung |  |  | Befehlsgruppe; unterstützten Unterbefehl aufrufen. |
| `baha mcp serve` | Darstellung |  |  | Starts the local MCP transport; clients launch it before discovery. |
| `baha new` | Darstellung |  |  | Human creation chooser delegating to existing app.new, stack.create, target.create, provider.init and workspace operations. |
| `baha open` | Darstellung |  |  | Host browser presentation over validated application/TLS state; typed clients inspect app status and TLS instead. |
| `baha openbao` | Darstellung |  |  | Befehlsgruppe; unterstützten Unterbefehl aufrufen. |
| `baha openbao bootstrap` | semantisch | CLI -o json / typisiertes MCP-Ergebnis | baseharbor.openbao.bootstrap |  |
| `baha openbao rotate` | semantisch | CLI -o json / typisiertes MCP-Ergebnis | baseharbor.openbao.rotate |  |
| `baha openbao status` | semantisch | CLI -o json / typisiertes MCP-Ergebnis | baseharbor.openbao.status |  |
| `baha openbao unseal` | semantisch | CLI -o json / typisiertes MCP-Ergebnis | baseharbor.openbao.unseal |  |
| `baha plan` | semantisch | CLI -o json / typisiertes MCP-Ergebnis | baseharbor.plan |  |
| `baha policy` | Darstellung |  |  | Befehlsgruppe; unterstützten Unterbefehl aufrufen. |
| `baha policy check` | semantisch | CLI -o json / typisiertes MCP-Ergebnis | baseharbor.policy.check |  |
| `baha policy explain` | semantisch | CLI -o json / typisiertes MCP-Ergebnis | baseharbor.policy.explain |  |
| `baha provider` | Darstellung |  |  | Befehlsgruppe; unterstützten Unterbefehl aufrufen. |
| `baha provider add` | semantisch | CLI -o json / typisiertes MCP-Ergebnis | baseharbor.provider.add |  |
| `baha provider init` | semantisch | CLI -o json / typisiertes MCP-Ergebnis | baseharbor.provider.init |  |
| `baha provider inspect` | semantisch | CLI -o json / typisiertes MCP-Ergebnis | baseharbor.provider.inspect |  |
| `baha provider list` | semantisch | CLI -o json / typisiertes MCP-Ergebnis | baseharbor.provider.list |  |
| `baha provider remove` | semantisch | CLI -o json / typisiertes MCP-Ergebnis | baseharbor.provider.remove |  |
| `baha provider test` | semantisch | CLI -o json / typisiertes MCP-Ergebnis | baseharbor.provider.test |  |
| `baha provider verify` | semantisch | CLI -o json / typisiertes MCP-Ergebnis | baseharbor.provider.verify |  |
| `baha restore` | semantisch | CLI -o json / typisiertes MCP-Ergebnis | baseharbor.restore |  |
| `baha serve` | ausgeschlossen |  |  | Starts a long-lived TLS API/broker transport using host-held credentials; transport supervision belongs to the host, typed lifecycle tools operate managed resources. |
| `baha shell-init` | Darstellung |  |  | Shell-lokale Hilfsfunktionen; die ausdrückliche Target-Auswahl bleibt Produktfunktionalität. |
| `baha stack` | Darstellung |  |  | Befehlsgruppe; unterstützten Unterbefehl aufrufen. |
| `baha stack create` | semantisch | CLI -o json / typisiertes MCP-Ergebnis | baseharbor.stack.create |  |
| `baha stack list` | semantisch | CLI -o json / typisiertes MCP-Ergebnis | baseharbor.stack.list |  |
| `baha stack show` | semantisch | CLI -o json / typisiertes MCP-Ergebnis | baseharbor.stack.show |  |
| `baha status` | semantisch | CLI -o json / typisiertes MCP-Ergebnis | baseharbor.control-plane.status | Control-plane mode maps here; repository application mode uses status. |
| `baha target` | semantisch | CLI -o json / typisiertes MCP-Ergebnis | baseharbor.target |  |
| `baha target activate` | Darstellung |  |  | Emits shell-local selection only; machine clients supply the explicit target argument on each semantic operation. |
| `baha target create` | semantisch | CLI -o json / typisiertes MCP-Ergebnis | baseharbor.target.create |  |
| `baha target deactivate` | Darstellung |  |  | Clears shell-local selection only; machine clients omit an explicit target to resolve configured defaults. |
| `baha target delete` | semantisch | CLI -o json / typisiertes MCP-Ergebnis | baseharbor.target.delete |  |
| `baha target list` | semantisch | CLI -o json / typisiertes MCP-Ergebnis | baseharbor.target.list |  |
| `baha target show` | semantisch | CLI -o json / typisiertes MCP-Ergebnis | baseharbor.target |  |
| `baha trust` | Darstellung |  |  | Befehlsgruppe; unterstützten Unterbefehl aufrufen. |
| `baha trust export` | semantisch | CLI --json (--output is CA destination) / typed MCP result | baseharbor.trust.export |  |
| `baha trust install` | semantisch | CLI -o json / typisiertes MCP-Ergebnis | baseharbor.trust.install |  |
| `baha trust status` | semantisch | CLI -o json / typisiertes MCP-Ergebnis | baseharbor.trust.status |  |
| `baha tui` | Darstellung |  |  | Interaktive Oberfläche; ihre Aktionen benötigen jeweils eine semantische Abdeckung. |
| `baha up` | semantisch | CLI -o json / typisiertes MCP-Ergebnis | baseharbor.control-plane.up | Control-plane-only mode maps here. Repository application mode additionally uses app.configure and apply; host trust installation uses trust.install with approval. |
| `baha update` | semantisch | CLI -o json / typisiertes MCP-Ergebnis | baseharbor.release.check | --check maps to release.check. Installing replaces the execution-host binary and is explicitly excluded from MCP; use the host operator CLI with --yes. |
| `baha version` | Darstellung |  |  | Die Build-Identität liefert agent describe beziehungsweise die MCP-Initialisierung. |
| `baha whoami` | semantisch | CLI -o json / typisiertes MCP-Ergebnis | baseharbor.operator.identity |  |
| `baha app cache` | ausgeschlossen |  |  | Interaktives Datenbankterminal und beliebige Befehlsweitergabe sind ausgeschlossen; app.connection liefert geheimnissichere Verbindungsmetadaten für einen operator-eigenen Client. |
| `baha app sql` | ausgeschlossen |  |  | Interaktives Datenbankterminal und rohe SQL-Weitergabe sind ausgeschlossen; app.connection liefert geheimnissichere Verbindungsmetadaten für einen operator-eigenen Client. |
| `baha node` | Darstellung |  |  | Befehlsgruppe; unterstützten Unterbefehl aufrufen. |
| `baha node add` | semantisch | CLI -o json / typisiertes MCP-Ergebnis | baseharbor.node.add |  |
| `baha node connect` | semantisch | CLI -o json / typisiertes MCP-Ergebnis | baseharbor.node.connect |  |
| `baha node disconnect` | semantisch | CLI -o json / typisiertes MCP-Ergebnis | baseharbor.node.disconnect |  |
| `baha node list` | semantisch | CLI -o json / typisiertes MCP-Ergebnis | baseharbor.node.list |  |
| `baha node status` | semantisch | CLI -o json / typisiertes MCP-Ergebnis | baseharbor.node.status |  |
| `baha trust uninstall` | semantisch | CLI -o json / typisiertes MCP-Ergebnis | baseharbor.trust.uninstall |  |
| `baha workspace` | Darstellung |  |  | Kanonische interaktive Darstellung derselben workspace.init/map-Operationen. |
| `baha workspace init` | semantisch | CLI -o json / typisiertes MCP-Ergebnis | baseharbor.workspace.init |  |
| `baha workspace map` | semantisch | CLI -o json / typisiertes MCP-Ergebnis | baseharbor.workspace.map |  |
| `baha workspace resolve` | semantisch | CLI -o json / typisiertes MCP-Ergebnis | baseharbor.workspace.resolve |  |
| `baha workspace show` | semantisch | CLI -o json / typisiertes MCP-Ergebnis | baseharbor.workspace.show |  |
| `baha workspace status` | semantisch | CLI -o json / typisiertes MCP-Ergebnis | baseharbor.workspace.status |  |
| `baha workspace update` | semantisch | CLI -o json / typisiertes MCP-Ergebnis | baseharbor.workspace.update |  |
| `baha use` | Darstellung |  |  | Speichert die interaktive Target-Auswahl wie `baha target activate`; Machine-Clients übergeben das Target explizit. |
