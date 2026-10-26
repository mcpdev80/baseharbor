package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// The applied intent remains authoritative for removed services. During retry
// the existing deployment owner retains its original intent explicitly.
func retainedApplicationBackendIntent(resolved resolvedApplication) (application.Manifest, error) {
	if resolved.DeploymentIdentity.DeploymentID == "" {
		return resolved.Manifest, nil
	}
	record, err := deployment.LoadDeploymentRecord(resolved.DeploymentIdentity)
	if errors.Is(err, os.ErrNotExist) {
		return resolved.Manifest, nil
	}
	if err != nil {
		return application.Manifest{}, err
	}
	intent := record.Applied.Intent
	if previous := record.Applied.GeneratedState["previous_intent"]; previous != "" {
		intent = json.RawMessage(previous)
	}
	var m application.Manifest
	if len(intent) == 0 || json.Unmarshal(intent, &m) != nil || m.ApplicationID != resolved.Manifest.ApplicationID || m.Environment != resolved.Manifest.Environment {
		return application.Manifest{}, invalidProviderConsumerInventory(fmt.Errorf("native backend intent differs from deployment owner"))
	}
	if err := m.ValidateIntent(); err != nil {
		return application.Manifest{}, err
	}
	key := "backend_placements"
	if record.Applied.GeneratedState["previous_intent"] != "" {
		key = "previous_backend_placements"
	}
	return retainedBackendPlacementProjection(resolved, m, record.Applied.GeneratedState[key])
}

func nativeBackendInventory(resolved resolvedApplication) (application.RuntimeFiles, application.Manifest, []bhruntime.ProjectResource, error) {
	prior, err := retainedApplicationBackendIntent(resolved)
	if err != nil {
		return application.RuntimeFiles{}, prior, nil, err
	}
	files := application.RuntimeFilesFor(resolved.Store, resolved.Manifest)
	intents, err := nativeBackendOwnerIntents(resolved)
	if err != nil {
		return files, prior, nil, err
	}
	if err := application.CheckExistingNativeBackendInventory(files, intents...); err != nil {
		return files, prior, nil, err
	}
	resources, err := nativeBackendOwnedResourceUnion(intents, files, nil)
	return files, prior, resources, err
}

// detectRuntimeForTargetFunc is the observer's runtime-detection seam. It
// defaults to detectRuntimeForTarget and lets failure regressions inject a
// deterministic detector/inspector outage without a live runtime gate.
var detectRuntimeForTargetFunc = detectRuntimeForTarget

