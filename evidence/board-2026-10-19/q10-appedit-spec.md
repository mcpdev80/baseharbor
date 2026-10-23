# Q10: app.edit MCP Tool – Exact Implementation Spec

Extracted from tree `a66e5de6` (core-921 worktree) via `git show a66e5de6:<path>`.
Checked-out tree is being edited by another lane – never read live files.

---

## 1. The MCP Tool Name and Input Schema

### 1.1 Registered tool name

**`baseharbor.app.edit`** — registered in `app_registration_921.go` via
`registerMCPAppEditTool(server, store)`.

Tool description: "Propose or apply reviewed application needs with exact fresh
provider-impact consent."

### 1.2 Input schema – `appMCPEditInput`

Defined at `app_registration_921.go` line ~95 (git show a66e5de6):

```go
type appMCPEditInput struct {
    Target      string                             `json:"target" jsonschema:"exact resolved deployment target for this edit"`
    Name        string                             `json:"name,omitempty" jsonschema:"optional stored application name; omit inside an application repository"`
    Environment string                             `json:"environment,omitempty" jsonschema:"optional deployment environment selected from repository intent"`
    Phase       string                             `json:"phase" jsonschema:"required exactly \"propose\" or \"apply\""`
    Desired     appMachineDesiredRequest           `json:"desired" jsonschema:"Target-bound edit intent; required for both phases"`
    Proposal    *appMachineDesiredResult           `json:"proposal,omitempty" jsonschema:"typed proposal from the matching propose phase; required for apply"`
    Consent     *application.ProviderChangeConsent `json:"consent,omitempty" jsonschema:"fresh provider-consequence consent from the matching propose phase; required for apply"`
}
```

### 1.3 Phase: propose – wire shape

`appMachineDesiredRequest` (from `app_machine_adapters.go`):

```go
type appMachineDesiredRequest struct {
    Target           string                    `json:"target" jsonschema:"exact resolved deployment target for this edit"`
    SnapshotRevision string                    `json:"snapshot_revision,omitempty" jsonschema:"optional current snapshot revision matched during propose"`
    Requirements     []capability.Requirement  `json:"requirements,omitempty" jsonschema:"portable capability requirements to converge"`
    Choices          []appMachineDesiredChoice `json:"choices,omitempty" jsonschema:"optional per-requirement provider instance selections"`
}
```

`appMachineDesiredChoice` (from `app_machine_adapters.go`):

```go
type appMachineDesiredChoice struct {
    Requirement capability.Requirement `json:"requirement"`
    InstanceID  string                 `json:"instance_id,omitempty"`
}
```

`Requirement` (from `internal/capability/domain.go`):

```go
type Requirement struct {
    Kind Kind   `json:"kind"`
    Name string `json:"name"`
}
```

`Kind` values: `"database.sql"`, `"cache.key-value"`, `"database.key-value"`,
`"database.document"`, `"secrets"`, `"exposure.http"`, `"object-storage.s3"`,
`"telemetry.otlp"`, `"metrics"`, `"logs"`, `"traces"`, `"identity.oidc"`,
`"messaging.queue"`, `"messaging.pubsub"`, `"messaging.stream"`.

### 1.4 Phase: apply – wire shape

The apply phase requires **all** of:
- `Target` (string)
- `Desired` (appMachineDesiredRequest) — the exact same request object from propose
- `Proposal` (*appMachineDesiredResult) — the full typed proposal from propose
- `Consent` (*application.ProviderChangeConsent) — fresh consent for that plan

### 1.5 Proposal response – `appMachineDesiredResult` wire shape

```go
type appMachineDesiredResult struct {
    ApplicationID    string                           `json:"application_id"`
    Desired          appMachineDesiredRequest         `json:"desired"`
    Requirements     []capability.Requirement         `json:"requirements,omitempty"`
    Preferences      []application.ProviderPreference `json:"preferences,omitempty"`
    Choices          []appProviderChoice              `json:"choices,omitempty"`
    Plan             application.ProviderImpactPlan   `json:"plan"`
    SnapshotRevision string                           `json:"snapshot_revision,omitempty"`
    PlanRevision     string                           `json:"plan_revision,omitempty"`
}
```

