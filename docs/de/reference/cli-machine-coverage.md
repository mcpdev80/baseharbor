# CLI / Machine-Abdeckung

Diese Referenz wird aus dem tatsächlichen Befehlsbaum und der typisierten Operation Registry erzeugt. Jede Operation ist semantisch, ein Alias, eine Darstellung oder ein ausdrücklich begründeter Host-/Transport-Ausschluss. Die technischen Ausgaben und Begründungen der Registry bleiben unverändert.

Aktuelle Daten liefert `baha agent describe -o json` (`cli_coverage`). Nach Codeänderungen wird die Referenz mit `BASEHARBOR_UPDATE_MACHINE_DOCS=1 go test ./cmd/baha -run TestMachineDocumentationTracksRegistryAndCommandTree` aktualisiert. Reguläre Tests erkennen Abweichungen.

| Befehl | Einordnung | CLI JSON / strukturiertes Ergebnis | Operation / Tool | Begründung |
| --- | --- | --- | --- | --- |
| `baha` | Darstellung |  |  | Command group; invoke its supported subcommands. |
| `baha access` | Darstellung |  |  | Command group; invoke its supported subcommands. |
| `baha access assign` | ausgeschlossen |  |  | Guided, human interactive profile assignment folds every decision into the single production policy authority (policy.Aggregate/policy.ResolveFinding); no typed MCP operation, so excluded from the machine tool boundary by design. |
| `baha access offboard` | ausgeschlossen |  |  | Guided, human interactive offboarding re-checks the current configuration at submit time and folds every decision into the single production policy authority; no typed MCP operation, so excluded from the machine tool boundary by design. |
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
| `baha app stop` | semantisch | CLI -o json / typed MCP result | baseharbor.app.stop |  |
| `baha app edit` | semantisch | Interactive CLI / typed two-phase MCP consent | baseharbor.app.edit |  |
| `baha app env` | semantisch | CLI --format json (masked) / typed MCP result | baseharbor.app.environment | MCP exposes masked values only. --reveal and loading the protected local dotenv file are execution-host credential access and are explicitly excluded modes. |
| `baha app evidence` | semantisch | CLI -o json / typed MCP result | baseharbor.evidence |  |
| `baha app exec` | ausgeschlossen |  |  | Arbitrary process execution is intentionally excluded: #787 forbids generic exec passthrough; use typed lifecycle and observation tools. |
| `baha app export` | Darstellung |  |  | Operator-local, password-locked file export of explicitly selected local credentials; no runtime mutation or machine tool, and fails closed on an empty selection. Ciphertext opens only with the export password; managed-secret sources require the human-authorization reveal seam. |
| `baha app footprint` | Darstellung |  |  | Command group; invoke its supported subcommands. |
| `baha app footprint show` | semantisch | CLI -o json / typed MCP result | baseharbor.app.footprint.show |  |
| `baha app init` | semantisch | CLI -o json / typed MCP result | baseharbor.app.adopt | New repository intent uses app.adopt; an existing repository uses app.configure. Interactive suggestions are presentation only. |
| `baha app inspect` | semantisch | CLI -o json / typed MCP result | baseharbor.inspect |  |
| `baha app list` | semantisch | CLI -o json / typed MCP result | baseharbor.app.list |  |
| `baha app logs` | ausgeschlossen |  |  | Raw/follow runtime streams can include application secrets and require a streaming transport; status, doctor and evidence provide bounded secret-safe observation. |
| `baha app new` | semantisch | CLI -o json / typed MCP result | baseharbor.app.new |  |
| `baha app plan` | semantisch | CLI -o json / typed MCP result | baseharbor.plan |  |
| `baha app perses` | praesentation |  |  | Command group; invoke its supported subcommands. |
| `baha app perses disable` | semantisch | CLI -o json / typed MCP result | baseharbor.app.perses.disable |  |
| `baha app perses enable` | semantisch | CLI -o json / typed MCP result | baseharbor.app.perses.enable |  |
| `baha app perses remove` | semantisch | CLI -o json / typed MCP result | baseharbor.app.perses.remove |  |
| `baha app perses status` | semantisch | CLI -o json / typed MCP result | baseharbor.app.perses.status |  |
| `baha app preflight` | semantisch | CLI -o json / typed MCP result | baseharbor.app.preflight |  |
| `baha app prune` | semantisch | CLI -o json / typed MCP result | baseharbor.app.prune |  |
| `baha app pull` | semantisch | CLI -o json / typed MCP result | baseharbor.app.pull |  |
| `baha app perses` | Darstellung |  |  | Command group; invoke its supported subcommands. |
| `baha app perses disable` | Darstellung |  |  | Guided human opt-out of the optional Perses surface; it folds into the manifest services.observability selection and the apply/preflight flow, which carry their own typed coverage, so no separate MCP operation is exposed. |
| `baha app perses enable` | Darstellung |  |  | Guided human enablement of the optional Perses surface; it folds into the manifest services.observability selection and apply/preflight flow, which carry their own typed coverage, so no separate MCP operation is exposed. |
| `baha app perses remove` | Darstellung |  |  | Guided human retirement of the owned Perses surface; manifest convergence plus an ownership-safe removal plan whose runtime retirement lands in the release window; no separate MCP operation is exposed. |
| `baha app perses status` | Darstellung |  |  | Repository-bound presentation of the app.perses.status read model for the current deployment; machine clients inspect app status instead. |
| `baha app restore` | semantisch | CLI -o json / typed MCP result | baseharbor.restore |  |
| `baha app runtime-identity` | Darstellung |  |  | Command group; invoke its supported subcommands. |
| `baha app runtime-identity revoke` | semantisch | CLI -o json / typed MCP result | baseharbor.runtime-identity.revoke |  |
| `baha app runtime-identity rotate` | semantisch | CLI -o json / typed MCP result | baseharbor.runtime-identity.rotate |  |
| `baha app secret` | Darstellung |  |  | Command group; invoke its supported subcommands. |
| `baha app secret delete` | semantisch | CLI -o json / typed MCP result | baseharbor.secret.delete |  |
| `baha app secret list` | semantisch | CLI -o json / typed MCP result | baseharbor.secret.list |  |
| `baha app secret set` | semantisch | CLI -o json / typed MCP result | baseharbor.secret.set |  |
| `baha app secret tls-set` | semantisch | CLI -o json / typed MCP result | baseharbor.secret.tls-set |  |
| `baha app secure-profile` | Darstellung |  |  | Command group; invoke its supported subcommands. |
| `baha app secure-profile export` | Darstellung |  |  | Operator-local, password-locked file export of opt-in local credentials; no runtime mutation or machine tool. Ciphertext opens only with the export password. |
| `baha app shell` | ausgeschlossen |  |  | Interactive container terminal is host/TTY dependent and would grant arbitrary exec; use typed lifecycle and app.environment instead. |
| `baha app show` | semantisch | CLI -o json / typed MCP result | baseharbor.app.show |  |
| `baha app sql` | ausgeschlossen |  |  | Interactive database terminal and raw SQL passthrough are excluded; app.connection supplies secret-safe connection metadata for an operator-owned client. |
| `baha app status` | semantisch | CLI -o json / typed MCP result | baseharbor.status |  |
| `baha app tls` | Darstellung |  |  | Command group; invoke its supported subcommands. |
| `baha app tls update` | semantisch | CLI -o json / typed MCP result | baseharbor.tls.update |  |
| `baha app up` | semantisch | CLI -o json / typed MCP result | baseharbor.apply |  |
| `baha app urls` | ausgeschlossen |  |  | Listet nur die verifizierten URLs und GUI-URLs gebundener Dienste des Status-Read-Modells; typisierte Clients prüfen app.status und TLS-Zustand. Das Öffnen im Browser bleibt die ausgeschlossene menschliche Naht 'baha open'. |
| `baha app update` | semantisch | CLI -o json / typed MCP result | baseharbor.update |  |
| `baha app version` | Darstellung |  |  | Repository-bound presentation of the app.versions read model for the current deployment; machine clients invoke app.versions with explicit identity. |
| `baha app versions` | semantisch | CLI -o json / typed MCP result | baseharbor.app.versions |  |
| `baha app workspace` | Darstellung |  |  | Interactive presentation combining workspace.init and workspace.map; clients invoke those typed operations explicitly. |
| `baha app workspace component` | presentation |  |  | Source-model component maintenance group; presentation over the portable source model that workspace.init/map/show/resolve read and write. |
| `baha app workspace component add` | presentation |  |  | Edits the versioned source model through the mutating workspace.init contract. |
| `baha app workspace component list` | presentation |  |  | Reads the versioned source model through the workspace.show contract. |
| `baha app workspace component remove` | presentation |  |  | Edits the versioned source model through the mutating workspace.init contract. |
| `baha app workspace init` | semantisch | CLI -o json / typed MCP result | baseharbor.workspace.init |  |
| `baha app workspace map` | semantisch | CLI -o json / typed MCP result | baseharbor.workspace.map |  |
| `baha app workspace path` | presentation |  |  | Sets or reads a local checkout path in the versioned source model; writes require workspace.map authorization and never create or delete files. |
| `baha app workspace paths` | alias |  |  | Canonical command: baha app workspace path; shares its coverage classification. |
| `baha app workspace resolve` | semantisch | CLI -o json / typed MCP result | baseharbor.workspace.resolve |  |
| `baha app workspace show` | semantisch | CLI -o json / typed MCP result | baseharbor.workspace.show |  |
| `baha app workspace source` | presentation |  |  | Source-model maintenance group; presentation over the portable source model that workspace.init reads and writes. |
| `baha app workspace source add` | presentation |  |  | Edits the versioned source model through the mutating workspace.init contract. |
| `baha app workspace source remove` | presentation |  |  | Detaches source, mapping and component entries through the mutating workspace.init contract; no local file deletion. |
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
 | `baha context` | Darstellung |  |  | Command group; invoke its supported subcommands. |
 | `baha context clear` | Darstellung |  |  | Process-level unset of the per-terminal BASEHARBOR_TARGET plus a hint; nothing is persisted, so machine clients simply omit --target to resolve configured defaults. |
 | `baha context show` | Darstellung |  |  | Repository-bound presentation of the effective per-terminal selection and the winning precedence level (explicit flag, shell-env, or configured default); machine clients read the same decision through their own explicit-argument resolution. |
 | `baha context use` | Darstellung |  |  | Emits the shell snippet that sets BASEHARBOR_TARGET for the current shell process only; no on-disk state. The per-terminal value lives in the shell-process environment (carried via the completion hook); machine clients resolve the same precedence through explicit --target, the BASEHARBOR_TARGET env, or the configured default target. |
