package main

import (
	"context"
	"errors"
	"fmt"
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
	return applicationLifecycleSnapshotContext(context.Background(), resolved, observations)
}

func applicationLifecycleSnapshotContext(ctx context.Context, resolved resolvedApplication, observations map[string]application.ProviderFact) (application.ResolutionSnapshot, []application.ProviderPreference, error) {
	// A poisoned registry (two records claiming one application_id) breaks the
	// projection reads below with a bare sentinel wrap; classify it once here
	// into the structured inventory error so EVERY planning surface — this
	// snapshot, the union plan and the footprint — fails closed with the same
	// repair text (0427-175).
	registry, err := (capability.RegistryStore{Path: filepath.Join(resolved.TargetStateRoot, "provider-registry.json")}).Load()
	if err != nil {
		return application.ResolutionSnapshot{}, nil, classifyInventoryConflict(err)
	}
	planned, err := application.PreviewReferenceProviderRegistryAt(resolved.TargetStateRoot, resolved.Manifest)
	if err != nil {
		return application.ResolutionSnapshot{}, nil, classifyInventoryConflict(err)
	}
	// Adapter availability is execution admission, not a veto on read-only
	// consequence planning. Preserve the actual selected offers and bindings.
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
	actualBindings := make([]capability.ProviderBinding, 0, len(planned.Bindings))
	knownBindings := make(map[capability.ProviderBinding]bool, len(planned.Bindings))
	for _, binding := range planned.Bindings {
		if binding.ApplicationID != resolved.Manifest.ApplicationID || binding.Environment != resolved.Manifest.Environment {
			actualBindings = append(actualBindings, binding)
			knownBindings[binding] = true
			continue
		}
		requirement := capability.Requirement{Kind: binding.Resource.Kind, Name: binding.Resource.Name}
		if requested[requirement] {
			preferences = append(preferences, application.ProviderPreference{Requirement: requirement, InstanceID: binding.ProviderInstanceID, ProviderKind: binding.Resource.Provider})
		}
		// Keep a binding only if the selected provider is already applied. A
		// newly selected replacement remains a preference until the normal
		// acknowledged apply publishes it; the old binding is handled by the
		// replacement planners below.
		for _, old := range registry.Bindings {
			if old == binding {
				actualBindings = append(actualBindings, binding)
				knownBindings[binding] = true
				break
			}
		}
	}
	// A removed capability may still have an applied binding in Target state.
	// Preserve that binding as retained ownership without reviving it as desired
	// intent. For still-requested capabilities, the projected provider above is
	// the desired selection and the change planners compare it with actual state.
	for _, binding := range registry.Bindings {
		if binding.ApplicationID != resolved.Manifest.ApplicationID || binding.Environment != resolved.Manifest.Environment {
			continue
		}
		requirement := capability.Requirement{Kind: binding.Resource.Kind, Name: binding.Resource.Name}
		if requested[requirement] || knownBindings[binding] {
			continue
		}
		actualBindings = append(actualBindings, binding)
		knownBindings[binding] = true
	}
	planned.Bindings = actualBindings
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
	if _, err := appendManagementProviderDependencies(ctx, resolved, observations, &snapshot); err != nil {
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
	resource := resolution.InstanceID
	next := "Reconcile the existing Target provider binding and dependency inventory before retrying; no provider was mutated."
	causeCode := "provider_resolution_" + strings.ReplaceAll(resolution.Code, "-", "_")
	if resolution.Consumer != "" && resolution.Code == "missing-dependency" {
		// The union resolver attributes a failed edge to the authoritative
		// consumer that references it. A stale reference left by another
		// application's record is therefore reported as the concrete conflict
		// it is, naming that consumer and its recovery owner.
		causeCode = "provider_stale_consumer_dependency"
		resource = resolution.Consumer
		next = fmt.Sprintf("Consumer %q references provider realization %q, which no longer exists on the Target. Restore that realization or release the named consumer through its own lifecycle owner, then rerun the last command; no provider was mutated.",
			resolution.Consumer, resolution.InstanceID)
	} else if resolution.Consumer != "" {
		// ResolveProviderPlan annotates all resolver failures with their owner,
		// not only missing dependency edges. Keep conflict/incompatibility
		// causes precise instead of misreporting them as stale consumers.
		next = fmt.Sprintf("Resolve provider selection for consumer %q and retry; no provider was mutated.", resolution.Consumer)
	}
	switch resolution.Code {
	case "missing", "missing-dependency":
		code = machine.ErrorNotFound
		if resolution.Consumer == "" {
			if resolution.InstanceID == "" {
				// An unmaterialized requirement names no instance; the
				// requirement itself is the resource the operator can act on.
				resource = string(resolution.Requirement.Kind) + "/" + resolution.Requirement.Name
			}
			next = fmt.Sprintf("Materialize or reconcile the provider realization %q, then rerun the last command; no provider was mutated.", resource)
		}
	case "incompatible", "dependency-cycle":
		code = machine.ErrorValidationFailed
	case "ambiguous", "binding-conflict":
		code = machine.ErrorConflict
	case "foreign", "foreign-target":
		code = machine.ErrorOwnershipAmbiguous
	}
	diagnostic := applicationResolutionDiagnostic{
		Code: resolution.Code, Consumer: resolution.Consumer, InstanceID: resolution.InstanceID,
		Requirement: string(resolution.Requirement.Kind) + "/" + resolution.Requirement.Name,
	}
	return &machine.Error{Code: code, CauseCode: causeCode, Message: err.Error(), Resource: resource, Next: next, Details: diagnostic, Cause: err}
}

// applicationResolutionDiagnostic contains only stable resolver identifiers.
// It lets proposal failures retain the actual failing edge without exposing
// filesystem paths, registry payloads, or runtime credentials.
type applicationResolutionDiagnostic struct {
	Code        string `json:"resolution_code"`
	Consumer    string `json:"consumer,omitempty"`
	InstanceID  string `json:"instance_id,omitempty"`
	Requirement string `json:"requirement,omitempty"`
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
	return buildResolvedApplicationPlanWithFacts(ctx, resolved, observeApplicationProviderFacts(ctx, resolved))
}

// buildResolvedApplicationPlanWithFacts lets callers that already took a
// provider observation use the same fresh facts for their snapshot and plan.
func buildResolvedApplicationPlanWithFacts(ctx context.Context, resolved resolvedApplication, observations map[string]application.ProviderFact) (applicationLifecyclePlan, error) {
	plan, err := application.BuildPlan(resolved.Manifest)
	if err != nil {
		return applicationLifecyclePlan{}, err
	}
	snapshot, preferences, err := applicationLifecycleSnapshotContext(ctx, resolved, observations)
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
	requests, err = nativeBackendChangeRequestsForPlan(resolved, requests)
	if err != nil {
		return applicationLifecyclePlan{}, err
	}
	requests, err = standaloneProviderVersionChangeRequestsForPlan(ctx, resolved, requests)
	if err != nil {
		return applicationLifecyclePlan{}, err
	}
	requests, err = backendBindingChangeRequests(resolved, union, requests)
	if err != nil {
		return applicationLifecyclePlan{}, err
	}
	requests, err = externalReceiverChangeRequests(resolved, requests)
	if err != nil {
		return applicationLifecyclePlan{}, err
	}
	requests, err = externalIdentityChangeRequests(resolved, requests)
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
			if resolvedRuntimeProjectionAbsent(resolved) {
				absent, err := retainedProviderOwnershipVerifiedAbsent(ctx, resolved, instance)
				if err != nil {
					var ambiguous *machine.Error
					if errors.As(err, &ambiguous) && ambiguous.CauseCode == "retained_provider_ownership_unverified" {
						return err
					}
					continue
				}
				if absent {
					// Verified never-realized owner of an interrupted first
					// apply (0428-286): the application runtime projection was
					// never materialized and the retained intent names no
					// canonical realization resource for this owner, so the
					// registry entry is a retained intention, not
					// infrastructure. This application's unbound,
					// BaseHarbor-owned owner is reclaimed; bound, shared,
					// foreign or realizable resources keep the refusal below.
					if err := reclaimUnboundRetainedProviderOwner(resolved, instance.ID); err != nil {
						return err
					}
					registry, err = (capability.RegistryStore{Path: filepath.Join(resolved.TargetStateRoot, "provider-registry.json")}).Load()
					if err != nil {
						return err
					}
					continue
				}
			}
			fact, verified := facts[instance.ID]
			if !verified || (fact.State != application.ProviderRunning && fact.State != application.ProviderStoppedOwned) || fact.Containers.Value == nil {
				return &machine.Error{Code: machine.ErrorOwnershipAmbiguous, CauseCode: "retained_provider_ownership_unverified", Resource: instance.ID, Message: "Retained provider ownership could not be verified through its canonical deployment and native renderer.", Next: "Inspect the existing owner and retry; runtime and data ownership remain available."}
			}
		}
	}
	return nil
}

