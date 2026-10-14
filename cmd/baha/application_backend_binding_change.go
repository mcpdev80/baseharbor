package main

import (
	"encoding/json"
	"path/filepath"
	"sort"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
)

func supportedBackendBindingReplacement(old, wanted capability.ProviderInstance) bool {
	if old.Ownership == capability.OwnershipExternal && wanted.Ownership == capability.OwnershipExternal && old.Scope == capability.ScopeExternal && wanted.Scope == capability.ScopeExternal && old.Provider.Kind == capability.ProviderExternalOIDC && wanted.Provider.Kind == capability.ProviderExternalOIDC {
		// The registered OIDC driver resolves explicit destination credential/
		// trust references and revalidates them before verified delivery. Only
		// bindings change; neither foreign realization is retired or destroyed.
		return true
	}
	if old.Ownership != capability.OwnershipBaseHarbor || wanted.Ownership != capability.OwnershipBaseHarbor || old.Provider.Kind != wanted.Provider.Kind || old.Scope == wanted.Scope {
		return false
	}
	nativeScope := func(scope capability.ProviderScope) bool {
		return scope == capability.ScopeShared || scope == capability.ScopeApplication
	}
	return nativeScope(old.Scope) && nativeScope(wanted.Scope) && (old.Provider.Kind == capability.ProviderPostgreSQL || old.Provider.Kind == capability.ProviderValkey)
}

func observeBackendBindingReplacementImpact(resolved resolvedApplication, snapshot application.ResolutionSnapshot, observation *application.ProviderImpactObservation) ([]byte, error) {
	actual, err := (capability.RegistryStore{Path: filepath.Join(resolved.TargetStateRoot, "provider-registry.json")}).Load()
	if err != nil {
		return nil, err
	}
	planned, err := application.PreviewReferenceProviderRegistryAt(resolved.TargetStateRoot, resolved.Manifest)
	if err != nil {
		return nil, err
	}
	for _, old := range actual.Bindings {
		if old.ApplicationID != resolved.Manifest.ApplicationID || (old.Environment != "" && old.Environment != resolved.Manifest.Environment) {
			continue
		}
		for _, wanted := range planned.Bindings {
			if wanted.ApplicationID != old.ApplicationID || wanted.Environment != resolved.Manifest.Environment || wanted.Resource.Kind != old.Resource.Kind || wanted.Resource.Name != old.Resource.Name || wanted.ProviderInstanceID == old.ProviderInstanceID {
				continue
			}
			observation.Functions = append(observation.Functions, application.ProviderFunctionUse{Consumer: "application/" + old.ApplicationID + "/" + resolved.Manifest.Environment, InstanceID: old.ProviderInstanceID, Function: string(old.Resource.Kind), Required: true, Evidence: "currently applied canonical provider binding"})
			fact := observation.Providers[old.ProviderInstanceID]
			fact.Effects = append(fact.Effects, application.OperationalConsequence{Aspect: "provider-selection", Before: old.ProviderInstanceID, After: wanted.ProviderInstanceID, Evidence: "canonical applied and selected binding projection for " + string(old.Resource.Kind) + "/" + old.Resource.Name})
			if old.Resource.Provider == capability.ProviderPostgreSQL || old.Resource.Provider == capability.ProviderValkey {
				fact.Effects = append(fact.Effects, application.OperationalConsequence{Aspect: "data-continuity", Before: "Application uses the original provider's data namespace.", After: "Destination uses its own retained or newly initialized data. Original data and protected credentials remain owned; no automatic content migration or continued access to original data is claimed. Destination access requires actual apply verification.", Evidence: "existing native destination provisioning and data-preserving retirement adapters"})
			} else {
				fact.Effects = append(fact.Effects, application.OperationalConsequence{Aspect: "consumer-rebinding", Before: "The application uses the displayed original provider binding.", After: "The selected destination may change endpoint, authentication, trust, service delivery and recovery access. No credential transfer, data migration, session or consumer continuity is verified by this plan. Actual execution requires an implemented adapter and verified delivery; unavailable execution preserves the current binding and owner.", Evidence: "canonical source/destination binding observation; operational handoff and consumer results remain unverified"})
			}
			if old.Resource.Provider == capability.ProviderValkey {
				fact.Effects = append(fact.Effects, application.OperationalConsequence{Aspect: "authentication-principal", Before: "Shared storage uses its scoped ACL user; application-scoped storage uses the native default principal.", After: "Destination uses its existing native principal. Password values and the original protected shared ACL reference are retained; switching back reprojects the shared identity. Native default access is limited to the separately owned application realization, not the shared provider.", Evidence: "existing native requirepass adapter and protected service-binding projection; actual destination authentication requires apply verification"})
			}
			observation.Providers[old.ProviderInstanceID] = fact
		}
	}
	records, err := deployment.ListDeployments(resolved.Target.Name)
	if err != nil {
		return nil, invalidProviderConsumerInventory(err)
	}
	owners := map[string]string{}
	for _, record := range records {
		owners[record.Identity.DeploymentID] = runtimeMutationRevision(record)
	}
	return json.Marshal(struct {
		Registry capability.Registry
		Owners   map[string]string
	}{actual, owners})
}