| `baha connect` | semantisch | CLI -o json / typed MCP result | baseharbor.connectivity.connect |  |
| `baha connections` | semantisch | CLI -o json / typed MCP result | baseharbor.connectivity.list |  |
| `baha destroy` | semantisch | CLI -o json / typed MCP result | baseharbor.installation.destroy | Entfernt die gesamte Installation. Entfernte Core-Targets werden über die authentifizierte Machine-API abgebaut; lokale Docker-/Podman-Targets behalten den nativen Ablauf. Autorisiert jedes gefundene Deployment und verlangt Eigentums- sowie Datenlöschungsbestätigung. |
| `baha dev` | Darstellung |  |  | Command group; invoke its supported subcommands. |
| `baha dev cleanup` | excluded |  |  | Workstation-local removal of owned hosts and CA trust, preserving other connections; explicit host operator approval required. No Core or application mutation. |
| `baha dev hosts` | excluded |  |  | Workstation-local hosts-file opt-in and ownership cleanup; cannot mutate the MCP server host on behalf of a remote client. Names come from authenticated control-plane.status metadata. |
| `baha dev credentials` | semantisch | CLI -o json / typed MCP result | baseharbor.dev.credentials | Ensure/reset/configuration use the shared authority. MCP returns username and protected file reference; CLI password display is an excluded operator-local mode. |
| `baha dev domain` | semantisch | CLI -o json / typed MCP result | baseharbor.dev.domain |  |
| `baha disconnect` | semantisch | CLI -o json / typed MCP result | baseharbor.connectivity.disconnect |  |
| `baha doctor` | semantisch | CLI -o json / typed MCP result | baseharbor.control-plane.doctor | Control-plane mode maps here; --fix uses control-plane.repair and rechecks readiness. Repository mode uses doctor or repair. |
| `baha down` | semantisch | CLI -o json / typed MCP result | baseharbor.control-plane.stop | Control-plane mode maps here; repository application mode uses app.stop. |
| `baha env` | Darstellung |  |  | Command group routing to env.show and env.destroy; invoke the supported subcommands explicitly. |
| `baha env destroy` | Darstellung |  |  | Environment-scoped identity removal with a CLI JSON impact plan; no machine tool yet (pending semantic MCP coverage). The --json surface exposes the plan. |
| `baha init` | semantisch | CLI -o json / typed MCP result | baseharbor.app.adopt |  |
| `baha inspect` | semantisch | CLI -o json / typed MCP result | baseharbor.inspect |  |
| `baha list` | semantisch | CLI -o json / typed MCP result | baseharbor.app.list |  |
| `baha login` | ausgeschlossen |  |  | Authorization Code/PKCE browser handoff establishes the operator's local protected session before semantic requests; operator.identity verifies that boundary without returning tokens. |
| `baha logout` | ausgeschlossen |  |  | Removes a client-local protected login session; performed by the authentication client, outside server product actions. operator.identity reports the resulting boundary. |
| `baha logs` | ausgeschlossen |  |  | Top-level shorthand for the excluded baha app logs: raw/follow runtime streams can include application secrets and require a streaming transport; status, doctor and evidence provide bounded secret-safe observation. |
| `baha mcp` | Darstellung |  |  | Command group; invoke its supported subcommands. |
| `baha mcp serve` | Darstellung |  |  | Starts the local MCP transport; clients launch it before discovery. |
| `baha new` | Darstellung |  |  | Human creation chooser delegating to existing app.new, stack.create, target.create, provider.init and workspace operations. |
| `baha node` | Darstellung |  |  | Command group; invoke its supported subcommands. |
| `baha node add` | semantisch | CLI -o json / typed MCP result | baseharbor.node.add |  |
| `baha node connect` | semantisch | CLI -o json / typed MCP result | baseharbor.node.connect |  |
| `baha node disconnect` | semantisch | CLI -o json / typed MCP result | baseharbor.node.disconnect |  |
| `baha node list` | semantisch | CLI -o json / typed MCP result | baseharbor.node.list |  |
| `baha node status` | semantisch | CLI -o json / typed MCP result | baseharbor.node.status |  |
| `baha onboard` | Darstellung |  |  | Guided workstation onboarding presentation; the underlying installation, target and enrollment operations retain their own authenticated semantic contracts. |
| `baha user` | Darstellung |  |  | Human directory and lifecycle commands for Core-managed people. |
| `baha user list` | Darstellung |  |  | Core-local human directory view; authentication state remains authoritative in the owned Keycloak realm. |
| `baha user offboard` | ausgeschlossen |  |  | Human-confirmed operator directory action disables the owned Keycloak user and revokes its workstation leaves; no typed MCP operation, so excluded from the machine tool boundary by design. |
| `baha workspace component` | presentation |  |  | Alias for the source-model component maintenance group; writes require workspace.init and reads require workspace.show. |
| `baha workspace component add` | presentation |  |  | Alias for component add; edits the source model through workspace.init. |
| `baha workspace component list` | presentation |  |  | Alias for component list; reads the source model through workspace.show. |
| `baha workspace component remove` | presentation |  |  | Alias for component remove; edits the source model through workspace.init. |
| `baha workspace path` | presentation |  |  | Alias for setting or reading a local checkout path; writes require workspace.map and never create or delete files. |
| `baha workspace paths` | alias |  |  | Canonical command: baha workspace path; shares its coverage classification. |
| `baha workspace source` | presentation |  |  | Alias for the source-model maintenance group; writes require workspace.init. |
| `baha workspace source add` | presentation |  |  | Alias for source add; edits the source model through workspace.init. |
| `baha workspace source remove` | presentation |  |  | Alias for source remove; detaches model references without deleting local files. |
| `baha workstation` | Darstellung |  |  | Human inventory and lifecycle commands for Core-managed workstation certificates. |
| `baha workstation list` | Darstellung |  |  | Core-local inventory of workstation certificate ownership, expiry and revocation state. |
| `baha workstation new` | ausgeschlossen |  |  | Human invitation workflow for a new device; private one-use invitation transfer is an operator action and has no machine equivalent. |
| `baha workstation revoke` | ausgeschlossen |  |  | Human-confirmed device revocation; no typed MCP operation, so excluded from the machine tool boundary by design. |
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
| `baha target policy` | Darstellung |  |  | Command group; invoke its supported subcommands. |
| `baha target policy set` | Darstellung |  |  | Geschuetzte lokale Target-Policy-Aenderung mit CAS; fuer Remote-Targets wird keine lokale Policy-Aenderung anstelle der Core-Operator-Autoritaet zugelassen. |
| `baha target policy show` | semantisch | CLI -o json / typed MCP result | baseharbor.target |  |
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
| `baha workspace` | Darstellung |  |  | Canonical interactive presentation of the same workspace.init/map operations. `baha workspace source` und `baha workspace component` sind die Kanalnamen für die Gruppenpflege von Quellen und Komponenten. |
| `baha workspace init` | semantisch | CLI -o json / typed MCP result | baseharbor.workspace.init |  |
| `baha workspace map` | semantisch | CLI -o json / typed MCP result | baseharbor.workspace.map |  |
| `baha workspace resolve` | semantisch | CLI -o json / typed MCP result | baseharbor.workspace.resolve |  |
| `baha workspace show` | semantisch | CLI -o json / typed MCP result | baseharbor.workspace.show |  |
| `baha workspace status` | semantisch | CLI -o json / typed MCP result | baseharbor.workspace.status |  |
| `baha workspace update` | semantisch | CLI -o json / typed MCP result | baseharbor.workspace.update |  |
