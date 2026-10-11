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
the plan incomplete. Automatic convergence then preserves resources; intentional
changes use the concrete consequence analysis and acknowledgement below.

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
ownership proof. Automatic convergence preserves shared-still-used resources;
intentional owned removal discloses every affected consumer. External/foreign
infrastructure is never deleted.
Repeat apply, interrupted retry and disable/re-enable converge idempotently.
Capability removal belongs to v0.4.25, not a later reclamation project.

CLI/human, JSON and MCP consume the same domain results and existing machine
error/exit/authorization mappings. ResolutionError remains typed. Fixtures:
`internal/application/testdata/v0425-resolution.json`; union invariants:
`internal/application/provider_union_test.go`. The full provider/capability wizard
is v0.4.26; #777 profile/host contracts below remain specification only.

## Provider changes: consequences and developer choice

Developers may add, change, replace, disable and remove functionality, including
choosing no continuation or complete deletion. Missing alternatives are disclosed
consequences, not a permanent policy veto. An alternative-less, unknown or
destructive outcome displays **Achtung** and requires **two independently explicit
acknowledgements of the concrete current plan**. Data deletion additionally needs
the existing distinct destructive authorization. Ordinary apply consent does not
implicitly acknowledge either stage or delete data.

`DescribeProviderChanges(snapshot, union, requests, observation)` enriches the same
resolved consumer union. It neither resolves another provider graph nor mutates
state. Direct provider operations and app/Target lifecycle adapters project their
requests into this API. `ProviderPlan.Impact` carries the shared optional result;
CLI, JSON/MCP and the later wizard consume the same `baseharbor.provider-impact/v1`.
The existing plan/preflight/confirmation/apply/verify authorities execute it.

| Domain result | Required meaning |
| --- | --- |
| before / after | Actual provider kind/instance, Target, scope/owner, locations and safe access/security references; proposed After is not an execution receipt |
| dependencies / dependents / functions | Affected apps and independent management/Console consumers; transitive dependency path, required/optional need, lost/changed/remaining function and observed expiry/validation behavior |
| scenario | Capability no longer requested; provider removed while still required; replacement; provider still used by another consumer; unused provider |
| materials | Individual current material and location, recovery references, intended retained/transferred/deleted/revoked/expired/invalidated/nonrecoverable/unknown disposition and destination/access |
| prerequisites | Concrete handoff, rebind or adapter-specific verification, support/evidence and consequence if unverified; no fictional fallback |
| effects | Applicable actual before/after availability, consistency, durability, retention, protocol, policy/TLS/HA, delivery, routing, reconnection or recovery changes from the adapter |
| complete | Knowledge completeness; false requires Achtung/two acknowledgements and does not prohibit the informed choice |
| plan_id / revision | Deterministic digest of consequences plus relevant binding/ownership/dependency state and existing-owner revision; materially changed facts invalidate old consent |
| confirmations_required / destructive | One ordinary stage, or two for alternative-less/unknown/destructive effects; explicit data-deletion consent remains separate |

`ProviderPlan.ConsumerActivity` includes explicitly inactive consumers from the
same intents; past runtime use is not mistaken for current need. Incidental
quantity sampling does not invalidate otherwise identical consequences.

`ProviderImpactObservation` is an ephemeral projection of existing installation,
deployment, runtime, registry, credential, certificate and recovery owners.
`IntegrationDescriptor.ChangeContract()` supplies the shipped provider's function,
material and operational inventory requirements. Iterate `ReferenceIntegrations()`;
there is no second provider registry or fixed product warning engine. Unknown
extension contracts and missing observations are visibly unknown. They are never
reported generically safe or successfully continued.

Consumer-specific `Continuations` prove a supported replacement and verified
rebind. A running compatible replacement by itself is insufficient. Verify secure
handoff/access before claiming continued function or deleting the old realization.
If no handoff exists, the developer can instead acknowledge the resulting lost
function twice and continue the requested removal. Retained inaccessible storage
is recovery, not secret delivery, token issuance or certificate renewal. Do not
write secrets to plaintext manifests, repositories, logs or an invented fallback.
Intentional removal of required authentication shows the affected service/access
as unavailable; it never silently enables unauthenticated management.

Only actual OpenBao usages produce secret-policy, dynamic-credential or PKI
consequences. Lease expiry/revocation and downstream validity require real evidence;
exporting a credential is not continuity proof. Current token/certificate validity
is distinct from future issuance, renewal, JWKS refresh, sessions and revocation.
Observed offline validation and expiry may produce changed rather than instantly
unavailable validation. Do not invent uniformly invalid tokens or certificates.
Caches, jobs, sessions, queues and telemetry buffers are not automatically
considered disposable. Unknown retention/location remains unknown. Retained
material stays at its actual location; deleted material says **no destination;
deletion**. Ownership and authorized references must be proven by existing owners.
Two acknowledgements grant no authority over somebody else's external resources.