### 1.6 `ProviderPreference` wire shape

```go
type ProviderPreference struct {
    Requirement  capability.Requirement `json:"requirement"`
    InstanceID   string                 `json:"instance_id,omitempty"`   // empty = automatic
    ProviderKind capability.ProviderKind `json:"provider_kind,omitempty"`
}
```

### 1.7 `appProviderChoice` wire shape

```go
type appProviderChoice struct {
    Target              string                 `json:"target"`
    Requirement         capability.Requirement `json:"requirement"`
    Candidates          []appProviderCandidate `json:"candidates,omitempty"`
    PreferredInstanceID string                 `json:"preferred_instance_id,omitempty"`
    ProposedInstanceID  string                 `json:"proposed_instance_id,omitempty"`
    Reason              string                 `json:"reason,omitempty"`
    BoundInstanceID     string                 `json:"bound_instance_id,omitempty"`
}
```

```go
type appProviderCandidate struct {
    Instance capability.ProviderInstance `json:"instance"`
    Reason   string                      `json:"reason,omitempty"`
}
```

---

## 2. Consent Struct

### 2.1 `ProviderChangeConsent`

From `internal/application/provider_change_types.go`:

```go
type ProviderChangeConsent struct {
    PlanID     string `json:"plan_id"`
    First      bool   `json:"first"`
    Second     bool   `json:"second"`
    DeleteData bool   `json:"delete_data"`
}
```

### 2.2 Consent validation flow

`ValidateProviderChangeConsent(plan ProviderImpactPlan, consent ProviderChangeConsent) error`
(from `internal/application/provider_change_consent.go`):

1. `validateProviderConsentRequirements(version, planID, confirmations)`:
   - `plan.Version != ProviderImpactVersion` → `ProviderChangeError{Code: "invalid-impact-plan"}`
   - `plan.PlanID == ""` → same
   - `confirmations < 1 || confirmations > 2` → same

2. `validateProviderConsent(planID, confirmations, destructive, consent)`:
   - `consent.PlanID != planID` → `"stale-impact-consent"`
   - `!consent.First` or `(confirmations == 2 && !consent.Second)` → `"impact-confirmation-required"`
   - `destructive && !consent.DeleteData` → `"data-deletion-consent-required"`

### 2.3 Consent error mapping in the adapter

From `app_machine_changeDesired()` / `appMachineChange()`:

| Error code from ValidateProviderChangeConsent | Adapter maps to |
|---|---|
| `"stale-impact-consent"` | `appMachineError{Code: "conflict"}` |
| `"invalid-impact-plan"` | `appMachineError{Code: "conflict"}` |
| `"impact-confirmation-required"` | `appMachineError{Code: "cancel"}` |
| `"data-deletion-consent-required"` | `appMachineError{Code: "cancel"}` |

---

## 3. How Registration / CLI Normally Authors the Manifest BEFORE Propose

The test does NOT hand-write the manifest. The normal flow is:

### 3.1 Source loading path (before propose)

1. **`appEditSourceBytes(resolved)`** (from `app_edit_apply_result.go`):
   - If `resolved.FromRepository`: reads `resolved.ManifestPath` directly
   - Otherwise: reads `{resolved.Store.Root}/{Manifest.Name}/baseharbor.yaml`

2. **`appMachineHydrate(ctx, target, source, read)`** (from `app_machine_adapters.go`):
   - Calls `appEditLoad(ctx, target, source, read)` which:
     - Parses YAML via `application.ParseYAML(source)` (or `ParseCanonicalYAML` as fallback)
     - Converts to `PortableContract` via `application.PortableContractFromManifest(manifest)`
     - Reads the existing authorized application context via `read(ctx, target)` → `GuidedReadContext`
     - Compares manifest intent against the footprint for compatibility
   - Returns `appEditLoaded` containing: `Source`, `Target`, `ApplicationID`,
     `Capabilities`, `Preferences`, `Complete`

### 3.2 The helper that projects wizard decisions back to manifest form