// observeNativeBackendProviderFacts records one fact per provable native
// backend instance. Genuine detection, inspection, listing or ownership
// failures stay fail-closed and unverifiable: a registry entry can never
// substitute an inferred zero for a failed measurement. The only registry-only
// exception is the narrowly qualified missing-canonical-inventory preview
// condition (F1-F3): an exact registry binding for the instance plus a missing
// canonical runtime directory (or an absent canonical Store identity in the
// wizard preview, which performs no CWD-relative lookup at all). Every
// independently proven instance receives its own fact and a typed machine
// diagnostic so callers never consume the preview state as verified runtime
// success.
func observeNativeBackendProviderFacts(ctx context.Context, resolved resolvedApplication, facts map[string]application.ProviderFact) error {
	if isRemoteApplication(resolved) {
		return nil
	}
	dbg := func(tag string, err error) {
		if dir := os.Getenv("BAHA_DEBUG_PLAN_DIR"); dir != "" {
			if f, e := os.OpenFile(dir+"/observe.log", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600); e == nil {
				fmt.Fprintf(f, "%s native-backend %s err=%v\n", time.Now().UTC().Format("15:04:05.000"), tag, err)
				f.Close()
			}
		}
	}
	var files application.RuntimeFiles
	var expected []bhruntime.ProjectResource
	if strings.TrimSpace(resolved.Store.Root) == "" {
		// Wizard preview: no trusted canonical Store identity. Prove the
		// condition from registry and binding alone; never touch the CWD.
		files = application.RuntimeFilesFor(resolved.Store, resolved.Manifest)
		proven, err := recordCanonicalMissingRuntimeInventory(resolved, facts)
		if proven {
			return err
		}
	} else {
		inventoryFiles, _, inventoryExpected, err := nativeBackendInventory(resolved)
		files = inventoryFiles
		switch {
		case err == nil:
			expected = inventoryExpected
		case errors.Is(err, fs.ErrNotExist) && canonicalRuntimeDirectoryMissing(inventoryFiles):
			// The verified canonical runtime directory itself is absent.
			dbg("inventory", err)
			proven, derr := recordCanonicalMissingRuntimeInventory(resolved, facts)
			if proven {
				return derr
			}
		case errors.Is(err, fs.ErrNotExist):
			// The canonical directory exists; the ENOENT came from a missing
			// compose/env or later owner file. That is not absence proof. Fall
			// through with no expectation so the actual runtime must observe
			// or fail closed.
			dbg("inventory", err)
			expected = nil
		default:
			dbg("inventory", err)
			return nil
		}
	}
	provider, err := detectRuntimeForTargetFunc(ctx, resolved.Target)
	if err != nil {
		dbg("detect", err)
		return nil
	}
	if _, err := provider.InspectProjectResources(ctx, files.Project, expected); err != nil {
		dbg("inspect-expected", err)
		return nil
	}
	containers, err := provider.ListRuntimeContainers(ctx)
	if err != nil {
		dbg("list", err)
		return nil
	}
	registry, err := (capability.RegistryStore{Path: filepath.Join(resolved.TargetStateRoot, "provider-registry.json")}).Load()
	if err != nil {
		dbg("registry", err)
		return nil
	}
	intents, err := nativeBackendOwnerIntents(resolved)
	if err != nil {
		dbg("intents", err)
		return nil
	}
	for _, instance := range registry.Instances {
		resources, err := nativeBackendOwnedResourceUnion(intents, files, &instance)
		if err != nil || len(resources) == 0 {
			// A Target-registered application SQL instance can be intentionally
			// absent from the previous applied workload. Derive only its
			// canonical inspection names from the retained Target binding;
			// the actual runtime inspector must still prove measured absence.
			if instance.Provider.Kind != capability.ProviderPostgreSQL ||
				instance.Scope != capability.ScopeApplication ||
				instance.Ownership != capability.OwnershipBaseHarbor ||
				instance.OwnerApplicationID != resolved.Manifest.ApplicationID ||
				instance.OwnerEnvironment != resolved.Manifest.Environment {
				continue
			}
			name := ""
			for _, binding := range registry.Bindings {
				if binding.ProviderInstanceID == instance.ID &&
					binding.ApplicationID == resolved.Manifest.ApplicationID &&
					binding.Environment == resolved.Manifest.Environment &&
					binding.Resource.Kind == capability.SQL {
					name = binding.Resource.Name
				}
			}
			if name == "" {
				continue
			}
			inspectionManifest := application.WithSQLInstances(resolved.Manifest, name)
			resources, err = application.NativeBackendInstanceResources(inspectionManifest, files, instance)
			if err != nil || len(resources) == 0 {
				continue
			}
			if _, err := provider.InspectProjectResources(ctx, files.Project, resources); err != nil {
				continue
			}
		}
		count, state := observeCanonicalProviderContainersConfirmed(ctx, provider, provider.Kind(), files.Project, resources, containers)
		dbgset := state
		_ = dbgset
		if dir := os.Getenv("BAHA_DEBUG_PLAN_DIR"); dir != "" {
			if f, e := os.OpenFile(dir+"/observe.log", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600); e == nil {
				fmt.Fprintf(f, "%s native-set %s count=%v state=%s\n", time.Now().UTC().Format("15:04:05.000"), instance.ID[:40], count, state)
				f.Close()
			}
		}
		facts[instance.ID] = application.ProviderFact{Target: resolved.Target.Name, State: state, Core: application.CoreNotRequired, Dependencies: []string{}, Containers: application.Quantity{Value: &count, Classification: "measured", Source: "protected canonical applied backend intent and native ownership inventory"}, MemoryBytes: application.Quantity{Classification: "unknown"}}
	}
	return nil
}

// canonicalRuntimeDirectoryMissing reports whether the canonical runtime
// directory itself is absent. An ENOENT raised while the directory exists came
// from a missing compose/env or later owner file and never proves runtime
// absence.
func canonicalRuntimeDirectoryMissing(files application.RuntimeFiles) bool {
	_, err := os.Lstat(files.Dir)
	return errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOTDIR)
}