func validateApplicationBindingReplacements(resolved resolvedApplication, actual, planned capability.Registry) error {
	instances := map[string]capability.ProviderInstance{}
	for _, instance := range planned.Instances {
		instances[instance.ID] = instance
	}
	for _, old := range actual.Bindings {
		if old.ApplicationID != resolved.Manifest.ApplicationID || (old.Environment != "" && old.Environment != resolved.Manifest.Environment) {
			continue
		}
		for _, wanted := range planned.Bindings {
			if wanted.ApplicationID == old.ApplicationID && wanted.Environment == resolved.Manifest.Environment && wanted.Resource.Kind == old.Resource.Kind && wanted.Resource.Name == old.Resource.Name && wanted.ProviderInstanceID != old.ProviderInstanceID {
				if !supportedBackendBindingReplacement(instances[old.ProviderInstanceID], instances[wanted.ProviderInstanceID]) {
					return &machine.Error{Code: machine.ErrorUnsupported, CauseCode: "provider_binding_migration_required", Resource: old.ProviderInstanceID, Message: "This provider binding replacement has no supported application lifecycle adapter.", Next: "Inspect the selected provider's technical replacement support; current runtime, bindings and data remain owned."}
				}
			}
		}
	}
	return nil
}

// Compare the original applied bindings with the EXISTING desired registration
// projection. ResolveProviderPlan remains the sole runtime/dependency policy.
func appendReleasedAppliedBindings(resolved resolvedApplication, plan *application.ProviderPlan) error {
	if resolved.TargetStateRoot == "" {
		return nil // A caller-supplied pure snapshot has no persistent binding owner.
	}
	actual, err := (capability.RegistryStore{Path: filepath.Join(resolved.TargetStateRoot, "provider-registry.json")}).Load()
	if err != nil {
		return err
	}
	planned, err := application.PreviewReferenceProviderRegistryAt(resolved.TargetStateRoot, resolved.Manifest)
	if err != nil {
		return err
	}
	for _, old := range actual.Bindings {
		if old.ApplicationID != resolved.Manifest.ApplicationID || (old.Environment != "" && old.Environment != resolved.Manifest.Environment) {
			continue
		}
		retained := false
		for _, binding := range planned.Bindings {
			retained = retained || binding == old
		}
		for _, binding := range plan.ReleasedBindings {
			retained = retained || binding == old
		}
		if !retained {
			plan.ReleasedBindings = append(plan.ReleasedBindings, old)
		}
	}
	return nil
}

