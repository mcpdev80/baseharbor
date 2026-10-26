package main

import (
	"context"
	"fmt"
	"io"
	"path/filepath"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/identityprovider"
	"github.com/mcpdev80/baseharbor/internal/machine"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

func prepareApplicationProviderRetirement(ctx context.Context, resolved resolvedApplication) (application.ProviderPlan, error) {
	observed, diagnostics := observeApplicationProviderFactsWithDiagnostics(ctx, resolved)
	plan, err := resolveApplicationProviderPlan(resolved, observed)
	if err != nil {
		return application.ProviderPlan{}, err
	}
	released := map[string]bool{}
	for _, binding := range plan.ReleasedBindings {
		released[binding.ProviderInstanceID] = true
	}
	registry, err := (capability.RegistryStore{Path: filepath.Join(resolved.TargetStateRoot, "provider-registry.json")}).Load()
	if err != nil {
		return application.ProviderPlan{}, err
	}
	for _, change := range plan.Changes {
		controlPlaneStorage := change.InstanceID == application.RuntimeStorageDependencyID(resolved.Target.Name)
		if (!released[change.InstanceID] && !controlPlaneStorage && !ownedControlPlaneSecrets(registry, change.InstanceID)) || len(change.Consumers) > 0 || change.Ownership == capability.OwnershipExternal {
			continue
		}
		if !plan.Complete || change.Action == "preserve" {
			return application.ProviderPlan{}, providerRetirementUnverifiable(plan, observed, diagnostics, change.InstanceID)
		}
		if change.Action == "stop" || change.Action == "remove-runtime" {
			fact, found := observed[change.InstanceID]
			// A selected last-consumer stop/remove-runtime is authorized only
			// by an actual Target-scoped runtime measurement. The registry-only
			// preview projection remains usable on preview surfaces and the
			// common resolver stays the sole dependency/removal authority, but
			// it can never qualify this preparation gate (CMF21 R2).
			if !found || fact.Target != resolved.Target.Name || !providerFactIsRuntimeMeasurement(fact) {
				return application.ProviderPlan{}, providerRetirementUnverifiable(plan, observed, diagnostics, change.InstanceID)
			}
		}
		for _, instance := range registry.Instances {
			sharedBackend := instance.Scope == capability.ScopeShared && (instance.Provider.Kind == capability.ProviderPostgreSQL || instance.Provider.Kind == capability.ProviderValkey)
			nativeBackend := applicationNativeBackendInstance(instance, resolved.Manifest)
			if instance.ID == change.InstanceID && instance.Provider.Kind != capability.ProviderKeycloak && !ownedControlPlaneSecrets(registry, instance.ID) && !sharedBackend && !nativeBackend && !standaloneProviderKind(instance.Provider.Kind) && !applicationExposureInstance(instance, resolved.Manifest) {
				return application.ProviderPlan{}, &machine.Error{Code: machine.ErrorUnsupported, CauseCode: "provider_retirement_adapter_unavailable", Resource: instance.ID, Message: "This provider has no verified data-preserving retirement adapter.", Next: "Keep the existing capability until its runtime retirement adapter is available."}
			}
		}
	}
	return plan, nil
}

// providerRetirementUnverifiable is the existing fail-closed preparation
// error. Diagnostic-only (preview) facts keep their typed materialization
// code/cause plus instance/Target context in the serialized Details alongside
// the plan/observation evidence, because machine.Error.Cause is json:"-" and a
// Go-only error chain never reaches CLI/JSON consumers.
func providerRetirementUnverifiable(plan application.ProviderPlan, observed map[string]application.ProviderFact, diagnostics error, instanceID string) error {
	details := map[string]any{"plan": plan, "observed": observed}
	if view, ok := materializationDiagnosticView(diagnostics).(map[string]any); ok {
		details["materialization_diagnostics"] = view["materialization_diagnostics"]
	}
	return &machine.Error{Code: machine.ErrorOwnershipAmbiguous, CauseCode: "provider_retirement_unverifiable", Resource: instanceID, Message: "The last consumer cannot be released without verified runtime and complete consumer inventory.", Next: "Recover the existing provider/deployment inventory and retry; ownership and runtime remain available.", Details: details, Cause: diagnostics}
}

func retireReleasedApplicationProviders(ctx context.Context, resolved resolvedApplication, provider bhruntime.RuntimeProvider, issuer serviceaccess.Issuer, plan application.ProviderPlan, out io.Writer) error {
	if plan.Version != application.ProviderPlanVersion || plan.Target != resolved.Target.Name {
		return invalidProviderConsumerInventory(fmt.Errorf("retirement plan differs from its Target"))
	}
	if err := retireUnneededApplicationBroker(ctx, resolved, provider, plan, out); err != nil {
		return err
	}
	if err := retireApplicationNativeBackendServices(ctx, resolved, provider, plan, out); err != nil {
		return err
	}
	registry, err := (capability.RegistryStore{Path: filepath.Join(resolved.TargetStateRoot, "provider-registry.json")}).Load()
	if err != nil {
		return err
	}
	plannedRegistry, err := application.PreviewReferenceProviderRegistryAt(resolved.TargetStateRoot, resolved.Manifest)
	if err != nil {
		return err
	}
	released := map[string]bool{}
	for _, binding := range plan.ReleasedBindings {
		released[binding.ProviderInstanceID] = true
	}
	standalone := map[string]bool{}
	for _, instance := range registry.Instances {
		standalone[instance.ID] = standaloneProviderKind(instance.Provider.Kind)
	}
	for _, change := range plan.Changes {
		// The complete acknowledged union also owns older unbound modules.
		// A new reference release is not necessary to converge their selected
		// runtime retirement; canonical inspection still proves authority.
		if (!released[change.InstanceID] && !standalone[change.InstanceID]) || (change.Action != "stop" && change.Action != "remove-runtime") {
			continue
		}
		if !plan.Complete || len(change.Consumers) > 0 || change.Ownership != capability.OwnershipBaseHarbor {
			return invalidProviderConsumerInventory(fmt.Errorf("provider retirement plan changed for %s", change.InstanceID))
		}
		for _, instance := range registry.Instances {
			if instance.ID != change.InstanceID {
				continue
			}
			if applicationNativeBackendInstance(instance, resolved.Manifest) {
				continue // The existing consolidated owner was reconciled above.
			}
			if instance.Scope == capability.ScopeShared && (instance.Provider.Kind == capability.ProviderPostgreSQL || instance.Provider.Kind == capability.ProviderValkey) {
				result, err := application.ConvergeExistingSharedBackendRuntime(ctx, provider, resolved.TargetStateRoot, resolved.Target.Name, plan, plannedRegistry)
				if err != nil {
					return err
				}
				for _, item := range result.Retained {
					fmt.Fprintf(out, "Retained %s %s: %s\n", item.Resource.Kind, item.Resource.Name, item.Reason)
				}
				continue
			}
			if ownedControlPlaneSecrets(registry, instance.ID) {
				continue
			}
			if applicationExposureInstance(instance, resolved.Manifest) {
				if err := retireApplicationExposureRuntime(ctx, resolved, provider, instance, plan, out); err != nil {
					return err
				}
				continue
			}
			if standaloneProviderKind(instance.Provider.Kind) {
				for _, binding := range registry.BindingsForProviderInstance(instance.ID) {
					selected := false
					for _, release := range plan.ReleasedBindings {
						selected = selected || release == binding
					}
					if !selected {
						return invalidProviderConsumerInventory(fmt.Errorf("standalone runtime still has an unreleased consumer"))
					}
				}
				for _, required := range plan.Required {
					if required == instance.ID {
						return invalidProviderConsumerInventory(fmt.Errorf("standalone runtime remains required"))
					}
				}
				if err := retireStandaloneApplicationProvider(ctx, resolved, provider, instance, registry, out); err != nil {
					return err
				}
				continue
			}
			if instance.Provider.Kind != capability.ProviderKeycloak {
				return fmt.Errorf("unsupported provider retirement %s", instance.ID)
			}
			files, _, err := identityprovider.ExistingKeycloakRuntimeInventoryAt(resolved.Manifest, providerInstancePlacement(instance), resolved.TargetStateRoot, resolved.Target.Name)
			if err != nil {
				return err
			}
			if err := retireIdentityGatewayRoutes(ctx, provider, resolved, files); err != nil {
				return fmt.Errorf("retire Identity gateway routes: %w", err)
			}
			result, err := identityprovider.RetireKeycloakRuntime(ctx, provider, resolved.Manifest, providerInstancePlacement(instance), resolved.TargetStateRoot, resolved.Target.Name)
			// A failed native operation can still have removed some containers.
			// Report its inspected partial result before retaining the owner and
			// returning the failure; never turn a command error into success.
			for _, resource := range result.Removed {
				fmt.Fprintf(out, "Removed %s %s after owned runtime inspection\n", resource.Kind, resource.Name)
			}
			for _, item := range result.Retained {
				fmt.Fprintf(out, "Retained %s %s: %s\n", item.Resource.Kind, item.Resource.Name, item.Reason)
			}
			fmt.Fprintf(out, "Retained Identity files at %s; runtime retirement does not verify continued login, token behavior or recovery usability.\n", files.Dir)
			if err != nil {
				return err
			}
		}
	}
	root, err := targetRuntimeStateRoot(resolved.Target)
	if err != nil {
		return err
	}
	result, err := application.RetireUnneededControlPlaneServices(ctx, provider, root, resolved.Target.Name, plan, registry)
	for _, resource := range result.Removed {
		fmt.Fprintf(out, "Removed %s %s after owned runtime inspection\n", resource.Kind, resource.Name)
	}
	for _, item := range result.Retained {
		fmt.Fprintf(out, "Retained %s %s: %s\n", item.Resource.Kind, item.Resource.Name, item.Reason)
	}
	return err
}

func ownedControlPlaneSecrets(registry capability.Registry, id string) bool {
	for _, instance := range registry.Instances {
		if instance.ID == id && instance.Provider.Kind == capability.ProviderOpenBao && instance.Scope == capability.ScopeShared && instance.Ownership == capability.OwnershipBaseHarbor {
			return true
		}
	}
	return false
}