// recordCanonicalMissingRuntimeInventory is the narrowly qualified missing-
// canonical-inventory exception. It evaluates every registry instance
// independently: an owned, application-scoped PostgreSQL/Valkey instance needs
// an exact binding (provider instance, application UUID, environment, offered
// capability and logical resource name derived from the manifest) plus a
// canonical resource union derivable from the applied intents or, for a never
// materialized instance, from the proven binding resource name before it
// receives its own stopped-but-owned preview. One instance's proof is never transferred to a
// peer, and no proven instance short-circuits the others. The recorded fact is
// preview-only: it carries unknown quantities, never a measured zero, and is
// paired with the typed materialization diagnostic so consumers must
// explicitly distinguish the preview from verification and can never consume
// the registry projection as verified runtime success.
func recordCanonicalMissingRuntimeInventory(resolved resolvedApplication, facts map[string]application.ProviderFact) (bool, error) {
	intents, err := nativeBackendOwnerIntents(resolved)
	if err != nil {
		return false, nil
	}
	registry, err := (capability.RegistryStore{Path: filepath.Join(resolved.TargetStateRoot, "provider-registry.json")}).Load()
	if err != nil {
		return false, nil
	}
	files := application.RuntimeFilesFor(resolved.Store, resolved.Manifest)
	proven := 0
	var diagnostics []error
	for _, instance := range registry.Instances {
		if instance.Provider.Kind != capability.ProviderPostgreSQL && instance.Provider.Kind != capability.ProviderValkey {
			continue
		}
		if !applicationNativeBackendInstance(instance, resolved.Manifest) {
			continue
		}
		binding, ok := nativeBackendBindingProof(registry, instance, resolved.Manifest)
		if !ok {
			continue
		}
		resources, err := nativeBackendOwnedResourceUnion(intents, files, &instance)
		if err != nil {
			continue
		}
		if len(resources) == 0 {
			// The applied intents have not yet materialized this instance. Its
			// proven binding names the canonical resource, so derive the same
			// resource projection from the manifest (as the confirmed runtime
			// observer does for registry-bound application SQL). Dropping to
			// no-fact here would make the preview unverifiable while a later
			// observation of the same instance path measures it: exactly the
			// preview-vs-revalidate drift this prognostic fact removes.
			inspectionManifest := resolved.Manifest
			switch instance.Provider.Kind {
			case capability.ProviderPostgreSQL:
				inspectionManifest = application.WithSQLInstances(resolved.Manifest, binding.Resource.Name)
			case capability.ProviderValkey:
				inspectionManifest = application.WithCacheInstances(resolved.Manifest, binding.Resource.Name)
			}
			resources, err = application.NativeBackendInstanceResources(inspectionManifest, files, instance)
			if err != nil || len(resources) == 0 {
				continue
			}
		}
		facts[instance.ID] = application.ProviderFact{
			Target: resolved.Target.Name,
			State:  application.ProviderStoppedOwned,
			Core:   application.CoreNotRequired,
			// A registry projection is preview evidence, never a runtime
			// measurement: the container quantity stays unknown so no
			// verification or authorization consumer can accept it as a
			// measured zero (CMF20 R1).
			Dependencies: []string{},
			Containers:   application.Quantity{Classification: "unknown", Source: "preview-only registry projection: registry-bound owned application-scoped native backend " + string(binding.Resource.Kind) + "/" + binding.Resource.Name + " without a materialized canonical runtime inventory"},
			MemoryBytes:  application.Quantity{Classification: "unknown"},
		}
		diagnostics = append(diagnostics, runtimeInventoryNotYetMaterialized(resolved, instance, binding.Resource))
		proven++
	}
	return proven > 0, errors.Join(diagnostics...)
}

// nativeBackendBindingProof reports the exact consumer binding of a named,
// application-scoped native backend instance: same provider instance, same
// application UUID, same environment, a provider of the instance's own
// capability kind and the logical resource name the manifest derives to that
// instance. Missing, foreign-application, foreign-environment or
// foreign-resource bindings never match.
func nativeBackendBindingProof(registry capability.Registry, instance capability.ProviderInstance, m application.Manifest) (capability.ProviderBinding, bool) {
	name, ok := nativeBackendLogicalResourceName(instance, m)
	if !ok {
		return capability.ProviderBinding{}, false
	}
	for _, binding := range registry.Bindings {
		if binding.ProviderInstanceID != instance.ID ||
			binding.ApplicationID != m.ApplicationID ||
			binding.Environment != m.Environment ||
			binding.Resource.Provider != instance.Provider.Kind ||
			binding.Resource.Name != name {
			continue
		}
		var offered bool
		for _, capabilityKind := range instance.Provider.Capabilities {
			if binding.Resource.Kind == capabilityKind {
				offered = true
				break
			}
		}
		if !offered {
			continue
		}
		return binding, true
	}
	return capability.ProviderBinding{}, false
}