func backendBindingChangeRequests(resolved resolvedApplication, union application.ProviderPlan, requests []application.ProviderChangeRequest) ([]application.ProviderChangeRequest, error) {
	planned, err := application.PreviewReferenceProviderRegistryAt(resolved.TargetStateRoot, resolved.Manifest)
	if err != nil {
		return nil, err
	}
	consumer := "application/" + resolved.Manifest.ApplicationID + "/" + resolved.Manifest.Environment
	destinations := map[string]map[string]bool{}
	for _, old := range union.ReleasedBindings {
		for _, wanted := range planned.Bindings {
			if wanted.ApplicationID == old.ApplicationID && wanted.Environment == resolved.Manifest.Environment && wanted.Resource.Kind == old.Resource.Kind && wanted.Resource.Name == old.Resource.Name && wanted.ProviderInstanceID != old.ProviderInstanceID {
				if destinations[old.ProviderInstanceID] == nil {
					destinations[old.ProviderInstanceID] = map[string]bool{}
				}
				destinations[old.ProviderInstanceID][wanted.ProviderInstanceID] = true
			}
		}
	}
	for _, old := range union.ReleasedBindings {
		for _, wanted := range planned.Bindings {
			if wanted.ApplicationID != old.ApplicationID || wanted.Environment != resolved.Manifest.Environment || wanted.Resource.Kind != old.Resource.Kind || wanted.Resource.Name != old.Resource.Name || wanted.ProviderInstanceID == old.ProviderInstanceID {
				continue
			}
			operation := "rebind"
			for _, change := range union.Changes {
				if change.InstanceID == old.ProviderInstanceID && (change.Action == "stop" || change.Action == "remove-runtime") {
					operation = "replace"
				}
			}
			request := application.ProviderChangeRequest{InstanceID: old.ProviderInstanceID, ReplacementID: wanted.ProviderInstanceID, Operation: operation, Selection: "explicit", Consumers: []string{consumer}, Materials: []application.MaterialDisposition{}}
			if len(destinations[old.ProviderInstanceID]) > 1 {
				// A physical shared source can split into multiple named native
				// destinations. Do not imply that one destination replaces all.
				request.ReplacementID, request.Operation = "", "unbind"
				for _, change := range union.Changes {
					if change.InstanceID == old.ProviderInstanceID && (change.Action == "stop" || change.Action == "remove-runtime") {
						request.Operation = change.Action
					}
				}
			}
			found := false
			for i := range requests {
				if requests[i].InstanceID == old.ProviderInstanceID {
					requests[i], found = request, true
				}
			}
			if !found {
				requests = append(requests, request)
			}
		}
	}
	// Foreign realizations have no native stop action in the union. Releasing
	// their selected application reference still has actual consumer effects.
	for _, old := range union.ReleasedBindings {
		if old.ApplicationID != resolved.Manifest.ApplicationID || old.Environment != "" && old.Environment != resolved.Manifest.Environment {
			continue
		}
		external := false
		for _, instance := range planned.Instances {
			external = external || instance.ID == old.ProviderInstanceID && instance.Ownership == capability.OwnershipExternal
		}
		if !external {
			continue
		}
		requested, already := false, false
		for _, wanted := range planned.Bindings {
			requested = requested || wanted.ApplicationID == old.ApplicationID && wanted.Environment == resolved.Manifest.Environment && wanted.Resource.Kind == old.Resource.Kind && wanted.Resource.Name == old.Resource.Name
		}
		for _, request := range requests {
			already = already || request.InstanceID == old.ProviderInstanceID
		}
		if !requested && !already {
			requests = append(requests, application.ProviderChangeRequest{InstanceID: old.ProviderInstanceID, Operation: "unbind", Selection: "explicit", Consumers: []string{consumer}, Materials: []application.MaterialDisposition{}})
		}
	}
	sort.Slice(requests, func(i, j int) bool { return requests[i].InstanceID < requests[j].InstanceID })
	return requests, nil
}
