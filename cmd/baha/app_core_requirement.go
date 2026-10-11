package main

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/machine"
)

// Resolve only through the existing Target registry and native provider plan.
// Registration is projected in memory; only the normal apply owner persists it.
func resolveApplicationLifecycleFootprint(resolved resolvedApplication) (application.Footprint, error) {
	return resolveApplicationLifecycleFootprintWithFacts(resolved, nil)
}

func resolveApplicationLifecycleFootprintWithFacts(resolved resolvedApplication, observations map[string]application.ProviderFact) (application.Footprint, error) {
	registry, err := (capability.RegistryStore{Path: filepath.Join(resolved.TargetStateRoot, "provider-registry.json")}).Load()
	if err != nil {
		return application.Footprint{}, err
	}
	planned, err := application.PreviewReferenceProviderRegistryAt(resolved.TargetStateRoot, resolved.Manifest)
	if err != nil {
		return application.Footprint{}, err
	}
	for _, binding := range planned.Bindings {
		if binding.ApplicationID != resolved.Manifest.ApplicationID || binding.Environment != resolved.Manifest.Environment {
			continue
		}
		for _, instance := range planned.Instances {
			if instance.ID == binding.ProviderInstanceID && instance.Ownership == capability.OwnershipExternal && instance.Provider.Kind != capability.ProviderExternalOIDC && instance.Provider.Kind != capability.ProviderExternalOTLP {
				return application.Footprint{}, &machine.Error{Code: machine.ErrorUnsupported, CauseCode: "external_provider_adapter_unavailable", Resource: instance.ID, Message: "The selected BYO provider has no compatible application lifecycle adapter.", Next: "Use an existing supported binding; BaseHarbor will neither install Core nor replace this external provider."}
			}
		}
	}
	for _, old := range registry.Bindings {
		if old.ApplicationID != resolved.Manifest.ApplicationID || (old.Environment != "" && old.Environment != resolved.Manifest.Environment) {
			continue
		}
		for _, wanted := range planned.Bindings {
			if wanted.ApplicationID == old.ApplicationID && wanted.Environment == resolved.Manifest.Environment && wanted.Resource.Kind == old.Resource.Kind && wanted.Resource.Name == old.Resource.Name && wanted.ProviderInstanceID != old.ProviderInstanceID {
				return application.Footprint{}, &machine.Error{Code: machine.ErrorUnsupported, CauseCode: "provider_binding_migration_required", Message: "The existing provider binding differs from the current native placement plan.", Next: "Restore the existing placement; provider migration requires an explicit supported operation."}
			}
		}
	}
	existing := map[string]bool{}
	for _, instance := range registry.Instances {
		existing[instance.ID] = true
	}
	contract, err := application.PortableContractFromManifest(resolved.Manifest)
	if err != nil {
		return application.Footprint{}, err
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
	planned.Bindings = actualBindings
	snapshot := application.ResolutionSnapshot{Target: resolved.Target.Name, Registry: planned, Facts: map[string]application.ProviderFact{}}
	for _, instance := range planned.Instances {
		// Native managed providers use the existing Core PKI lifecycle. This
		// preserves the full management topology rather than inventing slim SQL.
		fact := application.ProviderFact{Target: resolved.Target.Name, Core: application.CoreRequired, State: application.ProviderNewRequired}
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
		snapshot.Facts[instance.ID] = fact
	}
	return application.ResolveFootprint(resolved.Manifest, snapshot, preferences)
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
	retained, err := removedApplicationProviderBindings(resolved)
	if err != nil {
		return err
	}
	for _, binding := range retained {
		fmt.Fprintf(out, "Unused application resource retained: %s/%s on %s; no provider stop or reclamation is performed.\n", binding.Resource.Kind, binding.Resource.Name, binding.ProviderInstanceID)
	}
	for _, instanceID := range footprint.Unused {
		fmt.Fprintf(out, "Unused Target provider retained: %s; reclamation requires an explicit ownership-checked operation.\n", instanceID)
	}
	if footprint.Core == application.CoreNotRequired && !requiresManagedServiceIssuer(resolved.Manifest) && !application.RequiresRuntimeBroker(resolved.Manifest) {
		return nil
	}
	fmt.Fprintln(out, "Selected managed providers require the existing SQL/Secrets/Identity Core topology; selective provider installation is unavailable.")
	fmt.Fprintln(out, "Incremental memory and container counts are unknown until live runtime evidence is available.")
	if noInput(ctx) || (!readerIsTerminal(in) && !isBufferedCoreInput(in)) {
		ctx = context.WithValue(ctx, assumeYesKey{}, false)
	}
	return applicationCorePrerequisite(withTargetOverride(ctx, resolved.Target.Name), in, out)
}

// All read surfaces share the same desired plan and Target resolver snapshot.
type applicationLifecyclePlan struct {
	application.Plan
	Footprint         application.Footprint        `json:"footprint"`
	RetainedResources []capability.ProviderBinding `json:"retained_resources,omitempty"`
}

func buildResolvedApplicationPlan(ctx context.Context, resolved resolvedApplication) (applicationLifecyclePlan, error) {
	plan, err := application.BuildPlan(resolved.Manifest)
	if err != nil {
		return applicationLifecyclePlan{}, err
	}
	footprint, err := resolveApplicationLifecycleFootprintWithFacts(resolved, observeApplicationProviderFacts(ctx, resolved))
	if err != nil {
		return applicationLifecyclePlan{}, err
	}
	retained, err := removedApplicationProviderBindings(resolved)
	if err != nil {
		return applicationLifecyclePlan{}, err
	}
	return applicationLifecyclePlan{Plan: plan, Footprint: footprint, RetainedResources: retained}, nil
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

func checkRetainedApplicationProviderOwnership(resolved resolvedApplication) error {
	registry, err := (capability.RegistryStore{Path: filepath.Join(resolved.TargetStateRoot, "provider-registry.json")}).Load()
	if err != nil {
		return err
	}
	for _, instance := range registry.Instances {
		if instance.Scope != capability.ScopeApplication || instance.OwnerApplicationID != resolved.Manifest.ApplicationID || (instance.OwnerEnvironment != "" && instance.OwnerEnvironment != resolved.Manifest.Environment) {
			continue
		}
		if len(registry.BindingsForProviderInstance(instance.ID)) == 0 {
			return &machine.Error{Code: machine.ErrorUnsupported, CauseCode: "provider_reclamation_required", Resource: instance.ID, Message: "An unused application-owned provider still has retained ownership and may contain data.", Next: "Restore its original capability and placement before explicit destroy; separate ownership-checked reclamation is deferred by #872."}
		}
	}
	return nil
}

// Measure only app-scoped backend containers whose ownership was checked by
// the existing runtime owner. Shared/remote/BYO and unknown dependencies stay
// explicitly unverifiable; no estimate is inferred from registry metadata.
func observeApplicationProviderFacts(ctx context.Context, resolved resolvedApplication) map[string]application.ProviderFact {
	facts := map[string]application.ProviderFact{}
	if isRemoteApplication(resolved) || !application.HasApplicationScopedRuntimeServices(resolved.Manifest) {
		return facts
	}
	files, err := application.ExistingRuntimeFiles(resolved.Store, resolved.Manifest)
	if err != nil {
		return facts
	}
	runtime, err := detectRuntimeForTarget(ctx, resolved.Target)
	if err != nil {
		return facts
	}
	owned, err := application.InspectOwnedRuntimeResourcesForFiles(ctx, runtime, resolved.Manifest, files)
	if err != nil || len(owned) == 0 {
		return facts
	}
	containers, err := runtime.ListRuntimeContainers(ctx)
	if err != nil {
		return facts
	}
	registry, err := (capability.RegistryStore{Path: filepath.Join(resolved.TargetStateRoot, "provider-registry.json")}).Load()
	if err != nil {
		return facts
	}
	for _, instance := range registry.Instances {
		if instance.Scope != capability.ScopeApplication || instance.OwnerApplicationID != resolved.Manifest.ApplicationID || instance.OwnerEnvironment != resolved.Manifest.Environment {
			continue
		}
		prefix := ""
		switch instance.Provider.Kind {
		case capability.ProviderPostgreSQL:
			prefix = "postgres-"
		case capability.ProviderValkey:
			prefix = "valkey-"
		case capability.ProviderRabbitMQ:
			prefix = "rabbitmq-"
		case capability.ProviderMongoDB:
			prefix = "mongodb-"
		default:
			continue
		}
		// Multiple logical resources have distinct instances. Without the existing
		// provider-specific inventory mapping, a project count cannot be attributed.
		peers := 0
		for _, peer := range registry.Instances {
			if peer.Scope == instance.Scope && peer.OwnerApplicationID == instance.OwnerApplicationID && peer.OwnerEnvironment == instance.OwnerEnvironment && peer.Provider.Kind == instance.Provider.Kind {
				peers++
			}
		}
		if peers != 1 {
			continue
		}
		count := int64(0)
		running := false
		for _, container := range containers {
			if container.Project == files.Project && (strings.HasPrefix(container.Service, prefix) || container.Service == strings.TrimSuffix(prefix, "-")) {
				count++
				running = running || container.Running
			}
		}
		state := application.ProviderStoppedOwned
		if running {
			state = application.ProviderRunning
		}
		facts[instance.ID] = application.ProviderFact{Target: resolved.Target.Name, State: state, Core: application.CoreRequired, MemoryBytes: application.Quantity{Classification: "unknown"}, Containers: application.Quantity{Value: &count, Classification: "measured", Source: "ownership-checked runtime container inventory for " + files.Project}}
	}
	return facts
}