// resolvedRuntimeProjectionAbsent reports that the application runtime
// projection was never materialized (the distinct ErrRuntimeNotApplied state,
// not a damaged projection). It is the deployment-side half of the
// never-realized proof in checkRetainedApplicationProviderOwnership.
func resolvedRuntimeProjectionAbsent(resolved resolvedApplication) bool {
	_, err := application.ExistingRuntimeFiles(resolved.Store, resolved.Manifest)
	return errors.Is(err, application.ErrRuntimeNotApplied)
}

// retainedProviderOwnershipVerifiedAbsent decides whether one retained,
// unbound application owner is a VERIFIED never-realized intention. A
// provider whose realization would be detectable through the native renderer
// is never absent here: detection failure is ambiguous and the caller keeps
// the fail-closed refusal, so an owner that ever ran can never be reclaimed
// by this seam.
func retainedProviderOwnershipVerifiedAbsent(ctx context.Context, resolved resolvedApplication, instance capability.ProviderInstance) (bool, error) {
	switch {
	case instance.Provider.Kind == capability.ProviderRuntimeBroker:
		// The Target-wide secure broker runtime lives inside the application
		// runtime projection; its absence was proven above.
		return true, nil
	case applicationNativeBackendInstance(instance, resolved.Manifest):
		resources, err := application.NativeBackendResources(resolved.Manifest, application.RuntimeFilesFor(resolved.Store, resolved.Manifest))
		if err != nil {
			return false, err
		}
		for _, resource := range resources {
			if resource.Kind == "container" && strings.Contains(resource.Name, string(instance.Provider.Kind)) {
				return false, nil
			}
		}
		return true, nil
	default:
		provider, err := detectRuntimeForApplication(ctx, resolved, bhruntime.CapabilityResourceOwnership)
		if err != nil {
			return false, err
		}
		registry, err := (capability.RegistryStore{Path: filepath.Join(resolved.TargetStateRoot, "provider-registry.json")}).Load()
		if err != nil {
			return false, err
		}
		inventory, err := existingStandaloneProviderInventory(resolved, provider, instance, registry)
		if err != nil {
			return false, err
		}
		found, err := provider.InspectProjectResources(ctx, inventory.project, inventory.resources)
		if err != nil {
			return false, err
		}
		if len(found) != 0 {
			return false, nil
		}
		return true, nil
	}
}

