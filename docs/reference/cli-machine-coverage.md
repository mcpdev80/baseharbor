# CLI / machine coverage

Generated from the actual command tree and typed operation registry. Every command is semantic, an alias, presentation, or an explicitly justified host/transport exclusion. Optional and compound modes are documented in each row. Unknown product commands fail the inventory test.

Inspect current data with `baha agent describe -o json` (`cli_coverage`). Update this reference after a code change with `BASEHARBOR_UPDATE_MACHINE_DOCS=1 go test ./cmd/baha -run TestMachineDocumentationTracksRegistryAndCommandTree`. Ordinary test runs reject drift.

| Command | Classification | CLI JSON / structured result | Operation / tool | Reason |
| --- | --- | --- | --- | --- |
| `baha` | presentation |  |  | Command group; invoke its supported subcommands. |
| `baha agent` | presentation |  |  | Command group; invoke its supported subcommands. |
| `baha agent describe` | presentation |  |  | Machine registry/discovery itself, rather than a product mutation. |
| `baha app` | presentation |  |  | Command group; invoke its supported subcommands. |
| `baha app apply` | semantic | CLI -o json / typed MCP result | baseharbor.apply |  |
| `baha app backup` | semantic | CLI -o json / typed MCP result | baseharbor.backup |  |
| `baha app create` | semantic | CLI -o json / typed MCP result | baseharbor.app.create |  |
| `baha app creds` | semantic | CLI -o json / typed MCP result | baseharbor.app.connection | MCP exposes connection metadata without passwords or credential-bearing URIs; CLI credential reveal is operator-local and excluded from MCP. |
| `baha app destroy` | semantic | CLI -o json / typed MCP result | baseharbor.destroy |  |
| `baha app doctor` | semantic | CLI -o json / typed MCP result | baseharbor.doctor |  |
| `baha app down` | semantic | CLI -o json / typed MCP result | baseharbor.app.stop |  |
| `baha app env` | semantic | CLI --format json (masked) / typed MCP result | baseharbor.app.environment | MCP exposes masked values only. --reveal and loading the protected local dotenv file are execution-host credential access and are explicitly excluded modes. |
| `baha app evidence` | semantic | CLI -o json / typed MCP result | baseharbor.evidence |  |
| `baha app exec` | excluded |  |  | Arbitrary process execution is intentionally excluded: #787 forbids generic exec passthrough; use typed lifecycle and observation tools. |
| `baha app init` | semantic | CLI -o json / typed MCP result | baseharbor.app.adopt | New repository intent uses app.adopt; an existing repository uses app.configure. Interactive suggestions are presentation only. |
| `baha app inspect` | semantic | CLI -o json / typed MCP result | baseharbor.inspect |  |
| `baha app list` | semantic | CLI -o json / typed MCP result | baseharbor.app.list |  |
| `baha app logs` | excluded |  |  | Raw/follow runtime streams can include application secrets and require a streaming transport; status, doctor and evidence provide bounded secret-safe observation. |
| `baha app new` | semantic | CLI -o json / typed MCP result | baseharbor.app.new |  |
| `baha app plan` | semantic | CLI -o json / typed MCP result | baseharbor.plan |  |
| `baha app preflight` | semantic | CLI -o json / typed MCP result | baseharbor.app.preflight |  |
| `baha app psql` | excluded |  |  | Interactive database terminal and raw SQL passthrough are excluded; app.connection supplies secret-safe connection metadata for an operator-owned client. |
| `baha app redis` | excluded |  |  | Interactive database terminal and arbitrary command passthrough are excluded; app.connection supplies secret-safe connection metadata for an operator-owned client. |
| `baha app restore` | semantic | CLI -o json / typed MCP result | baseharbor.restore |  |
| `baha app runtime-identity` | presentation |  |  | Command group; invoke its supported subcommands. |
| `baha app runtime-identity revoke` | semantic | CLI -o json / typed MCP result | baseharbor.runtime-identity.revoke |  |
| `baha app runtime-identity rotate` | semantic | CLI -o json / typed MCP result | baseharbor.runtime-identity.rotate |  |
| `baha app secret` | presentation |  |  | Command group; invoke its supported subcommands. |
| `baha app secret delete` | semantic | CLI -o json / typed MCP result | baseharbor.secret.delete |  |
| `baha app secret list` | semantic | CLI -o json / typed MCP result | baseharbor.secret.list |  |
| `baha app secret set` | semantic | CLI -o json / typed MCP result | baseharbor.secret.set |  |
| `baha app secret tls-set` | semantic | CLI -o json / typed MCP result | baseharbor.secret.tls-set |  |
| `baha app shell` | excluded |  |  | Interactive container terminal is host/TTY dependent and would grant arbitrary exec; use typed lifecycle and app.environment instead. |
| `baha app show` | semantic | CLI -o json / typed MCP result | baseharbor.app.show |  |
| `baha app status` | semantic | CLI -o json / typed MCP result | baseharbor.status |  |
| `baha app tls` | presentation |  |  | Command group; invoke its supported subcommands. |
| `baha app tls update` | semantic | CLI -o json / typed MCP result | baseharbor.tls.update |  |
| `baha app up` | semantic | CLI -o json / typed MCP result | baseharbor.apply |  |
| `baha app update` | semantic | CLI -o json / typed MCP result | baseharbor.update |  |
| `baha app valkey` | alias |  |  | Canonical command: baha app redis; shares its coverage classification. |
| `baha app workspace` | presentation |  |  | Interactive presentation combining workspace.init and workspace.map; clients invoke those typed operations explicitly. |
| `baha app workspace init` | semantic | CLI -o json / typed MCP result | baseharbor.workspace.init |  |
| `baha app workspace map` | semantic | CLI -o json / typed MCP result | baseharbor.workspace.map |  |
| `baha app workspace resolve` | semantic | CLI -o json / typed MCP result | baseharbor.workspace.resolve |  |
| `baha app workspace show` | semantic | CLI -o json / typed MCP result | baseharbor.workspace.show |  |
| `baha app workspace status` | semantic | CLI -o json / typed MCP result | baseharbor.workspace.status |  |
| `baha app workspace update` | semantic | CLI -o json / typed MCP result | baseharbor.workspace.update |  |
| `baha backup` | semantic | CLI -o json / typed MCP result | baseharbor.backup |  |
| `baha completion` | presentation |  |  | Shell completion emits client-local shell text; semantic discovery is agent describe. |
| `baha config` | presentation |  |  | Command group; invoke its supported subcommands. |
| `baha config organization` | presentation |  |  | Command group; invoke its supported subcommands. |
| `baha config organization check` | semantic | CLI -o json / typed MCP result | baseharbor.organization.check |  |
| `baha config organization set` | semantic | CLI -o json / typed MCP result | baseharbor.organization.set |  |
| `baha config organization show` | semantic | CLI -o json / typed MCP result | baseharbor.organization.inspect |  |
| `baha config organization update` | semantic | CLI -o json / typed MCP result | baseharbor.organization.update |  |
| `baha config prompt` | presentation |  |  | Client-local shell prompt display preferences; explicit target arguments and target inspection provide semantic context. |
| `baha connect` | semantic | CLI -o json / typed MCP result | baseharbor.connectivity.connect |  |
| `baha connections` | semantic | CLI -o json / typed MCP result | baseharbor.connectivity.list |  |
| `baha destroy` | semantic | CLI -o json / typed MCP result | baseharbor.control-plane.destroy | Control-plane mode maps here; repository application mode uses destroy. --all maps to installation.destroy after authorization of every discovered deployment. All destruction retains ownership checks and explicit approval. |
| `baha dev` | presentation |  |  | Command group; invoke its supported subcommands. |
| `baha dev credentials` | semantic | CLI -o json / typed MCP result | baseharbor.dev.credentials | Ensure/reset/configuration use the shared authority. MCP returns username and protected file reference; CLI password display is an excluded operator-local mode. |
| `baha dev domain` | semantic | CLI -o json / typed MCP result | baseharbor.dev.domain |  |
| `baha disconnect` | semantic | CLI -o json / typed MCP result | baseharbor.connectivity.disconnect |  |
| `baha doctor` | semantic | CLI -o json / typed MCP result | baseharbor.control-plane.doctor | Control-plane mode maps here; --fix uses control-plane.repair and rechecks readiness. Repository mode uses doctor or repair. |
| `baha down` | semantic | CLI -o json / typed MCP result | baseharbor.control-plane.stop | Control-plane mode maps here; repository application mode uses app.stop. |
| `baha init` | semantic | CLI -o json / typed MCP result | baseharbor.app.adopt |  |
| `baha inspect` | semantic | CLI -o json / typed MCP result | baseharbor.inspect |  |
| `baha list` | semantic | CLI -o json / typed MCP result | baseharbor.app.list |  |
| `baha login` | excluded |  |  | Authorization Code/PKCE browser handoff establishes the operator's local protected session before semantic requests; operator.identity verifies that boundary without returning tokens. |
| `baha logout` | excluded |  |  | Removes a client-local protected login session; performed by the authentication client, outside server product actions. operator.identity reports the resulting boundary. |
| `baha mcp` | presentation |  |  | Command group; invoke its supported subcommands. |
| `baha mcp serve` | presentation |  |  | Starts the local MCP transport; clients launch it before discovery. |
| `baha new` | presentation |  |  | Human creation chooser delegating to existing app.new, stack.create, target.create, provider.init and workspace operations. |
| `baha open` | presentation |  |  | Host browser presentation over validated application/TLS state; typed clients inspect app status and TLS instead. |
| `baha openbao` | presentation |  |  | Command group; invoke its supported subcommands. |
| `baha openbao bootstrap` | semantic | CLI -o json / typed MCP result | baseharbor.openbao.bootstrap |  |
| `baha openbao rotate` | semantic | CLI -o json / typed MCP result | baseharbor.openbao.rotate |  |
| `baha openbao status` | semantic | CLI -o json / typed MCP result | baseharbor.openbao.status |  |
| `baha openbao unseal` | semantic | CLI -o json / typed MCP result | baseharbor.openbao.unseal |  |
| `baha plan` | semantic | CLI -o json / typed MCP result | baseharbor.plan |  |
| `baha policy` | presentation |  |  | Command group; invoke its supported subcommands. |
| `baha policy check` | semantic | CLI -o json / typed MCP result | baseharbor.policy.check |  |
| `baha policy explain` | semantic | CLI -o json / typed MCP result | baseharbor.policy.explain |  |
| `baha provider` | presentation |  |  | Command group; invoke its supported subcommands. |
| `baha provider add` | semantic | CLI -o json / typed MCP result | baseharbor.provider.add |  |
| `baha provider init` | semantic | CLI -o json / typed MCP result | baseharbor.provider.init |  |
| `baha provider inspect` | semantic | CLI -o json / typed MCP result | baseharbor.provider.inspect |  |
| `baha provider list` | semantic | CLI -o json / typed MCP result | baseharbor.provider.list |  |
| `baha provider remove` | semantic | CLI -o json / typed MCP result | baseharbor.provider.remove |  |
| `baha provider test` | semantic | CLI -o json / typed MCP result | baseharbor.provider.test |  |
| `baha provider verify` | semantic | CLI -o json / typed MCP result | baseharbor.provider.verify |  |
| `baha restore` | semantic | CLI -o json / typed MCP result | baseharbor.restore |  |
| `baha serve` | excluded |  |  | Starts a long-lived TLS API/broker transport using host-held credentials; transport supervision belongs to the host, typed lifecycle tools operate managed resources. |
| `baha shell-init` | presentation |  |  | Shell-local helpers; explicit Target selection remains product functionality. |
| `baha stack` | presentation |  |  | Command group; invoke its supported subcommands. |
| `baha stack create` | semantic | CLI -o json / typed MCP result | baseharbor.stack.create |  |
| `baha stack list` | semantic | CLI -o json / typed MCP result | baseharbor.stack.list |  |
| `baha stack show` | semantic | CLI -o json / typed MCP result | baseharbor.stack.show |  |
| `baha status` | semantic | CLI -o json / typed MCP result | baseharbor.control-plane.status | Control-plane mode maps here; repository application mode uses status. |
| `baha target` | semantic | CLI -o json / typed MCP result | baseharbor.target |  |
| `baha target activate` | presentation |  |  | Emits shell-local selection only; machine clients supply the explicit target argument on each semantic operation. |
| `baha target create` | semantic | CLI -o json / typed MCP result | baseharbor.target.create |  |
| `baha target deactivate` | presentation |  |  | Clears shell-local selection only; machine clients omit an explicit target to resolve configured defaults. |
| `baha target delete` | semantic | CLI -o json / typed MCP result | baseharbor.target.delete |  |
| `baha target list` | semantic | CLI -o json / typed MCP result | baseharbor.target.list |  |
| `baha target show` | semantic | CLI -o json / typed MCP result | baseharbor.target |  |
| `baha trust` | presentation |  |  | Command group; invoke its supported subcommands. |
| `baha trust export` | semantic | CLI --json (--output is CA destination) / typed MCP result | baseharbor.trust.export |  |
| `baha trust install` | semantic | CLI -o json / typed MCP result | baseharbor.trust.install |  |
| `baha trust status` | semantic | CLI -o json / typed MCP result | baseharbor.trust.status |  |
| `baha tui` | presentation |  |  | Human interactive presentation; its product actions require individual semantic coverage. |
| `baha up` | semantic | CLI -o json / typed MCP result | baseharbor.control-plane.up | Control-plane-only mode maps here. Repository application mode additionally uses app.configure and apply; host trust installation uses trust.install with approval. |
| `baha update` | semantic | CLI -o json / typed MCP result | baseharbor.release.check | --check maps to release.check. Installing replaces the execution-host binary and is explicitly excluded from MCP; use the host operator CLI with --yes. |
| `baha version` | presentation |  |  | Executable build identity is provided by agent describe and MCP initialization. |
| `baha whoami` | semantic | CLI -o json / typed MCP result | baseharbor.operator.identity |  |
