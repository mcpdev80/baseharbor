# CLI / machine coverage

Generated from the actual command tree and typed operation registry. `gap` explicitly means pending #787 implementation, not an approved exclusion. A semantic mapping describes the canonical operation; interactive/optional modes require separate review before release acceptance. `baha up`, top-level status/doctor and control-plane operations retain explicit gaps because their repository and host modes are broader than one application tool.

Inspect current data with `baha agent describe -o json` (`cli_coverage`). Update this reference after a code change with `BASEHARBOR_UPDATE_MACHINE_DOCS=1 go test ./cmd/baha -run TestMachineDocumentationTracksRegistryAndCommandTree`. Ordinary test runs reject drift.

| Command | Classification | Operation / tool | Reason |
| --- | --- | --- | --- |
| `baha` | presentation |  | Command group; invoke its supported subcommands. |
| `baha agent` | presentation |  | Command group; invoke its supported subcommands. |
| `baha agent describe` | presentation |  | Machine registry/discovery itself, rather than a product mutation. |
| `baha app` | presentation |  | Command group; invoke its supported subcommands. |
| `baha app apply` | semantic | baseharbor.apply |  |
| `baha app backup` | semantic | baseharbor.backup |  |
| `baha app create` | gap |  | Supported product operation pending semantic machine coverage in #787. |
| `baha app creds` | gap |  | Supported product operation pending semantic machine coverage in #787. |
| `baha app destroy` | semantic | baseharbor.destroy |  |
| `baha app doctor` | semantic | baseharbor.doctor |  |
| `baha app down` | gap |  | Supported product operation pending semantic machine coverage in #787. |
| `baha app env` | gap |  | Supported product operation pending semantic machine coverage in #787. |
| `baha app evidence` | semantic | baseharbor.evidence |  |
| `baha app exec` | gap |  | Supported product operation pending semantic machine coverage in #787. |
| `baha app init` | gap |  | Supported product operation pending semantic machine coverage in #787. |
| `baha app inspect` | semantic | baseharbor.inspect |  |
| `baha app list` | semantic | baseharbor.app.list |  |
| `baha app logs` | gap |  | Supported product operation pending semantic machine coverage in #787. |
| `baha app new` | semantic | baseharbor.app.new |  |
| `baha app plan` | semantic | baseharbor.plan |  |
| `baha app preflight` | gap |  | Supported product operation pending semantic machine coverage in #787. |
| `baha app psql` | gap |  | Supported product operation pending semantic machine coverage in #787. |
| `baha app redis` | gap |  | Supported product operation pending semantic machine coverage in #787. |
| `baha app restore` | semantic | baseharbor.restore |  |
| `baha app runtime-identity` | presentation |  | Command group; invoke its supported subcommands. |
| `baha app runtime-identity revoke` | gap |  | Supported product operation pending semantic machine coverage in #787. |
| `baha app runtime-identity rotate` | gap |  | Supported product operation pending semantic machine coverage in #787. |
| `baha app secret` | presentation |  | Command group; invoke its supported subcommands. |
| `baha app secret delete` | gap |  | Supported product operation pending semantic machine coverage in #787. |
| `baha app secret list` | gap |  | Supported product operation pending semantic machine coverage in #787. |
| `baha app secret set` | gap |  | Supported product operation pending semantic machine coverage in #787. |
| `baha app secret tls-set` | gap |  | Supported product operation pending semantic machine coverage in #787. |
| `baha app shell` | gap |  | Supported product operation pending semantic machine coverage in #787. |
| `baha app show` | gap |  | Supported product operation pending semantic machine coverage in #787. |
| `baha app status` | semantic | baseharbor.status |  |
| `baha app tls` | presentation |  | Command group; invoke its supported subcommands. |
| `baha app tls update` | gap |  | Supported product operation pending semantic machine coverage in #787. |
| `baha app up` | semantic | baseharbor.apply |  |
| `baha app update` | semantic | baseharbor.update |  |
| `baha app valkey` | alias |  | Canonical command: baha app redis; shares its coverage classification. |
| `baha app workspace` | gap |  | Supported product operation pending semantic machine coverage in #787. |
| `baha app workspace init` | semantic | baseharbor.workspace.init |  |
| `baha app workspace map` | semantic | baseharbor.workspace.map |  |
| `baha app workspace resolve` | semantic | baseharbor.workspace.resolve |  |
| `baha app workspace show` | gap |  | Supported product operation pending semantic machine coverage in #787. |
| `baha app workspace status` | semantic | baseharbor.workspace.status |  |
| `baha app workspace update` | semantic | baseharbor.workspace.update |  |
| `baha completion` | presentation |  | Shell completion emits client-local shell text; semantic discovery is agent describe. |
| `baha config` | presentation |  | Command group; invoke its supported subcommands. |
| `baha config organization` | presentation |  | Command group; invoke its supported subcommands. |
| `baha config organization check` | semantic | baseharbor.organization.check |  |
| `baha config organization set` | semantic | baseharbor.organization.set |  |
| `baha config organization show` | semantic | baseharbor.organization.inspect |  |
| `baha config organization update` | semantic | baseharbor.organization.update |  |
| `baha config prompt` | gap |  | Supported product operation pending semantic machine coverage in #787. |
| `baha connect` | gap |  | Supported product operation pending semantic machine coverage in #787. |
| `baha connections` | gap |  | Supported product operation pending semantic machine coverage in #787. |
| `baha destroy` | gap |  | Supported product operation pending semantic machine coverage in #787. |
| `baha dev` | presentation |  | Command group; invoke its supported subcommands. |
| `baha dev credentials` | gap |  | Supported product operation pending semantic machine coverage in #787. |
| `baha dev domain` | gap |  | Supported product operation pending semantic machine coverage in #787. |
| `baha disconnect` | gap |  | Supported product operation pending semantic machine coverage in #787. |
| `baha doctor` | gap |  | Supported product operation pending semantic machine coverage in #787. |
| `baha down` | gap |  | Supported product operation pending semantic machine coverage in #787. |
| `baha init` | presentation |  | Explains initialization paths; app init and target create remain explicit product coverage gaps. |
| `baha login` | gap |  | Supported product operation pending semantic machine coverage in #787. |
| `baha logout` | gap |  | Supported product operation pending semantic machine coverage in #787. |
| `baha mcp` | presentation |  | Command group; invoke its supported subcommands. |
| `baha mcp serve` | presentation |  | Starts the local MCP transport; clients launch it before discovery. |
| `baha openbao` | presentation |  | Command group; invoke its supported subcommands. |
| `baha openbao bootstrap` | gap |  | Supported product operation pending semantic machine coverage in #787. |
| `baha openbao rotate` | gap |  | Supported product operation pending semantic machine coverage in #787. |
| `baha openbao status` | gap |  | Supported product operation pending semantic machine coverage in #787. |
| `baha openbao unseal` | gap |  | Supported product operation pending semantic machine coverage in #787. |
| `baha plan` | semantic | baseharbor.plan |  |
| `baha policy` | presentation |  | Command group; invoke its supported subcommands. |
| `baha policy check` | semantic | baseharbor.policy.check |  |
| `baha policy explain` | semantic | baseharbor.policy.explain |  |
| `baha provider` | presentation |  | Command group; invoke its supported subcommands. |
| `baha provider add` | semantic | baseharbor.provider.add |  |
| `baha provider init` | gap |  | Supported product operation pending semantic machine coverage in #787. |
| `baha provider inspect` | semantic | baseharbor.provider.inspect |  |
| `baha provider list` | semantic | baseharbor.provider.list |  |
| `baha provider remove` | semantic | baseharbor.provider.remove |  |
| `baha provider test` | gap |  | Supported product operation pending semantic machine coverage in #787. |
| `baha provider verify` | semantic | baseharbor.provider.verify |  |
| `baha serve` | gap |  | Supported product operation pending semantic machine coverage in #787. |
| `baha shell-init` | presentation |  | Shell-local helpers; explicit Target selection remains product functionality. |
| `baha stack` | presentation |  | Command group; invoke its supported subcommands. |
| `baha stack create` | gap |  | Supported product operation pending semantic machine coverage in #787. |
| `baha stack list` | gap |  | Supported product operation pending semantic machine coverage in #787. |
| `baha stack show` | gap |  | Supported product operation pending semantic machine coverage in #787. |
| `baha status` | gap |  | Supported product operation pending semantic machine coverage in #787. |
| `baha target` | semantic | baseharbor.target |  |
| `baha target activate` | gap |  | Supported product operation pending semantic machine coverage in #787. |
| `baha target create` | gap |  | Supported product operation pending semantic machine coverage in #787. |
| `baha target deactivate` | gap |  | Supported product operation pending semantic machine coverage in #787. |
| `baha target delete` | gap |  | Supported product operation pending semantic machine coverage in #787. |
| `baha target list` | semantic | baseharbor.target.list |  |
| `baha target show` | semantic | baseharbor.target |  |
| `baha trust` | presentation |  | Command group; invoke its supported subcommands. |
| `baha trust export` | gap |  | Supported product operation pending semantic machine coverage in #787. |
| `baha trust install` | gap |  | Supported product operation pending semantic machine coverage in #787. |
| `baha trust status` | gap |  | Supported product operation pending semantic machine coverage in #787. |
| `baha tui` | presentation |  | Human interactive presentation; its product actions require individual semantic coverage. |
| `baha up` | gap |  | Supported product operation pending semantic machine coverage in #787. |
| `baha update` | gap |  | Supported product operation pending semantic machine coverage in #787. |
| `baha version` | presentation |  | Executable build identity is provided by agent describe and MCP initialization. |
| `baha whoami` | gap |  | Supported product operation pending semantic machine coverage in #787. |