// reclaimUnboundRetainedProviderOwner removes one verified never-realized,
// unbound, BaseHarbor-owned application provider instance under the registry
// lock. The live entry is re-verified inside the mutation: any drift (a
// binding appeared, ownership or scope changed) refuses instead of deleting.
func reclaimUnboundRetainedProviderOwner(resolved resolvedApplication, instanceID string) error {
	store := capability.RegistryStore{Path: filepath.Join(resolved.TargetStateRoot, "provider-registry.json")}
	return store.Update(func(current *capability.Registry) error {
		found := false
		for _, live := range current.Instances {
			if live.ID != instanceID {
				continue
			}
			found = true
			if live.Scope != capability.ScopeApplication || live.Ownership != capability.OwnershipBaseHarbor ||
				live.OwnerApplicationID != resolved.Manifest.ApplicationID || (live.OwnerEnvironment != "" && live.OwnerEnvironment != resolved.Manifest.Environment) ||
				len(current.BindingsForProviderInstance(live.ID)) != 0 {
				return &machine.Error{Code: machine.ErrorOwnershipAmbiguous, CauseCode: "retained_provider_ownership_unverified", Resource: live.ID, Message: "Retained provider ownership changed during verification.", Next: "Inspect the existing owner and retry; runtime and data ownership remain available."}
			}
			break
		}
		if !found {
			return nil
		}
		kept := make([]capability.ProviderInstance, 0, len(current.Instances))
		for _, live := range current.Instances {
			if live.ID == instanceID {
				continue
			}
			kept = append(kept, live)
		}
		current.Instances = kept
		return nil
	})
}

// Observe existing native owners even after their capability leaves desired
// intent. The common resolver remains the sole dependency/removal authority.
func observeApplicationProviderFacts(ctx context.Context, resolved resolvedApplication) map[string]application.ProviderFact {
	facts := observeIdentityProviderFacts(ctx, resolved)
	observeBrokerProviderFacts(ctx, resolved, facts)
	observeSharedBackendProviderFacts(ctx, resolved, facts)
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