Multiple changes form one aggregate plan. Removing a replacement's dependency
invalidates its continuation; stopping SQL identifies dependent Identity/broker
functions. Scoped unbinding preserves the other consumers' runtime and reports
which consumer loses access. Automatic last-consumer convergence preserves data;
explicit fully destructive requests inventory every affected item before consent.

`ValidateProviderChangeConsent` checks both stages, current plan digest and separate
deletion consent. Existing interactive and noninteractive authority adapters collect
acknowledgements; no extra CLI flags or independent frontend policy engine. Always
replan from current authoritative observations immediately before mutation. A stale
plan must be displayed and acknowledged again. Technical inability to execute an
unsupported handoff is reported honestly, not disguised as successful relocation.

`RecordProviderChangeResult` accepts actual verified execution outcomes, reports
complete/partial/failed state, actual final locations and every pending operation.
Actual affected-function outcomes require their own consumer/root/function
verification and evidence; a runtime stop alone is not a login/lease/PKI receipt.
It never copies proposed After into an accomplished result. Missing material,
changed disposition or missing recovery evidence remains a partial failure.
`PendingProviderChanges` preserves the original consumer scope and returns only
unfinished operations/material actions for the existing idempotent replan/retry
lifecycle. It is not a scheduler or new persistence authority. Interrupted handoff,
rollback/reconnection work and irreversible effects remain visible in receipts.

### Existing ancillary realizations

`ResolutionSnapshot.Ancillary` observes existing gateway/sidecar realizations from
runtime/route owners. `ProviderFact.Ancillary` and `OwnerReference` identify them;
`ProviderFootprint` and impact realizations retain that distinction. This is an
ephemeral inventory alongside the existing registry, not another registration or
state owner. Ancillary `Provider.Capabilities` is empty: Automatic does not offer
these as application providers, and explicit capability selection is incompatible.
The same dependency closure includes their actual IDs only through verified
provider dependencies or consumer roots. The same union accounts for shared users,
owned retirement, unknown state and impact. No unconditional sidecar/Core package.

A Target-owned shared gateway uses its real existing installation/route owner and
no fabricated application UUID. It does not claim that Caddy's selectable provider
integration supports generic Shared placement. App sidecars retain their actual
system-owned app ID. The native executor uses Shared scope and its single existing
Target project owner (`RuntimeExecutorDependency`); every app/management root
references that same physical ID. It is never owned by one app or counted through
independent app aliases. The per-app broker uses Application scope.

Ordinary declared workload components use distinct `runtime-workload` ancillary
metadata (`RuntimeWorkloadDependency`), including zero-capability apps. They are
not privileged executors or selectable capability providers. Actual run/access,
in-flight work, configuration and volume consequences require observation;
a matching project label does not authorize an undeclared component.
Runtime broker/executor metadata describes actual authorization,
credential, execution/control and in-flight effects; usages still require observation.

### Shipped provider coverage

This is the inventory of declaration-level coverage. The adapter must provide
**actual** data/access locations and usage; the material kinds below are potential
inventory categories, not assertions that every instance has every item. Runtime
entry-point coverage and native acceptance belong to the consuming lifecycle tests.
Existing runtime-broker/runtime-executor sidecars have ancillary-only consequence
metadata, not shipped selectable capability integrations. Undeclared extension
contracts remain unknown.