// nativeBackendLogicalResourceName derives the logical resource name whose
// canonical instance ID equals the registry instance for this manifest.
func nativeBackendLogicalResourceName(instance capability.ProviderInstance, m application.Manifest) (string, bool) {
	switch instance.Provider.Kind {
	case capability.ProviderPostgreSQL:
		for _, name := range application.SQLInstanceNames(m) {
			if application.ApplicationProviderInstanceIDForScopedPostgreSQL(m.ApplicationID, m.Environment, name) == instance.ID {
				return name, true
			}
		}
	case capability.ProviderValkey:
		for _, name := range application.ValkeyInstanceNames(m) {
			if application.ApplicationProviderInstanceIDForScopedValkey(m.ApplicationID, m.Environment, name) == instance.ID {
				return name, true
			}
		}
	}
	return "", false
}

// causeRuntimeInventoryNotYetMaterialized is the stable machine cause code of
// the preview-only materialization condition. It is distinct from verified
// runtime success and from genuine runtime failure.
const causeRuntimeInventoryNotYetMaterialized = "runtime-inventory-not-yet-materialized"

// runtimeInventoryNotYetMaterialized represents the condition as a typed
// machine error with stable cause/code plus instance and target context, so it
// propagates through the structured result/error path and machine JSON clients
// instead of being discarded as process-global stderr text.
func runtimeInventoryNotYetMaterialized(resolved resolvedApplication, instance capability.ProviderInstance, resource capability.Resource) error {
	return &machine.Error{
		Code:      machine.ErrorRuntimeUnavailable,
		CauseCode: causeRuntimeInventoryNotYetMaterialized,
		Resource:  instance.ID,
		Message:   "Native backend " + instance.ID + " runtime inventory is not yet materialized: the registry-bound owned instance is proven without a canonical runtime directory.",
		Next:      "Materialize the canonical runtime inventory through apply and re-observe; do not consume this stopped-but-owned preview fact as verified runtime success.",
		Details: map[string]any{
			"instance":         instance.ID,
			"provider":         string(instance.Provider.Kind),
			"capability":       string(resource.Kind),
			"logical_resource": resource.Name,
			"target":           resolved.Target.Name,
			"application_id":   resolved.Manifest.ApplicationID,
			"environment":      resolved.Manifest.Environment,
		},
	}
}

// providerFactIsRuntimeMeasurement reports whether an observed fact is backed
// by an actual runtime measurement. The preview-only registry projection
// never qualifies: it carries a stopped-owned preview state with unknown
// quantities, so verification and authorization consumers can explicitly
// distinguish preview from verification.
func providerFactIsRuntimeMeasurement(fact application.ProviderFact) bool {
	return fact.Containers.Classification == "measured" && fact.Containers.Value != nil
}

// materializationDiagnosticView projects the typed per-instance materialization
// diagnostics into a serialized machine.Error.Details view. Machine output
// reaches CLI/JSON consumers this way; machine.Error.Cause is json:"-"
// (internal/machine/contract.go), so a Go-only error chain is insufficient.
func materializationDiagnosticView(err error) any {
	var entries []map[string]any
	collectMaterializationDiagnostics(err, &entries)
	if len(entries) == 0 {
		return nil
	}
	return map[string]any{"materialization_diagnostics": entries}
}

func collectMaterializationDiagnostics(err error, entries *[]map[string]any) {
	switch typed := err.(type) {
	case nil:
		return
	case *machine.Error:
		if typed.CauseCode == causeRuntimeInventoryNotYetMaterialized {
			*entries = append(*entries, map[string]any{
				"code":     string(typed.Code),
				"cause":    typed.CauseCode,
				"resource": typed.Resource,
				"details":  typed.Details,
			})
		}
		collectMaterializationDiagnostics(typed.Cause, entries)
	case interface{ Unwrap() []error }:
		for _, element := range typed.Unwrap() {
			collectMaterializationDiagnostics(element, entries)
		}
	default:
		collectMaterializationDiagnostics(errors.Unwrap(err), entries)
	}
}

// withMaterializationDiagnostics attaches the typed materialization diagnostics
// of a preview observation to a fail-closed verification error so lifecycle
// CLI/JSON output preserves the stable cause instead of discarding the
// observer's return value.
func withMaterializationDiagnostics(err error, diagnostics error) error {
	if err == nil || diagnostics == nil {
		return err
	}
	view := materializationDiagnosticView(diagnostics)
	if view == nil {
		return err
	}
	var typed *machine.Error
	if errors.As(err, &typed) && typed != nil {
		if typed.Details == nil {
			typed.Details = view
		}
		typed.Cause = errors.Join(typed.Cause, diagnostics)
	}
	return err
}
