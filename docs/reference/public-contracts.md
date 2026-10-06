# Public contract register

All entries are pre-freeze versioned drafts. Core maintainers own the canonical
artifacts and coordinate consumer changes; Runtime/Capability/Delivery/Target
Access implementations own their conformance. See the
[compatibility policy](https://github.com/mcpdev80/baseharbor/blob/HEAD/COMPATIBILITY.md)
for additive/breaking/result/deprecation rules and
[ADR 0018](../decisions/0018-public-contract-namespace-and-compatibility.md)
for namespace governance.

| Surface | Canonical artifact | Version / evolution boundary |
| --- | --- | --- |
| Application manifest / portable intent | [Manifest](manifest.md), [Application contract](../spec/application-contract-v1.md), `internal/application/manifest.go`, `internal/application/contract.go` | Manifest/Application v1; requirements remain portable |
| Service / capability intent | `contracts/service/v1/*.schema.json`, `spec/capabilities/*/v1.md` | Service/capability v1; product-neutral intent |
| Connection bindings | `contracts/binding/v1/service-binding.schema.json`, `spec/bindings/service-binding-1.1.md` | Binding 1.1; standards-aligned connection output |
| Secure bindings / credential references | `spec/bindings/secure/v1.md`, [Credential access](../spec/credential-access-v1.md) | v1; secret references, no ordinary secret payloads |
| Capability Provider protocol | `spec/provider/v1/provider.proto`, [Provider contract](../spec/provider-contract-v1.md) | Provider v1; distinct from product version |
| Provider distribution / descriptor | `contracts/provider/v1/provider-descriptor.schema.json` | Descriptor v1; immutable artifact/trust references |
| Runtime Provider | `spec/runtime-provider/v1/runtime_provider.proto`, [Runtime contract](../spec/runtime-provider-contract-v1.md) | Runtime v1; realization separate from intent |
| Delivery Provider | [Delivery contract](../spec/delivery-provider-contract-v1.md) | Delivery v1; not runtime authority |
| Workload Source Adapter | `internal/repositoryinspect/workload_source.go`, [Application contract](../spec/application-contract-v1.md) | Normalized source model; source is not runtime |
| Development Integration / StackProfile | `contracts/development/v1/*.schema.json`, [Development extension](../spec/development-extension-v1.md) | Development v1; ADR 0018 profile URI |
| Extension descriptor / artifact trust | `contracts/extension/v1/*.schema.json`, [Artifact trust](../spec/extension-artifact-trust-v1.md) | Extension/trust v1; separate signed artifact acceptance |
| Organization configuration | `internal/orgconfig`, [Organization configuration](../explanation/organization-configuration.md) | Organization v1; [resolution/policy](../spec/organization-resolution-v1.md) separate from distribution |
| Machine operations / JSON / MCP | `internal/machine/contract.go`, [Machine interface](../spec/machine-interface-v1.md) | Machine v1; shared semantic operations and typed failures |
| HTTP projection | `internal/machinehttp`, [Machine HTTP](../spec/machine-http-v1.md) | `/api/v1/machine`; same actor/policy/safety |
| Executions / events | `internal/machine/execution.go` | Execution/Event v1; explicit actor, state, sequence |
| Streams / terminal | `internal/machine/stream.go`, `internal/machine/terminal.go`, `contracts/machine/v1/terminal-*.schema.json`, [Machine HTTP](../spec/machine-http-v1.md) | Stream v1; implementation capabilities must be proven |
| Runtime Explorer | `internal/runtimeexplorer`, [Explorer](../spec/runtime-explorer-v1.md) | Explorer v1; ownership-safe inventory/operations |
| Runtime log normalization | [Log source contract](../architecture/runtime-log-source-contract.md) | Existing runtime-neutral log semantics; no new logs model |
| Target Access | `internal/targetaccess`, [Target Access](../spec/target-access-v1.md) | Access v1; descriptor is not authenticated live negotiation |
| Management surfaces / identity | [Management access](../spec/management-access-v1.md), [Application identity](../spec/application-runtime-identity.md) | v1; context/trust boundaries retained |
| Runtime Resource API | `spec/runtime-api/v1/openapi.yaml` | Runtime API v1; standard OpenAPI authority |
| Reconciliation / availability / consumption | [Reconciliation](../spec/reconciliation-v1.md), [Availability](../spec/availability-v1.md), [Consumption](../spec/application-consumption-v1.md) | v1; one Core lifecycle |
| Audit / evidence | `internal/evidence`, [Audit/Evidence](../spec/audit-evidence-v1.md) | v1; retained exact-input proof required |
| Persisted state / recovery | `internal/runtime/config.go`, `internal/orgconfig/store.go`, [Backup/restore](backup-and-restore.md) | Artifact-specific version/ownership; no pre-freeze migration promise |

Paths outside `docs/` are repository-relative artifacts. The embedded
`contracts.SchemaRegistry` is the executable schema inventory and resolves
every packaged schema without network access. Human references do not replace
schemas, Go wire types, OpenAPI or Protobuf definitions.

Coverage pending for v0.4.23: complete configuration consumers, interactive
terminal runtime/browser qualification and real remote/Console integration are tracked in #609,
#806, #807 and #808. Do not treat an entry in this register as live support.