| Registered provider | Relevant functions/material inventory | Location/access authority and specific consequences |
| --- | --- | --- |
| PostgreSQL | Transactions/queries; database, schemas, WAL, credentials, backups | Existing database/runtime/recovery owners; transactions, consistency, durability, reconnect and recovery |
| Valkey | Cache/durable KV, actual sessions/jobs; snapshots/AOF | Existing runtime/data owners; expiry/eviction, durability and meaningful transient loss |
| OpenBao | Actual secret/policy, lease and PKI usage; secrets, keys/certificates, recovery | Existing credential/OpenBao/PKI owners and selected backing store; secure delivery, renewal, revocation and expiry |
| Caddy | HTTP routing and actual TLS/renewal; routes/configuration, keys/certificates | Existing exposure/PKI/runtime owners; DNS/routes, HTTPS availability and future renewal |
| SeaweedFS | Object read/write; objects/files, metadata, credentials, backups | Existing S3/runtime/recovery owners; durability, S3 compatibility, permissions and retention |
| OpenTelemetry Collector | Receive/forward; actual buffers/configuration | Existing telemetry/runtime owners; forwarding gaps, buffer loss, routing and retention |
| External OTLP | Actual receive/forward and registered references | External registration/access owner; unbind/rebind effects only, no foreign infrastructure deletion |
| Prometheus | Ingest/query and actual rules; metrics, WAL, configuration | Existing metrics/runtime/data owners; scrape gaps, retention, query access and recovery |
| Loki | Ingest/query; logs, WAL/index, configuration | Existing logs/runtime/storage owners; delivery gaps, retention and query access |
| Tempo | Ingest/query; traces, WAL/index, configuration | Existing traces/runtime/storage owners; delivery gaps, retention and query access |
| Keycloak | Actual login/SSO, issuance/validation/JWKS, sessions/refresh; users/realms/clients/signing keys/tokens | Existing Identity/database/access owners; login/refresh, cached validity and authenticated management availability |
| External OIDC | Actual OIDC consumers and registered references | External registration/access owner; local binding removal, no foreign user/realm destruction |
| RabbitMQ | Actual queue/pubsub/stream users; messages/in-flight, offsets/topology/credentials | Existing messaging/runtime/storage owners; ordering, acknowledgements, delivery, durability and reconnect |
| Ancillary runtime broker | Actual authorization/credentials/policy; configuration, certificates/recovery | Existing broker/runtime/credential owners; authorization, renewal, TLS and availability |
| Ancillary runtime workload | Actual component run/access; in-flight/configuration and volumes | Existing application/deployment owner; outage/reconnect and observed material retention, never privileged executor authority |
| Ancillary runtime executor | Actual execute/observe/control; in-flight/configuration, certificates/credentials | Existing executor/runtime owners; in-flight work, authorization/TLS and reconnect |
| MongoDB | Read/write/transactions; documents/indexes/oplog, credentials/backups | Existing document/runtime/storage/recovery owners; consistency, durability, protocol and reconnect |

Fixtures: `internal/application/testdata/v0425-provider-impact.json` and
`v0425-provider-impact-consent.json`. Domain tests cover every registered provider,
negative ownership/unknown state, cross-provider and two-app/Console effects,
replacement verification, aggregate dependency removal, scoped retry, actual final
and partial dispositions. Offline schemas validate identical machine results and
reject secret-content fields or one-stage unknown/destructive acknowledgements.
The complete capability/provider wizard remains v0.4.26.


### Derived consumer actions and lifecycle coverage

The lifecycle matrix covers **all declared functions and material kinds** for
all fourteen reference integrations, for add/resume/update, stop, destroy,
remove-runtime, disable/unbind, rebind and replacement. External runtime mutation
is rejected for lack of authority; an intentional local loss of access remains
possible. Declaration tests are not native execution acceptance.

`ProviderImpactObservation.ConsumerActions` projects supported operations of the
existing consumer adapter. `ProviderChangeRequest.ConsumerActions` explicitly
selects their IDs. Each action identifies consumer/root/function, actual modes,
selected function outcome, dependencies, credential references and locations,
and credential owner, synchronization and rotation before/after. An unselected
local authentication mode never becomes a fallback. Unsupported/unverified actions
remain unknown and can be explicitly accepted; they are never reported working.
Credential references stay identical unless their change is explicitly selected.
Adapters additionally verify that retained **values** and established synchronization
remain consistent; this pure result neither reads nor regenerates passwords.
OIDC ownership is distinct from native database/service credentials. For example,
Prometheus uses existing `devaccess` credentials and native basic authentication;
SeaweedFS keeps native admin references in its existing protected runtime owner.
Identity removal must not reset either merely because an issuer disappears.

`ObservedAt` and actual `ValidUntil`/`ExpiresAt` derive
`remaining_validity_seconds` for tokens, certificates and leases. Missing current
validity evidence stays unknown; expired offline validation is unavailable.
Retained certificate files imply neither indefinite TLS nor future renewal.

The consent digest ignores only the changing remaining-validity countdown; absolute
expiry, actual expired/valid consequences and changed material still require fresh consent.
`ProviderMaterial.Retention` separately verifies survival of stop and runtime
removal for meaningful transient state: cache, sessions, jobs, queues/messages,
in-flight work, telemetry buffers, leases and tokens. A volume alone is insufficient;
verified loss is individually nonrecoverable and requires destructive consent.

Actual execution receipts must account for each affected function, selected
consumer action and operational effect. Missing reconfiguration or changed owner,
credential reference, synchronization, rotation or guarantee remains partial/failed.
Retry retains selected consumer actions and obtains consent against the fresh plan.
A material step is complete for retry only when its verified disposition, final
location, delivery/access references and preserved recovery references all match;
the correct destination alone cannot hide an incomplete secure handoff.
Recovery references follow explicitly selected owned backup transfers/deletions
across the aggregate plan. A complete owned deletion may include its inventoried
backups with destructive consent; unselected or foreign recovery stays protected.