`appEditProjectRequirements(original Manifest, requirements []capability.Requirement)`
(from `app_edit_apply.go`):

- Clears all service booleans and instance lists to `false`/`nil`
- For each requirement, re-sets the service booleans and instance lists:
  - `database.sql` → `manifest.Services.SQL = true`, `WithSQLInstances(manifest, name)`
  - `cache.key-value` → `manifest.Services.Cache = true`, `WithCacheInstances(manifest, name)`
  - `database.key-value` → `manifest.Services.KeyValue = true`, `WithKeyValueInstances(manifest, name)`
  - `database.document` → `manifest.Services.DocumentDatabase = true`, `WithDocumentDatabaseInstances(manifest, name)`
  - `messaging.queue` → `manifest.Services.MessagingQueue = true`, `WithMessagingQueueInstances(manifest, name)`
  - `messaging.pubsub` → `manifest.Services.MessagingPubSub = true`, `WithMessagingPubSubInstances(manifest, name)`
  - `messaging.stream` → `manifest.Services.MessagingStream = true`, `WithMessagingStreamInstances(manifest, name)`
  - `object-storage.s3` → `manifest.Services.ObjectStorage = true`, `WithObjectStorageBuckets(manifest, name)`
  - `secrets` → `manifest.Services.Secrets = true`
  - `identity.oidc` → `manifest.Services.Identity = true`
  - `exposure.http` → copies from original (cannot be invented)
  - `metrics` → copies from original (cannot be invented)
  - `logs` → copies from original (cannot be invented)
  - `telemetry.otlp` → copies from original (cannot be invented)
  - `traces` → rejected: not representable by the current manifest contract

### 3.3 Manifest serialization path

- `appEditParseSource(loaded)` → parses YAML, sets `manifest.ApplicationID = loaded.ApplicationID`
- `appEditLoadSource(loaded)` → `bytes.Clone(loaded.Source)` for exact round-trip
- Authored YAML: `manifest.AuthoredYAML()` — clears ApplicationID before serializing
- Canonical YAML: `manifest.YAML()` — preserves ApplicationID

### 3.4 The `machineApplicationAuthoringTool` helper

For MCP tools that author manifests (not edit), `machineApplicationAuthoringTool[Input]()`
(from `mcp_application_authoring.go`):
- Uses `jsonschema.For[application.Manifest]()` to project the existing Manifest types
- Deletes `ApplicationID` from properties (system-owned, never authored)
- Sets required to `["Version", "Name", "Environment"]`
- Wraps the schema via `jsonschema.For[Input]()` with `TypeSchemas` map

---

## 4. Persisted-State Files an app.edit Apply Touches – Byte-Wise Snapshots

### 4.1 Binding state

**Read API**: `application.PreviewReferenceProviderRegistryAt(dataDir string, m Manifest)`
from `internal/application/provider_registry.go`:

```go
func PreviewReferenceProviderRegistryAt(dataDir string, m Manifest) (capability.Registry, error) {
    store, err := referenceProviderRegistryStoreAt(dataDir)
    registry, err := store.Load()
    registry.ReleaseApplicationDeployment(m.ApplicationID, m.Environment)  // release managed bindings
    registerReferenceProvidersAt(dataDir, &registry, m)  // re-register from data dir
    return registry, registry.Validate()
}
```

The `capability.Registry` structure:

```go
type Registry struct {
    Instances []capability.ProviderInstance `json:"instances"`
    Bindings  []capability.ProviderBinding    `json:"bindings"`
}
```

**Persisted file**: `provider-registry.json` at `{stateRoot}/provider-registry.json`
(set in test via `t.Setenv("BASEHARBOR_STATE_DIR", ...)` then `filepath.Join(stateRoot, "provider-registry.json")`).

**Write API** (for the binding test):
`capability.RegistryStore{Path: filepath.Join(stateRoot, "provider-registry.json")}.Save(available)`.

**Binding read for verification**: `bindingPreserved(bindings, instanceID)` checks:
- Iterate `bindings.Bindings`
- Match `binding.Resource.Kind == capability.SQL && binding.Resource.Name == "default" && binding.ProviderInstanceID == instanceID`

