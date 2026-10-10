# CLI / Machine-Abdeckung

Diese Referenz wird aus dem tatsächlichen Befehlsbaum und der typisierten Operation Registry erzeugt. Jede Operation ist semantisch, ein Alias, eine Darstellung oder ein ausdrücklich begründeter Host-/Transport-Ausschluss. Die technischen Ausgaben und Begründungen der Registry bleiben unverändert.

Aktuelle Daten liefert `baha agent describe -o json` (`cli_coverage`). Nach Codeänderungen wird die Referenz mit `BASEHARBOR_UPDATE_MACHINE_DOCS=1 go test ./cmd/baha -run TestMachineDocumentationTracksRegistryAndCommandTree` aktualisiert. Reguläre Tests erkennen Abweichungen.

| Befehl | Einordnung | CLI JSON / strukturiertes Ergebnis | Operation / Tool | Begründung |
| --- | --- | --- | --- | --- |
| `baha` | Darstellung |  |  | Command group; invoke its supported subcommands. |
| `baha agent` | Darstellung |  |  | Command group; invoke its supported subcommands. |
| `baha agent describe` | Darstellung |  |  | Machine registry/discovery itself, rather than a product mutation. |
| `baha app` | Darstellung |  |  | Command group; invoke its supported subcommands. |
| `baha app apply` | semantisch | CLI -o json / typed MCP result | baseharbor.apply |  |
| `baha app backup` | semantisch | CLI -o json / typed MCP result | baseharbor.backup |  |
| `baha app cache` | ausgeschlossen |  |  | Interactive database terminal and arbitrary command passthrough are excluded; app.connection supplies secret-safe connection metadata for an operator-owned client. |
| `baha app create` | semantisch | CLI -o json / typed MCP result | baseharbor.app.create |  |
| `baha app creds` | semantisch | CLI -o json / typed MCP result | baseharbor.app.connection | MCP exposes connection metadata without passwords or credential-bearing URIs; CLI credential reveal is operator-local and excluded from MCP. |
| `baha app destroy` | semantisch | CLI -o json / typed MCP result | baseharbor.destroy |  |
| `baha app doctor` | semantisch | CLI -o json / typed MCP result | baseharbor.doctor |  |
| `baha app down` | semantisch | CLI -o json / typed MCP result | baseharbor.app.stop |  |
| `baha app env` | semantisch | CLI --format json (masked) / typed MCP result | baseharbor.app.environment | MCP exposes masked values only. --reveal and loading the protected local dotenv file are execution-host credential access and are explicitly excluded modes. |
| `baha app evidence` | semantisch | CLI -o json / typed MCP result | baseharbor.evidence |  |
| `baha app exec` | ausgeschlossen |  |  | Arbitrary process execution is intentionally excluded: #787 forbids generic exec passthrough; use typed lifecycle and observation tools. |
| `baha app init` | semantisch | CLI -o json / typed MCP result | baseharbor.app.adopt | New repository intent uses app.adopt; an existing repository uses app.configure. Interactive suggestions are presentation only. |
| `baha app inspect` | semantisch | CLI -o json / typed MCP result | baseharbor.inspect |  |
| `baha app list` | semantisch | CLI -o json / typed MCP result | baseharbor.app.list |  |
| `baha app logs` | ausgeschlossen |  |  | Raw/follow runtime streams can include application secrets and require a streaming transport; status, doctor and evidence provide bounded secret-safe observation. |
| `baha app new` | semantisch | CLI -o json / typed MCP result | baseharbor.app.new |  |
| `baha app plan` | semantisch | CLI -o json / typed MCP result | baseharbor.plan |  |
| `baha app preflight` | semantisch | CLI -o json / typed MCP result | baseharbor.app.preflight |  |
| `baha app restore` | semantisch | CLI -o json / typed MCP result | baseharbor.restore |  |
| `baha app runtime-identity` | Darstellung |  |  | Command group; invoke its supported subcommands. |
| `baha app runtime-identity revoke` | semantisch | CLI -o json / typed MCP result | baseharbor.runtime-identity.revoke |  |
| `baha app runtime-identity rotate` | semantisch | CLI -o json / typed MCP result | baseharbor.runtime-identity.rotate |  |
| `baha app secret` | Darstellung |  |  | Command group; invoke its supported subcommands. |
| `baha app secret delete` | semantisch | CLI -o json / typed MCP result | baseharbor.secret.delete |  |
| `baha app secret list` | semantisch | CLI -o json / typed MCP result | baseharbor.secret.list |  |
| `baha app secret set` | semantisch | CLI -o json / typed MCP result | baseharbor.secret.set |  |
| `baha app secret tls-set` | semantisch | CLI -o json / typed MCP result | baseharbor.secret.tls-set |  |
| `baha app shell` | ausgeschlossen |  |  | Interactive container terminal is host/TTY dependent and would grant arbitrary exec; use typed lifecycle and app.environment instead. |
| `baha app show` | semantisch | CLI -o json / typed MCP result | baseharbor.app.show |  |
| `baha app sql` | ausgeschlossen |  |  | Interactive database terminal and raw SQL passthrough are excluded; app.connection supplies secret-safe connection metadata for an operator-owned client. |
| `baha app status` | semantisch | CLI -o json / typed MCP result | baseharbor.status |  |
| `baha app tls` | Darstellung |  |  | Command group; invoke its supported subcommands. |
| `baha app tls update` | semantisch | CLI -o json / typed MCP result | baseharbor.tls.update |  |
| `baha app up` | semantisch | CLI -o json / typed MCP result | baseharbor.apply |  |
| `baha app update` | semantisch | CLI -o json / typed MCP result | baseharbor.update |  |
| `baha app workspace` | Darstellung |  |  | Interactive presentation combining workspace.init and workspace.map; clients invoke those typed operations explicitly. |
| `baha app workspace init` | semantisch | CLI -o json / typed MCP result | baseharbor.workspace.init |  |
| `baha app workspace map` | semantisch | CLI -o json / typed MCP result | baseharbor.workspace.map |  |
| `baha app workspace resolve` | semantisch | CLI -o json / typed MCP result | baseharbor.workspace.resolve |  |
| `baha app workspace show` | semantisch | CLI -o json / typed MCP result | baseharbor.workspace.show |  |
| `baha app workspace status` | semantisch | CLI -o json / typed MCP result | baseharbor.workspace.status |  |
| `baha app workspace update` | semantisch | CLI -o json / typed MCP result | baseharbor.workspace.update |  |
| `baha backup` | semantisch | CLI -o json / typed MCP result | baseharbor.backup |  |
| `baha completion` | Darstellung |  |  | Shell completion emits client-local shell text; semantic discovery is agent describe. |
| `baha config` | Darstellung |  |  | Command group; invoke its supported subcommands. |
| `baha config organization` | Darstellung |  |  | Command group; invoke its supported subcommands. |
| `baha config organization check` | semantisch | CLI -o json / typed MCP result | baseharbor.organization.check |  |
| `baha config organization set` | semantisch | CLI -o json / typed MCP result | baseharbor.organization.set |  |
| `baha config organization show` | semantisch | CLI -o json / typed MCP result | baseharbor.organization.inspect |  |
| `baha config organization update` | semantisch | CLI -o json / typed MCP result | baseharbor.organization.update |  |
| `baha config prompt` | Darstellung |  |  | Client-local shell prompt display preferences; explicit target arguments and target inspection provide semantic context. |
| `baha connect` | semantisch | CLI -o json / typed MCP result | baseharbor.connectivity.connect |  |
| `baha connections` | semantisch | CLI -o json / typed MCP result | baseharbor.connectivity.list |  |
| `baha destroy` | semantisch | CLI -o json / typed MCP result | baseharbor.control-plane.destroy | Control-plane mode maps here; repository application mode uses destroy. --all maps to installation.destroy after authorization of every discovered deployment. All destruction retains ownership checks and explicit approval. |
| `baha dev` | Darstellung |  |  | Command group; invoke its supported subcommands. |
| `baha dev credentials` | semantisch | CLI -o json / typed MCP result | baseharbor.dev.credentials | Ensure/reset/configuration use the shared authority. MCP returns username and protected file reference; CLI password display is an excluded operator-local mode. |
| `baha dev domain` | semantisch | CLI -o json / typed MCP result | baseharbor.dev.domain |  |
| `baha disconnect` | semantisch | CLI -o json / typed MCP result | baseharbor.connectivity.disconnect |  |
| `baha doctor` | semantisch | CLI -o json / typed MCP result | baseharbor.control-plane.doctor | Control-plane mode maps here; --fix uses control-plane.repair and rechecks readiness. Repository mode uses doctor or repair. |
| `baha down` | semantisch | CLI -o json / typed MCP result | baseharbor.control-plane.stop | Control-plane mode maps here; repository application mode uses app.stop. |
| `baha init` | semantisch | CLI -o json / typed MCP result | baseharbor.app.adopt |  |
| `baha inspect` | semantisch | CLI -o json / typed MCP result | baseharbor.inspect |  |
| `baha list` | semantisch | CLI -o json / typed MCP result | baseharbor.app.list |  |
| `baha login` | ausgeschlossen |  |  | Authorization Code/PKCE browser handoff establishes the operator's local protected session before semantic requests; operator.identity verifies that boundary without returning tokens. |
| `baha logout` | ausgeschlossen |  |  | Removes a client-local protected login session; performed by the authentication client, outside server product actions. operator.identity reports the resulting boundary. |
| `baha mcp` | Darstellung |  |  | Command group; invoke its supported subcommands. |
| `baha mcp serve` | Darstellung |  |  | Starts the local MCP transport; clients launch it before discovery. |
| `baha new` | Darstellung |  |  | Human creation chooser delegating to existing app.new, stack.create, target.create, provider.init and workspace operations. |
| `baha node` | Darstellung |  |  | Command group; invoke its supported subcommands. |
| `baha node add` | semantisch | CLI -o json / typed MCP result | baseharbor.node.add |  |
| `baha node connect` | semantisch | CLI -o json / typed MCP result | baseharbor.node.connect |  |
| `baha node disconnect` | semantisch | CLI -o json / typed MCP result | baseharbor.node.disconnect |  |
| `baha node list` | semantisch | CLI -o json / typed MCP result | baseharbor.node.list |  |
| `baha node status` | semantisch | CLI -o json / typed MCP result | baseharbor.node.status |  |
| `baha open` | Darstellung |  |  | Host browser presentation over validated application/TLS state; typed clients inspect app status and TLS instead. |
| `baha openbao` | Darstellung |  |  | Command group; invoke its supported subcommands. |
| `baha openbao bootstrap` | semantisch | CLI -o json / typed MCP result | baseharbor.openbao.bootstrap |  |
| `baha openbao rotate` | semantisch | CLI -o json / typed MCP result | baseharbor.openbao.rotate |  |
| `baha openbao status` | semantisch | CLI -o json / typed MCP result | baseharbor.openbao.status |  |
| `baha openbao unseal` | semantisch | CLI -o json / typed MCP result | baseharbor.openbao.unseal |  |
| `baha plan` | semantisch | CLI -o json / typed MCP result | baseharbor.plan |  |
| `baha policy` | Darstellung |  |  | Command group; invoke its supported subcommands. |
| `baha policy check` | semantisch | CLI -o json / typed MCP result | baseharbor.policy.check |  |
| `baha policy explain` | semantisch | CLI -o json / typed MCP result | baseharbor.policy.explain |  |
| `baha provider` | Darstellung |  |  | Command group; invoke its supported subcommands. |
| `baha provider add` | semantisch | CLI -o json / typed MCP result | baseharbor.provider.add |  |
| `baha provider init` | semantisch | CLI -o json / typed MCP result | baseharbor.provider.init |  |
| `baha provider inspect` | semantisch | CLI -o json / typed MCP result | baseharbor.provider.inspect |  |
| `baha provider list` | semantisch | CLI -o json / typed MCP result | baseharbor.provider.list |  |
| `baha provider remove` | semantisch | CLI -o json / typed MCP result | baseharbor.provider.remove |  |
| `baha provider test` | semantisch | CLI -o json / typed MCP result | baseharbor.provider.test |  |
| `baha provider verify` | semantisch | CLI -o json / typed MCP result | baseharbor.provider.verify |  |
| `baha restore` | semantisch | CLI -o json / typed MCP result | baseharbor.restore |  |
| `baha serve` | ausgeschlossen |  |  | Starts a long-lived TLS API/broker transport using host-held credentials; transport supervision belongs to the host, typed lifecycle tools operate managed resources. |
| `baha shell-init` | Darstellung |  |  | Shell-local helpers; explicit Target selection remains product functionality. |
| `baha stack` | Darstellung |  |  | Command group; invoke its supported subcommands. |
| `baha stack create` | semantisch | CLI -o json / typed MCP result | baseharbor.stack.create |  |
| `baha stack list` | semantisch | CLI -o json / typed MCP result | baseharbor.stack.list |  |
| `baha stack show` | semantisch | CLI -o json / typed MCP result | baseharbor.stack.show |  |
| `baha status` | semantisch | CLI -o json / typed MCP result | baseharbor.control-plane.status | Control-plane mode maps here; repository application mode uses status. |
| `baha target` | semantisch | CLI -o json / typed MCP result | baseharbor.target |  |
| `baha target activate` | Darstellung |  |  | Persists local user selection; machine clients supply explicit target arguments on each semantic operation. |
| `baha target create` | semantisch | CLI -o json / typed MCP result | baseharbor.target.create |  |
| `baha target deactivate` | Darstellung |  |  | Clears persisted local user selection; machine clients omit explicit target to resolve configured defaults. |
| `baha target delete` | semantisch | CLI -o json / typed MCP result | baseharbor.target.delete |  |
| `baha target list` | semantisch | CLI -o json / typed MCP result | baseharbor.target.list |  |
| `baha target show` | semantisch | CLI -o json / typed MCP result | baseharbor.target |  |
| `baha trust` | Darstellung |  |  | Command group; invoke its supported subcommands. |
| `baha trust export` | semantisch | CLI --json (--output is CA destination) / typed MCP result | baseharbor.trust.export |  |
| `baha trust install` | semantisch | CLI -o json / typed MCP result | baseharbor.trust.install |  |
| `baha trust status` | semantisch | CLI -o json / typed MCP result | baseharbor.trust.status |  |
| `baha trust uninstall` | semantisch | CLI -o json / typed MCP result | baseharbor.trust.uninstall |  |
| `baha tui` | Darstellung |  |  | Human interactive presentation; its product actions require individual semantic coverage. |
| `baha up` | semantisch | CLI -o json / typed MCP result | baseharbor.control-plane.up | Control-plane-only mode maps here. Repository application mode additionally uses app.configure and apply; host trust installation uses trust.install with approval. |
| `baha update` | semantisch | CLI -o json / typed MCP result | baseharbor.release.check | --check maps to release.check. Installing replaces the execution-host binary and is explicitly excluded from MCP; use the host operator CLI with --yes. |
| `baha use` | Darstellung |  |  | Persists local user selection; machine clients supply explicit target arguments on each semantic operation. |
| `baha version` | Darstellung |  |  | Executable build identity is provided by agent describe and MCP initialization. |
| `baha whoami` | semantisch | CLI -o json / typed MCP result | baseharbor.operator.identity |  |
| `baha workspace` | Darstellung |  |  | Canonical interactive presentation of the same workspace.init/map operations. |
| `baha workspace init` | semantisch | CLI -o json / typed MCP result | baseharbor.workspace.init |  |
| `baha workspace map` | semantisch | CLI -o json / typed MCP result | baseharbor.workspace.map |  |
| `baha workspace resolve` | semantisch | CLI -o json / typed MCP result | baseharbor.workspace.resolve |  |
| `baha workspace show` | semantisch | CLI -o json / typed MCP result | baseharbor.workspace.show |  |
| `baha workspace status` | semantisch | CLI -o json / typed MCP result | baseharbor.workspace.status |  |
| `baha workspace update` | semantisch | CLI -o json / typed MCP result | baseharbor.workspace.update |  |
