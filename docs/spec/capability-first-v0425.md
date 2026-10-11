# Capability-first application contracts (v0.4.25)

Scope: [#872](https://github.com/mcpdev80/baseharbor/issues/872).
These contracts supersede the unconditional first-application Core bootstrap
rule for application deployment. They do not change the delivered management
Core requirements in #810. Corrections before v0.5 require explicit contract
review; later releases consume this semantic foundation.

## Authoring and identity

The minimum developer-authored manifest uses the existing parser and version:

```yaml
version: 1
app:
  name: myapp
  environment: dev
```

SQL remains `services.sql: true` (a YAML mapping beneath `services`). Rich
`services.sql.enabled: true`, named instances and other existing fields remain
valid. Services are optional. Top-level SQL and a second shorthand parser are
not supported. An empty document or standalone `{}` cannot supply the required
name/environment/version; detection must supply actual intent, never fabricate
an executable workload.

`New` retains explicit flags and generates an ID for newly created canonical
objects; it no longer adds SQL when all flags are false. `ParseYAML` permits an
omitted ID, never generates one, and validates authoring with `ValidateIntent`.
`Validate` retains executable-manifest prerequisites. Passing authoring
validation does not prove source availability, running containers or readiness.

| Operation | ID and mutation contract |
| --- | --- |
| Parse / inspect / plan | Resolve existing owner claims; fresh unresolved ID stays empty; no filesystem/runtime creation |
| Authorized init / registration | Detect supported workload, reconcile claims, allocate once only when truly new, persist through existing Store |
| Repeat / another Target | Reuse existing canonical ID; never mint because an engine is stopped or a Target differs |
| Existing authored UUID | Preserve it; disagreement with Store/deployment identity is an error |
| Previously registered missing ID | Restore/reconcile the owner; never silently allocate a replacement |
| Multiple UUIDs for one owner | Fail closed; explicit ownership reconciliation required |
| Rename with existing Store ownership | Require owner reconciliation; never create an unnoticed duplicate identity |

`ResolveIdentity` is pure and takes scoped claims from existing owners.
`Store.InitializeIdentity` uses the existing repository-local
`.baseharbor/apps/<name>/baseharbor.yaml` canonical Store copy when authored YAML
omits an ID. This copy is the sole durable identity owner for that input.
Authored UUIDs remain authoritative and must agree with that copy. There is no
new ID file/registry. The Target deployment Store is a projection and cannot
bootstrap portable identity because its root already depends on the UUID.

Adapters must correlate existing canonical repository/source ownership and
deployment records before initialization. App name alone across unrelated
repositories is insufficient evidence. Repository YAML is never rewritten
implicitly. Concurrent initialization converges on the existing Store.Create
winner or reports incomplete initialization for retry; it never replaces the
winner. Read-only adapters must not call Create, InitializeIdentity or Sync.

## Provider resolution and footprint

`PortableContractFromManifest` remains portable capability intent.
`ProviderPreference` contains an optional concrete instance ID in existing
deployment/operator binding state, outside developer YAML; empty means automatic.
A provider product, provider distribution ID and provider instance ID are
different identities. Selection negotiates the actual instance capability
surface using existing `capability.Registry` version 2.

`ResolveFootprint(manifest, snapshot, preferences)` consumes one selected Target's
registry and facts from existing plans/observers. It has no discovery, filesystem,
registry, lifecycle, enrollment or scheduling effects. Facts must identify that
Target and contain verified provider state, declared Core need, and actual
selected dependency instance IDs. Nil dependencies mean unknown; an empty list
means verified none. Unknown declarations must never be filled with guesses.
Cross-Target facts, cycles, missing dependencies and foreign resources fail closed.

| Case | Result / required behavior |
| --- | --- |
| No capability/runtime/consumption requirements | Core not required; no selected provider resources; existing unused resources retained |
| Automatic compatible instance | Select one compatible eligible instance; no question unless ambiguous |
| Existing binding | Reuse UUID/environment/resource binding; explicit disagreement fails |
| Explicit missing / incompatible | Typed missing / incompatible error before mutation |
| Multiple eligible instances | Typed ambiguous error; no arbitrary first choice |
| Foreign owner / Target | Typed foreign / foreign-target error; no adoption or mutation |
| Shared / reused | Include physical instance once, preserve all registered consumers |
| External / BYO | External ownership, no owned provisioning/restart; SQL need does not imply managed Core |
| Automatic with no instance | Requested/unbound, Core unknown; existing planner must negotiate a supported declaration before provisioning |
| Missing runtime observation | Unverifiable, incomplete; registry existence does not prove running or absence |
| Stopped owned instance | Existing resource, stopped-but-owned; restart through existing lifecycle, no duplicate |
| Declared newly required realization | Additional resource; use actual provider plan, never inferred topology |
| HTTPS | Does not add Identity; dependencies come from selected provider facts |
| Explicit management Core | SQL/Secrets/Identity and security remain mandatory regardless of app needs |

The shared JSON projection is `baseharbor.footprint/v1`:

| Field | Meaning |
| --- | --- |
| version / target / application_id | Version, selected Target and known canonical app UUID |
| requirements | Requested capability, bound or requested/unbound, selected instance and observed state |
| existing / additional | Existing/shared/stopped/unverifiable realizations versus declared incremental requirements |
| unused | Target-wide registered instance IDs with no remaining bindings and outside the selected dependency union; **not** reclamation permission |
| core | required / not-required / unknown; unknown cannot authorize a core-less or managed mutation |
| complete | Structural resolution evidence complete; false for unbound, unknown Core/dependencies or unverifiable state |
| scope / ownership / shared / consumers | Existing registry scope/ownership; consumers is binding count, not invented app count |
| memory_bytes / containers | Per-realization measured / estimated / unknown evidence with source; unknown omits value |

Measured/estimated values require nonnegative actual values and a provenance
source. Estimates are supplied by reviewed provider adapters, never invented by
the resolver. Unknown measurements can coexist with structurally complete
resolution. No fixed RAM/container promises or unsubstantiated totals.
Full existing managed Core topology stays unchanged. Removal of one capability
only changes app intent/binding plans; unused providers remain visible and owned.
Reclamation is a separately authorized future operation.

`ManagementCoreRequirements` preserves #810. Application lifecycle consumers
use `Footprint.Core` as the common early decision: not-required allows supported
local workload execution without Core or login; required invokes existing
authorized reuse/bootstrap; unknown blocks premature mutation until resolved.
This does not permit unsecured `baha serve`, weaken test/prod or remote security,
or erase already installed Core resources.

CLI/human, JSON and MCP must use the same resolved domain view and existing
machine error/exit mappings. No CLI-only provider selection or second identity
state. Current resolver errors are typed `ResolutionError.Code`; adapters
translate them through existing ownership, conflict, validation and not-found
contracts. No new command/transport is introduced by this pure domain API.
Pinned executable fixtures: `internal/application/testdata/v0425-resolution.json`.

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

Existing Runtime.Permissions and Consumes are not service requirements in the portable projection. Their unmodeled dependencies force complete=false and Core unknown unless a selected provider already proves Core required. Absence from the service projection never proves Core-less safety; lifecycle consumers must verify the existing broker/consumption plan before mutation. This is a conservative limitation, not a second intent model.