### 4.2 Manifest / source state

**Source file**: `{repoRoot}/baseharbor.yaml` (or `application.RepositoryManifestName`).

**Read**: `os.ReadFile(manifestPath)` in `appEditSourceBytes(resolved)`.

**Write**: `os.WriteFile(sourcePath, []byte(desired.AuthoredYAML()), 0o600)` — only in test.
Normal flow writes through `appEditSourceWriter` callback inside the Target lifecycle lock.

**Canonical record**: `repository_manifest.go`:
- `FindRepositoryManifest(start)` → walks up from start for `baseharbor.yaml`
- `LoadManifestFile(path)` → reads + parses YAML
- `SyncManifest(m Manifest, root string)` → stores canonical internal copy

### 4.3 Deployment record state

**Read API**: `deployment.LoadDeploymentRecord(resolved.DeploymentIdentity)` from `deployment_record.go`.

**Write APIs**:
- `recordPendingDeployment(ctx, resolved)` → `recordDeploymentBeforeMutation(ctx, resolved, "configured")`
- `recordAppliedDeployment(ctx, resolved, files)` → `recordAppliedDeploymentObservation(ctx, resolved, files, "ready", true)`
- `recordObservedDeployment(resolved, state, ready)`

**Persisted via**: `deployment.SaveDeploymentRecord(record)`.

**Record structure** (from `internal/deployment`):
```go
type DeploymentRecord struct {
    Version  int                    `json:"version"`
    Identity deployment.DeploymentIdentity `json:"identity"`
    Source   deployment.DeploymentSource `json:"source"`
    Applied  deployment.AppliedDeployment  `json:"applied"`
    Observed deployment.ObservedDeployment `json:"observed"`
}
```

**GeneratedState map** keys:
- `"deployment"` → resolved.DeploymentStateRoot
- `"runtime"` → files.Dir (on applied)
- `"state"` → `{DeploymentStateRoot}/state`
- `"previous_intent"` → on "applying" state

### 4.4 State directory layout

From test setup: `t.Setenv("BASEHARBOR_STATE_DIR", filepath.Join(t.TempDir(), "state"))`.

The state root is `{stateRoot}/provider-registry.json` for bindings.

---

## 5. Structured Error Surface

### 5.1 machine.Error codes used in the adapter

From `internal/machine/contract.go` — `ErrorCode` constants:

```go
const (
    ErrorValidationFailed         ErrorCode = "validation_failed"
    ErrorNotFound                 ErrorCode = "not_found"
    ErrorPortConflict             ErrorCode = "port_conflict"
    ErrorRequiredSecretMissing    ErrorCode = "required_secret_missing"
    ErrorSourceMissing            ErrorCode = "source_missing"
    ErrorApprovalRequired         ErrorCode = "approval_required"
    ErrorWorkloadStartFailed      ErrorCode = "workload_start_failed"
    ErrorImagePullFailed          ErrorCode = "image_pull_failed"
    ErrorAuthenticationFailed     ErrorCode = "authentication_failed"
    ErrorInvalidWorkload          ErrorCode = "invalid_workload"
    ErrorCapabilityMissing        ErrorCode = "capability_missing"
    ErrorPolicyDenied             ErrorCode = "policy_denied"
    ErrorConflict                 ErrorCode = "conflict"
    ErrorOwnershipAmbiguous       ErrorCode = "ownership_ambiguous"
    ErrorProviderUnavailable      ErrorCode = "provider_unavailable"
    ErrorRuntimeUnavailable       ErrorCode = "runtime_unavailable"
    ErrorHostResourceInsufficient ErrorCode = "host_resource_insufficient"
    ErrorTimeout                  ErrorCode = "timeout"
    ErrorVerificationFailed       ErrorCode = "verification_failed"
    ErrorUnsupported              ErrorCode = "unsupported_operation"
    ErrorInternal                 ErrorCode = "internal_error"
)
```

### 5.2 Adapter classification via `classifyAppMachineError()`

