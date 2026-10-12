package main

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/machine"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// Resolve only through the existing Target registry and native provider plan.
// Registration is projected in memory; only the normal apply owner persists it.
func resolveApplicationLifecycleFootprint(resolved resolvedApplication) (application.Footprint, error) {
	return resolveApplicationLifecycleFootprintWithFacts(resolved, nil)
}

func resolveApplicationLifecycleFootprintWithFacts(resolved resolvedApplication, observations map[string]application.ProviderFact) (application.Footprint, error) {
	snapshot, preferences, err := applicationLifecycleSnapshot(resolved, observations)
	if err != nil {
		return application.Footprint{}, err
	}
	footprint, err := application.ResolveFootprint(resolved.Manifest, snapshot, preferences)
	return footprint, classifyApplicationResolutionError(err)
}

func applicationLifecycleSnapshot(resolved resolvedApplication, observations map[string]application.ProviderFact) (application.ResolutionSnapshot, []application.ProviderPreference, error) {
	registry, err := (capability.RegistryStore{Path: filepath.Join(resolved.TargetStateRoot, "provider-registry.json")}).Load()
	if err != nil {
		return application.ResolutionSnapshot{}, nil, err
	}
	planned, err := application.PreviewReferenceProviderRegistryAt(resolved.TargetStateRoot, resolved.Manifest)
	if err != nil {
		return application.ResolutionSnapshot{}, nil, err
	}
	for _, binding := range planned.Bindings {
		if binding.ApplicationID != resolved.Manifest.ApplicationID || binding.Environment != resolved.Manifest.Environment {
			continue
		}
		for _, instance := range planned.Instances {
			if instance.ID == binding.ProviderInstanceID && instance.Ownership == capability.OwnershipExternal && instance.Provider.Kind != capability.ProviderExternalOIDC && instance.Provider.Kind != capability.ProviderExternalOTLP {
				return application.ResolutionSnapshot{}, nil, &machine.Error{Code: machine.ErrorUnsupported, CauseCode: "external_provider_adapter_unavailable", Resource: instance.ID, Message: "The selected BYO provider has no compatible application lifecycle adapter.", Next: "Use an existing supported binding; BaseHarbor will neither install Core nor replace this external provider."}
			}
		}
	}
	for _, old := range registry.Bindings {
		if old.ApplicationID != resolved.Manifest.ApplicationID || (old.Environment != "" && old.Environment != resolved.Manifest.Environment) {
			continue
		}
		for _, wanted := range planned.Bindings {
			if wanted.ApplicationID == old.ApplicationID && wanted.Environment == resolved.Manifest.Environment && wanted.Resource.Kind == old.Resource.Kind && wanted.Resource.Name == old.Resource.Name && wanted.ProviderInstanceID != old.ProviderInstanceID {
				return application.ResolutionSnapshot{}, nil, &machine.Error{Code: machine.ErrorUnsupported, CauseCode: "provider_binding_migration_required", Message: "The existing provider binding differs from the current native placement plan.", Next: "Restore the existing placement; provider migration requires an explicit supported operation."}
			}
		}
	}
	existing := map[string]bool{}
	for _, instance := range registry.Instances {
		existing[instance.ID] = true
	}
	contract, err := application.PortableContractFromManifest(resolved.Manifest)
	if err != nil {
		return application.ResolutionSnapshot{}, nil, err
	}
	requested := map[capability.Requirement]bool{}
	for _, requirement := range contract.Capabilities {
		requested[requirement] = true
	}
	if contract.Secrets.Managed {
		requested[capability.Requirement{Kind: capability.Secrets, Name: "default"}] = true
	}
	var preferences []application.ProviderPreference
	actualBindings := planned.Bindings[:0]
	for _, binding := range planned.Bindings {
		if binding.ApplicationID != resolved.Manifest.ApplicationID || binding.Environment != resolved.Manifest.Environment {
			actualBindings = append(actualBindings, binding)
			continue
		}
		requirement := capability.Requirement{Kind: binding.Resource.Kind, Name: binding.Resource.Name}
		if requested[requirement] {
			preferences = append(preferences, application.ProviderPreference{Requirement: requirement, InstanceID: binding.ProviderInstanceID})
		}
		for _, old := range registry.Bindings {
			if old == binding {
				actualBindings = append(actualBindings, binding)
				break
			}
		}
	}
	planned.Bindings = registry.Bindings
	snapshot := application.ResolutionSnapshot{DependencyRoots: []string{}, Target: resolved.Target.Name, Registry: planned, Facts: map[string]application.ProviderFact{}}
	snapshot.DependencyRoots, err = application.RuntimeDependencyRoots(resolved.Manifest, planned)
	if err != nil {
		return application.ResolutionSnapshot{}, nil, err
	}
	for _, instance := range planned.Instances {
		// Standalone native realizations use service-access PKI without a
		// management installation. Embedded provider dependencies remain in
		// their canonical renderer; no unrelated provider bundle is inferred.
		fact := application.ProviderFact{Target: resolved.Target.Name, Core: application.CoreNotRequired, State: application.ProviderNewRequired, Dependencies: []string{}}
		if instance.Provider.Kind == capability.ProviderOpenBao {
			fact.Core = application.CoreRequired
			fact.Dependencies = []string{application.RuntimeStorageDependencyID(resolved.Target.Name)}
		}
		if instance.Scope == capability.ScopeShared && instance.Ownership == capability.OwnershipBaseHarbor && (instance.Provider.Kind == capability.ProviderPostgreSQL || instance.Provider.Kind == capability.ProviderValkey) {
			module := application.SharedBackendRuntimeDependency(resolved.Target.Name, instance.Provider.Kind)
			fact.Dependencies = []string{module.ID}
		}
		if existing[instance.ID] {
			// Registry presence is not a live or stopped-state observation.
			fact.State = application.ProviderUnverifiable
		}
		if instance.Ownership == capability.OwnershipExternal {
			fact.State, fact.Core = application.ProviderUnverifiable, application.CoreNotRequired
			fact.Dependencies = []string{}
		}
		if observation, ok := observations[instance.ID]; ok && existing[instance.ID] {
			fact = observation
		}
		if instance.Provider.Kind == capability.ProviderKeycloak && instance.Ownership == capability.OwnershipBaseHarbor {
			var dependencyErr error
			fact, dependencyErr = appendIdentitySQLDependency(resolved, instance, fact, observations, &snapshot)
			if dependencyErr != nil {
				return application.ResolutionSnapshot{}, nil, dependencyErr
			}
		}
		snapshot.Facts[instance.ID] = fact
	}
	appendSharedBackendDependencyObservations(resolved, observations, &snapshot)
	if err := appendApplicationBrokerDependencies(resolved, observations, &snapshot); err != nil {
		return application.ResolutionSnapshot{}, nil, err
	}
	// Observe backing modules in the same resolver without creating competing
	// selectable offers or another persistent ownership record. Identity may
	// reference this exact module even when the desired app has no broker.
	storage := application.RuntimeStorageDependency(resolved.Target.Name)
	needed := false
	retainedBroker := false
	for _, fact := range snapshot.Facts {
		for _, id := range fact.Dependencies {
			needed = needed || id == storage.ID
			retainedBroker = retainedBroker || id == storage.ID && fact.State != application.ProviderNewRequired
		}
	}
	if needed {
		snapshot.Ancillary = append(snapshot.Ancillary, storage)
		fact := application.ProviderFact{Target: resolved.Target.Name, State: application.ProviderNewRequired, Core: application.CoreRequired, Dependencies: []string{}, Ancillary: true, OwnerReference: "runtime/" + bhruntime.SharedProjectName(resolved.Target.Name)}
		if retainedBroker {
			fact.State = application.ProviderUnverifiable
		}
		if observation, ok := observations[storage.ID]; ok {
			fact = observation
		}
		snapshot.Facts[storage.ID] = fact
	}
	return snapshot, preferences, nil
}

