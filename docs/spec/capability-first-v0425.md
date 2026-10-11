# Capability-first application contracts (v0.4.25)

Binding scope: [#872](https://github.com/mcpdev80/baseharbor/issues/872), corrected
2026-10-11. This supersedes the fixed SQL/Secrets/Identity bundle, authored IDs
and deferred removal described in earlier planning and qualification records.
Existing security, ownership, HA and recovery guarantees remain requirements.

## Authoring and system-owned identity

```yaml
version: 1
app:
  name: myapp
  environment: dev
```

Services are optional and never implicit. SQL is `services.sql: true`, beneath
`services`; rich service requirements and named instances use the existing grammar.
No `provider:` section, alternate shorthand parser or additional required file.

Developers NEVER set application IDs. `ParseYAML` rejects every `app.id`, including
empty values, before mutation. Remove that field and explicitly initialize the
application. There is no ID question or flag and no legacy authored-ID conversion.
`ParseCanonicalYAML` uses the same parser with internal-record permission; only
existing system-owned Store/backup adapters may use it. `YAML` serializes internal
records, while `AuthoredYAML` omits the ID from developer outputs. Internal IDs and
machine results remain valid and must never be regenerated during reading.

| Operation | Identity and mutation contract |
| --- | --- |
| Parse / inspect / plan | No allocation or persistence; resolve existing authoritative claims only |
| Explicit initialization / registration | Generate once if truly new, persist in existing canonical Store |
| Repeat / restart / edit / another Target | Reuse the same system-owned identity |
| Conflicting or missing registered identity | Fail before mutation; recover authoritative state, never allocate replacement |
| Concurrent initialization | Reuse Store.Create winner or report incomplete state for safe retry |
| Developer `app.id` | Actionable rejection, preserving authored file and existing user data |

`ResolveIdentity` is pure. `Store.InitializeIdentity` uses the existing canonical
`.baseharbor/apps/<name>/baseharbor.yaml` owner. Deployment records project this
identity. Adapters correlate repository/source ownership, not names across unrelated
repositories. Read-only adapters never call Create, InitializeIdentity or Sync.
No parallel identity registry, automatic migration or deletion of existing data.

## Provider resolution and footprint

Application contract expresses need. Existing Target/deployment bindings express
provider and placement. `ProviderPreference` optionally selects `InstanceID` or
`ProviderKind` (both must agree when provided); empty means Automatic. Negotiation
uses actual registered capability surfaces, without a hard-coded provider list.
Existing compatible bindings remain stable; explicit disagreement is a typed
conflict. Different Targets can realize the same portable requirement differently.

`ResolveFootprint` reads one Target's existing `capability.Registry` v2 and
provider facts from existing plans/observers. Facts carry actual dependency
instance IDs: null means unknown, [] means verified none. No discovery writes,
new registry, scheduler or second resolver. Cross-Target/foreign resources,
cycles, missing dependencies and incompatible or ambiguous choices fail closed.
Automatic without an available declaration is unresolved and cannot authorize
provisioning. A verified selected provider is structurally complete even while
its binding awaits the existing apply transaction.

Runtime permission/consumption adapters supply verified `DependencyRoots` from
existing plans to the same resolver. Nil roots with those intents mean incomplete;
[] proves no extra provider realization, not absence of security requirements.
Each ConsumerIntent supplies its own roots; one consumer cannot borrow another
consumer's verification. Unknown roots never authorize a Core-free mutation.

No capability implies the old Core package. SQL, cache, Secrets, Identity and
HTTPS select only the realization and its genuine declared dependencies. Identity
may need SQL. HTTPS does not automatically need Identity or OpenBao. Independently
enabled management/Console services contribute their actual security requirements
as separate consumers; TLS/OIDC/mTLS/authorization/HA/recovery are never weakened.
The legacy `core` output classifies an actual declaration, never requests a bundle.

The shared read-only result is `baseharbor.footprint/v1`:

| Field | Meaning |
| --- | --- |
| requirements | Requested capability, existing binding or requested/unbound, selected instance/state |
| existing / additional | Existing/shared/stopped resources versus declared newly needed resources |
| dependencies | Selected dependency IDs; null is unverifiable, [] is verified none |
| unused | Informational IDs outside this request and without bindings; never removal authority |
| scope / ownership / shared / consumers | Existing ownership and binding count; no invented app counts |
| memory_bytes / containers | measured/estimated/unknown; values require nonnegative quantity and source |
| core / complete | Actual declared requirement and structural certainty; incomplete forbids unsafe mutation |

Unknown measurements may coexist with complete structural resolution. Estimates
come from provider evidence, never guessed RAM/container promises or totals.

## Target consumer union and add/remove/re-add

`ResolveProviderPlan(snapshot, consumers, inventoryComplete)` delegates every
active consumer to `ResolveFootprint` and unions its full dependency closure.
`ConsumerIntent` is an ephemeral projection of an existing app or independent
management owner: Key, Active, Manifest, Preferences, DependencyRoots. Inactive entries retain
identity and release bindings. The caller proves complete Target inventory from
authoritative existing owners. Unaccounted bindings or unverifiable facts make
the plan incomplete, blocking all stop/remove decisions.

`baseharbor.provider-plan/v1` contains per-consumer footprints, required provider
IDs, released bindings and ownership/evidence-backed changes:

| Action | Meaning |
| --- | --- |
| add | Materialize selected newly required realization |
| reuse | Keep realization required by an app, management consumer or dependency |
| restart | Reactivate an owned stopped required runtime |
| stop | Stop owned running runtime after the last consumer releases it |
| remove-runtime | Remove unneeded owned stopped runtime, preserving persistent data |
| preserve | External/BYO, foreign/unverifiable or incomplete inventory; never implicit deletion |

All changes carry `PreserveData=true`. These are plans, not permissions to invoke
runtime mutation directly. Session 2 consumes them through existing
plan -> preflight -> confirm -> apply -> verify. Preserve each data/recovery item
with an individual reason; deletion requires explicit destructive consent and
ownership proof. Shared-still-used and external/foreign resources are never deleted.
Repeat apply, interrupted retry and disable/re-enable converge idempotently.
Capability removal belongs to v0.4.25, not a later reclamation project.

CLI/human, JSON and MCP consume the same domain results and existing machine
error/exit/authorization mappings. ResolutionError remains typed. Fixtures:
`internal/application/testdata/v0425-resolution.json`; union invariants:
`internal/application/provider_union_test.go`. The full provider/capability wizard
is v0.4.26; #777 profile/host contracts below remain specification only.

## #777 portable connection-profile transport (contract only)

This is an import/export transport envelope for later #777 implementation,
**not** a replacement for existing `development.StackProfile` (development/v1),
Target configuration, access storage or provider registry. No implementation,
file creation or onboarding UI is delivered by this specification.

| Envelope field | Contract |
| --- | --- |
| version | Exact `baseharbor.connection-profile/v1`; reject unsupported versions |
| name | Human alias; never an identity or authorization claim |
| core.installation_id / core.url | Existing canonical Core UUID and HTTPS management endpoint; verify server identity |
| trust.ca_pem / trust.sha256 | One public PEM CA certificate and its DER SHA-256 fingerprint; verify agreement and require an independently authorized trust decision |
| targets | List of portable descriptors: name, tenant_id, runtime_provider, scope; retain existing Target semantics |
| access | OIDC issuer, client_id and scopes only; no imported authorization claims |
| expires_at | Optional transport expiry; reject expired enrollment material |
| extensions | Namespaced future optional fields; no change to existing required semantics |

An example transport describes a Core, trust and selected Targets; it does not
contain application intent, provider credentials or a new authoritative state.
Targets reference a Core where applicable; profiles are not Targets and do not
merge dev/prod installations. Core-less local operation needs no imported profile.

Exclude passwords, private keys, refresh/access tokens, AppRole secrets, recovery
material, local sockets/engine contexts, checkout paths and local state roots.
Authentication/enrollment artifacts use existing protected access owners after
explicit login/enrollment; importing descriptors cannot grant a role, claim a
foreign deployment, activate arbitrary endpoints or start resources. Imported
public trust alone cannot self-authorize a server. Verify Core UUID, HTTPS,
issuer, tenant and selected Target before writing existing config/access owners.

Import is parse -> validate -> resolve references/conflicts -> show diff ->
explicit authorization -> existing owner transaction -> verification. Same
identity plus same content is idempotent; same alias with another identity is a
conflict. Partial writes must be rolled back or exposed as incomplete with a
recovery action, never reported as successful. Export is read-only and preserves
portable identity without secret/local fields. Host-local values are resolved
after import through existing local config. Implementations arrive in later
releases and require schema/transaction conformance against these semantics.

## Host Platform boundary (contract only)

The semantic boundary `baseharbor.host-platform/v1` is adapter capability
negotiation, not a new runtime, registry, scheduler or multi-OS implementation.

| Responsibility | Boundary |
| --- | --- |
| Platform/feature detection | Host facts and supported/unsupported/unverifiable outcome with evidence |
| Engine selection | Actual runtime endpoint, context and rootless/rootful mode; no silent fallback to privileged engine |
| Local paths / source access | Host-local mapping stays outside portable manifest/profile; deterministic failure for unavailable source |
| Credential / trust access | Existing protected owner and explicit trust changes; no automatic security downgrade |
| Browser / PTY / shell | Host operation support and typed unavailable result; preserve non-TTY/EOF/cancel contracts |
| Runtime execution | Existing RuntimeProvider owns workload lifecycle and provider verification |
| Enrollment / remote authority | Existing Core/Node authority and mTLS; local host adapter cannot mint remote authority |

Linux, WSL2 and Darwin implementations are later releases. Adapters report real
support instead of pretending an unsupported host operation succeeded. Host
paths, sockets and platform defaults must not change ApplicationID, capability
intent, Target ownership, footprint evidence or machine error semantics.
No host probing with persistent effects from inspection/footprint.


Normative offline schemas: `contracts/footprint/v1/footprint.schema.json` and `contracts/connection-profile/v1/profile.schema.json`. Profile shape validation never proves valid CA material, fingerprint agreement, expiry, tenant uniqueness or authorization; later importers must enforce these semantic checks and allow only reviewed non-secret extensions.