From `app_machine_adapters.go` — the internal `appMachineError.Code` maps to external `machine.Error`:

| Internal `appMachineError.Code` | External `machine.Error.Code` | Retryable |
|---|---|---|
| `"conflict"` | `ErrorConflict` | false |
| `"cancel"` | `ErrorRuntimeUnavailable` | true |
| `"denied"` | `ErrorPolicyDenied` | false |
| `"unsupported"` | `ErrorPolicyDenied` | false |
| `"partial"` | `ErrorVerificationFailed` | true |
| default | `ErrorInternal` | false |

### 5.3 Stale/conflict/approval classification in `appMachinePropose()`

| Condition | Error code |
|---|---|
| `read == nil || choices == nil || plan == nil || req.Target == ""` | `appMachineError{"unsupported"}` |
| `view.SnapshotRevision != req.SnapshotRevision` | `appMachineError{"conflict"}` |
| duplicate/empty requirements | `appMachineError{"conflict"}` |
| eligible map size mismatch | `appMachineError{"unsupported"}` |
| provider selection error | `appMachineError{"unsupported"}` |
| plan version/ID mismatch | `appMachineError{"conflict"}` |
| context canceled/timeout | mapped via `appMachineReadError()` → `appMachineError{"cancel"}` |

### 5.4 Stale/conflict/approval in `appMachineChangeDesired()`

| Condition | Error code |
|---|---|
| `authorize == nil || apply == nil` | `appMachineError{"unsupported"}` |
| `!reflect.DeepEqual(req.Desired, req.Proposal.Desired)` | `appMachineError{"conflict"}` |
| `req.Desired.SnapshotRevision == ""` | `appMachineError{"conflict"}` |
| Fresh propose diverges from proposal | `appMachineError{"conflict"}` |
| `authorize()` fails | mapped via `appMachineReadError()` |
| `ValidateProviderChangeConsent` → `"stale-impact-consent"` | `appMachineError{"conflict"}` |
| `ValidateProviderChangeConsent` → `"invalid-impact-plan"` | `appMachineError{"conflict"}` |
| `ValidateProviderChangeConsent` → `"impact-confirmation-required"` | `appMachineError{"cancel"}` |
| `ValidateProviderChangeConsent` → `"data-deletion-consent-required"` | `appMachineError{"cancel"}` |
| `RecordProviderChangeResult()` fails | `appMachineError{"partial"}` |
| `apply()` returns error | `appMachineError{"partial"}` + original |

### 5.5 Error struct wire shape

```go
type Error struct {
    Code        ErrorCode `json:"code"`
    Message     string    `json:"message"`
    CauseCode   string    `json:"cause,omitempty"`
    Retryable   bool      `json:"retryable,omitempty"`
    Next        string    `json:"next,omitempty"`
    Resource    string    `json:"resource,omitempty"`
    Remediation string    `json:"remediation_class,omitempty"`
    Details     any       `json:"details,omitempty"`
    Cause       error     `json:"-"`
}
```

### 5.6 ProviderChangeError codes

```go
type ProviderChangeError struct{ Code string }
```

Codes: `"invalid-impact-plan"`, `"stale-impact-consent"`, `"impact-confirmation-required"`,
`"data-deletion-consent-required"`, `"stale-impact-result"`, `"invalid-provider-union"`,
`"provider-instance-missing"`, `"conflicting-provider-changes"`, `"foreign-provider-target"`,
`"external-provider-runtime-not-owned"`, `"replacement-provider-missing"`,
`"unsupported-change-operation"`.

---

## 6. Full MCP Apply Entry Point (for context)

The MCP tool `baseharbor.apply` uses `machineApplyInput`:

```go
type machineApplyInput struct {
    ProviderChangeConsent *application.ProviderChangeConsent `json:"provider_change_consent,omitempty"`
    Target                string                             `json:"target,omitempty"`
    Name                  string                             `json:"name,omitempty"`
    Environment           string                             `json:"environment,omitempty"`
    SkipMemoryPreflight   bool                               `json:"skip_memory_preflight,omitempty"`
}
```

---

Q10-SPEC-DONE