// Translate domain outcomes once at the common CLI/JSON/MCP/HTTP consumer.
// Existing machine error envelopes and exit-code mappings remain authoritative.
func classifyApplicationResolutionError(err error) error {
	if err == nil {
		return nil
	}
	var existing *machine.Error
	if errors.As(err, &existing) {
		return err
	}
	var resolution *application.ResolutionError
	if !errors.As(err, &resolution) {
		return err
	}
	code := machine.ErrorInternal
	switch resolution.Code {
	case "missing", "missing-dependency":
		code = machine.ErrorNotFound
	case "incompatible", "dependency-cycle":
		code = machine.ErrorValidationFailed
	case "ambiguous", "binding-conflict":
		code = machine.ErrorConflict
	case "foreign", "foreign-target":
		code = machine.ErrorOwnershipAmbiguous
	}
	return &machine.Error{Code: code, CauseCode: "provider_resolution_" + strings.ReplaceAll(resolution.Code, "-", "_"), Message: err.Error(), Resource: resolution.InstanceID, Next: "Reconcile the existing Target provider binding and dependency inventory before retrying; no provider was mutated.", Cause: err}
}

func requireResolvedApplicationCore(ctx context.Context, resolved resolvedApplication, in io.Reader, out io.Writer) error {
	if err := preflightRepositoryWorkload(resolved); err != nil {
		return err
	}
	footprint, err := resolveApplicationLifecycleFootprint(resolved)
	if err != nil {
		return err
	}
	if len(footprint.Requirements) == 0 && !application.HasExplicitWorkload(resolved.Manifest) {
		return &machine.Error{Code: machine.ErrorInvalidWorkload, CauseCode: "workload_missing", Message: "Application has no supported workload or requested runtime services.", Next: "Add a supported Compose workload and initialize the application before starting it."}
	}
	if footprint.Core == application.CoreUnknown {
		return &machine.Error{Code: machine.ErrorCapabilityMissing, CauseCode: "provider_dependencies_unverifiable", Message: "Selected consumer dependencies have no verified realization inventory.", Next: "Resolve the existing consumption and Target dependency bindings before applying."}
	}
	if _, err := resolveApplicationProviderPlan(resolved, nil); err != nil {
		return err
	}
	return nil
}