| Provider and existing implementation authority | Necessary provider-specific analysis/actions for add/change/remove |
| --- | --- |
| PostgreSQL — `application/runtime_postgres.go`, `shared_postgres_*` | Actual SQL consumers and Identity/backing-store dependents; provision/bind and verify access; version/protocol/transactions/WAL/backup consistency; reconnect dependent consumers after selected restore/replacement; distinguish preserved database from lost endpoint |
| Valkey — `application/shared_core_valkey.go`, `valkey_stable_verify.go`, `valkey_ha*`, `valkey_credential_rotation*` | Persistence/AOF/snapshot configuration versus cache/session/job semantics; verify access and actual expiry; rebind clients, preserve credentials; do not treat in-memory sessions/jobs as a preserved volume |
| OpenBao — `openbao/managed_credentials.go`, `service_pki*`, `runtime_pki*`, application dynamic-secret operations | Enumerate actual KV/policy/lease/PKI consumers and backing SQL; selected secure delivery handoff with permissions/encryption checks; preserve/revoke credentials deliberately; inventory certificate/key locations, remaining validity, renewal and revocation dependencies; reconfigure each selected consumer before teardown |
| Keycloak — `identityprovider/keycloak_files.go`, `keycloak_driver.go`, `application/identity_binding.go` | Actual shared SQL, gateway and OIDC consumers; realm/client/key/session inventory; supported login configuration change and access verification; distinguish offline token validity from JWKS/refresh/issuance; preserve separately owned native service credentials |
| External OIDC — `identityprovider` external realization and existing registration/binding owner | Verify issuer/discovery/client compatibility and every selected consumer mode; remove local bindings only; no authority over foreign accounts, sessions or signing keys |
| Caddy — `devgateway/render.go`, application exposure and PKI owners | Actual app/shared route users, DNS/endpoints, certificate/key references and renewal owner; verify selected routes/TLS/auth access; distinguish retained certificate validity from provider renewal and preserve unrelated routes |
| SeaweedFS — `objectstorage/admin_credentials.go`, `pki_rotation.go`, provider realization | Actual objects/files/metadata and S3 consumers; consistency/durability/permissions and selected handoff; preserve native admin/client credentials; verify TLS reload, endpoint rebinding and object access |
| RabbitMQ — `application/rabbitmq_credentials.go`, `rabbitmq_ha*`, credential rotation and verification | Actual queues/streams/topology, offsets, unacknowledged messages and jobs; ordering/delivery/acknowledgement and durable-versus-transient survival; preserve separate application/admin accounts; reconnect and verify publish/consume after change |
| MongoDB — `application/mongodb_runtime.go`, `mongodb_ha*`, credential rotation and verification | Actual documents/indexes/oplog and clients; protocol/transactions/consistency/durable data; preserve credentials, rebind clients and verify read/write and selected recovery |
| OTel Collector — `telemetry` realization, application telemetry binding and PKI reload | Actual receive/forward destinations, buffers and TLS references; delivery gaps/backpressure or irrecoverable buffered telemetry; configure and verify new routing before claiming downstream delivery |
| External OTLP — existing registered endpoint and telemetry binding owner | Protocol/TLS/auth and chosen forwarding endpoint; verify consumer rebinding and delivery; disclose gaps; no mutation/deletion of foreign buffers/stores |
| Prometheus — `metrics/prometheus_files.go`, `prometheus_shared.go`, topology/recovery owner | Actual scrape/query/rule users, TSDB/WAL and retention; preserve native basic-auth credentials independently of Identity; verify selected config, authentication and query access; disclose scrape gaps and lost alerts |
| Loki — `logs/render.go`, topology and PKI reload owner | Actual log delivery/query consumers, store/index/WAL, labels/protocol/retention; rebind writers/readers and verify access/TLS; disclose log gaps and retained-but-inaccessible history |
| Tempo — `traces/tempo_ha.go`, topology and PKI reload owner | Actual trace ingest/query consumers, buffers/store/index and retention; rebind delivery/query routes and verify TLS/access; distinguish sampling/delivery gaps from deleted history |
| Ancillary broker/executor — `runtimebroker/compose_render.go`, existing executor/runtime owners | Existing policy, service credentials/certificates and in-flight control/work consumers; authorization and expiry/renewal dependencies; verify selected reconnect/control operation; never register these sidecars as new capability offers |

The same dependency graph propagates database, gateway, secret/PKI and authentication
changes to every actual direct and transitive consumer. Selected transition
dependencies are evaluated against aggregate changes: removing the newly selected
mode's dependency cannot be reported as a working continuation. Runtime mutation
path inventory and native proof of these actions are Session 2's acceptance duty.

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