// Materialization belongs to the acknowledged mutation phase, never to the
// read-only prerequisite decision used by init and plan.
func materializeResolvedApplicationBroker(ctx context.Context, resolved resolvedApplication, in io.Reader, out io.Writer) error {
	if !application.RequiresRuntimeBroker(resolved.Manifest) {
		return nil
	}
	// The broker's actual authorization/PKI dependency is OpenBao and its
	// selected SQL storage. Identity is not a dependency of this realization.
	selected := withTargetOverride(ctx, resolved.Target.Name)
	options := runtimeUpOptions{Yes: true, HA: application.ComponentHA(resolved.Manifest, "secrets"), ControlPlaneOnly: true}
	if err := materializeControlPlaneProviders(selected, in, out, options); err != nil {
		return err
	}
	return ensureRepositoryOpenBaoReady(selected, in, out, io.Discard, options)
}

// All read surfaces share the same desired plan and Target resolver snapshot.
type applicationLifecyclePlan struct {
	application.Plan
	Footprint         application.Footprint        `json:"footprint"`
	RetainedResources []capability.ProviderBinding `json:"retained_resources,omitempty"`
	ProviderPlan      application.ProviderPlan     `json:"provider_plan"`
}

func buildResolvedApplicationPlan(ctx context.Context, resolved resolvedApplication) (applicationLifecyclePlan, error) {
	plan, err := application.BuildPlan(resolved.Manifest)
	if err != nil {
		return applicationLifecyclePlan{}, err
	}
	snapshot, preferences, err := applicationLifecycleSnapshot(resolved, observeApplicationProviderFacts(ctx, resolved))
	if err != nil {
		return applicationLifecyclePlan{}, err
	}
	footprint, err := application.ResolveFootprint(resolved.Manifest, snapshot, preferences)
	if err != nil {
		return applicationLifecyclePlan{}, classifyApplicationResolutionError(err)
	}
	retained, err := removedApplicationProviderBindings(resolved)
	if err != nil {
		return applicationLifecyclePlan{}, err
	}
	union, err := resolveApplicationProviderPlanFromSnapshot(resolved, snapshot, preferences)
	if err != nil {
		return applicationLifecyclePlan{}, err
	}
	requests := applicationProviderChangeRequests(union)
	requests, err = nativeBackendChangeRequests(resolved, requests)
	if err != nil {
		return applicationLifecyclePlan{}, err
	}
	if len(requests) > 0 {
		impact, err := resolveApplicationProviderImpact(ctx, resolved, snapshot, union, requests)
		if err != nil {
			return applicationLifecyclePlan{}, err
		}
		union.Impact = &impact
	}
	return applicationLifecyclePlan{Plan: plan, Footprint: footprint, RetainedResources: retained, ProviderPlan: union}, nil
}

func removedApplicationProviderBindings(resolved resolvedApplication) ([]capability.ProviderBinding, error) {
	registry, err := (capability.RegistryStore{Path: filepath.Join(resolved.TargetStateRoot, "provider-registry.json")}).Load()
	if err != nil {
		return nil, err
	}
	contract, err := application.PortableContractFromManifest(resolved.Manifest)
	if err != nil {
		return nil, err
	}
	requested := map[capability.Requirement]bool{}
	for _, requirement := range contract.Capabilities {
		requested[requirement] = true
	}
	if contract.Secrets.Managed {
		requested[capability.Requirement{Kind: capability.Secrets, Name: "default"}] = true
	}
	var removed []capability.ProviderBinding
	for _, binding := range registry.Bindings {
		if binding.ApplicationID == resolved.Manifest.ApplicationID && (binding.Environment == "" || binding.Environment == resolved.Manifest.Environment) && !requested[capability.Requirement{Kind: binding.Resource.Kind, Name: binding.Resource.Name}] {
			removed = append(removed, binding)
		}
	}
	return removed, nil
}

func checkRetainedApplicationProviderOwnership(ctx context.Context, resolved resolvedApplication) error {
	registry, err := (capability.RegistryStore{Path: filepath.Join(resolved.TargetStateRoot, "provider-registry.json")}).Load()
	if err != nil {
		return err
	}
	facts := observeApplicationProviderFacts(ctx, resolved)
	for _, instance := range registry.Instances {
		if instance.Scope != capability.ScopeApplication || instance.OwnerApplicationID != resolved.Manifest.ApplicationID || (instance.OwnerEnvironment != "" && instance.OwnerEnvironment != resolved.Manifest.Environment) {
			continue
		}
		if len(registry.BindingsForProviderInstance(instance.ID)) == 0 {
			fact, verified := facts[instance.ID]
			if !verified || (fact.State != application.ProviderRunning && fact.State != application.ProviderStoppedOwned) || fact.Containers.Value == nil {
				return &machine.Error{Code: machine.ErrorOwnershipAmbiguous, CauseCode: "retained_provider_ownership_unverified", Resource: instance.ID, Message: "Retained provider ownership could not be verified through its canonical deployment and native renderer.", Next: "Inspect the existing owner and retry; runtime and data ownership remain available."}
			}
		}
	}
	return nil
}

// Observe existing native owners even after their capability leaves desired
// intent. The common resolver remains the sole dependency/removal authority.
func observeApplicationProviderFacts(ctx context.Context, resolved resolvedApplication) map[string]application.ProviderFact {
	facts := observeIdentityProviderFacts(ctx, resolved)
	observeSharedBackendProviderFacts(ctx, resolved, facts)
	observeBrokerProviderFacts(ctx, resolved, facts)
	observeApplicationBrokerFacts(ctx, resolved, facts)
	owners, err := applicationRuntimeOwners(resolved)
	if err == nil {
		for _, owner := range owners {
			observeStandaloneProviderFacts(ctx, owner, facts)
			observeNativeBackendProviderFacts(ctx, owner, facts)
			observeApplicationExposureFacts(ctx, owner, facts)
		}
	}
	return facts
}
