package application

import (
	"github.com/mcpdev80/baseharbor/internal/capability"
	"testing"
)

// Every declared function/material traverses the same domain authority for
// creation, reactivation, change and removal. This is declaration qualification;
// existing adapters must separately provide actual consumer/native receipts.
func TestProviderChangeWholeCatalogLifecycleConsequences(t *testing.T) {
	descriptors := append(capability.ReferenceIntegrations(), capability.IntegrationDescriptor{Provider: capability.Provider{Kind: capability.ProviderRuntimeBroker}, SupportedScopes: []capability.ProviderScope{capability.ScopeApplication}}, capability.IntegrationDescriptor{Provider: capability.Provider{Kind: capability.ProviderRuntimeExecutor}, SupportedScopes: []capability.ProviderScope{capability.ScopeApplication}})
	for _, descriptor := range descriptors {
		t.Run(string(descriptor.Provider.Kind), func(t *testing.T) {
			scope := descriptor.SupportedScopes[0]
			owned := capability.OwnershipBaseHarbor
			if scope == capability.ScopeExternal {
				owned = capability.OwnershipExternal
			}
			instance := capability.ProviderInstance{ID: "source", Provider: descriptor.Provider, Scope: scope, Ownership: owned}
			if scope == capability.ScopeApplication {
				instance.OwnerApplicationID = contractAppID
				instance.OwnerApplication = "app"
				instance.OwnerEnvironment = "dev"
			}
			if scope == capability.ScopeExternal {
				instance.Reference = "existing-registration"
			}
			ancillary := descriptor.Provider.Kind == capability.ProviderRuntimeBroker || descriptor.Provider.Kind == capability.ProviderRuntimeExecutor
			registry := capability.NewRegistry()
			if !ancillary {
				if err := registry.Register(instance); err != nil {
					t.Fatal(err)
				}
			}
			replacement := instance
			replacement.ID = "replacement"
			if scope == capability.ScopeShared {
				replacement.SharingBoundary = "replacement"
			}
			if !ancillary {
				if err := registry.Register(replacement); err != nil {
					t.Fatal(err)
				}
			}
			snapshot := ResolutionSnapshot{Target: "local", Registry: registry, Facts: map[string]ProviderFact{}}
			observation := ProviderImpactObservation{Revision: "verified/1", InventoryComplete: true, Providers: map[string]ProviderImpactFact{}}
			contract, _ := descriptor.ChangeContract()
			instances := registry.Instances
			if ancillary {
				snapshot.Ancillary = []capability.ProviderInstance{instance, replacement}
				instances = snapshot.Ancillary
			}
			for _, i := range instances {
				snapshot.Facts[i.ID] = ProviderFact{Target: "local", State: ProviderRunning, Core: CoreNotRequired, Dependencies: []string{}}
				observation.Providers[i.ID] = impactProvider(i, "local", contract.MaterialKinds[0])
				if ancillary {
					f := snapshot.Facts[i.ID]
					f.Ancillary = true
					f.OwnerReference = i.OwnerApplicationID
					snapshot.Facts[i.ID] = f
					observed := observation.Providers[i.ID]
					observed.Realization.Ancillary = true
					observation.Providers[i.ID] = observed
				}
			}
			fact := observation.Providers[instance.ID]
			fact.Materials = nil
			for _, kind := range contract.MaterialKinds {
				m := impactProvider(instance, "local", kind).Materials[0]
				m.ID = kind
				fact.Materials = append(fact.Materials, m)
			}
			observation.Providers[instance.ID] = fact
			for _, function := range contract.Functions {
				observation.Functions = append(observation.Functions, ProviderFunctionUse{Consumer: "app", InstanceID: instance.ID, Function: function, Required: true, Evidence: "actual consumer configuration"})
			}
			union, err := ResolveProviderPlan(snapshot, nil, true)
			if err != nil {
				t.Fatal(err)
			}
			for _, operation := range []string{"add", "resume", "update", "stop", "destroy", "remove-runtime", "disable", "unbind", "rebind", "replace"} {
				t.Run(operation, func(t *testing.T) {
					request := ProviderChangeRequest{InstanceID: instance.ID, Operation: operation}
					if operation == "replace" {
						request.ReplacementID = replacement.ID
					}
					plan, err := DescribeProviderChanges(snapshot, union, []ProviderChangeRequest{request}, observation)
					foreignMutation := owned == capability.OwnershipExternal && operation != "unbind" && operation != "disable" && operation != "replace" && operation != "rebind"
					if foreignMutation {
						if err == nil {
							t.Fatal("foreign mutation authorized")
						}
						return
					}
					if err != nil {
						t.Fatal(err)
					}
					impact := plan.Impacts[0]
					if len(impact.Functions) != len(contract.Functions) || len(impact.Materials) != len(contract.MaterialKinds) {
						t.Fatal("catalog functions or inventory omitted")
					}
					for _, f := range impact.Functions {
						if interruptsProvider(operation) && f.Outcome != "unavailable" {
							t.Fatal("continuation invented without adapter verification")
						}
						if !interruptsProvider(operation) && f.Outcome != "changed" {
							t.Fatal("addition/change hidden")
						}
					}
					for _, m := range impact.Materials {
						if m.Before.ID == "" || m.Before.Location == nil {
							t.Fatal("actual material identity/location lost")
						}
					}
					if interruptsProvider(operation) && plan.ConfirmationsRequired != 2 {
						t.Fatal("affected access loss without two acknowledgements")
					}
					if err := ValidateProviderChangeConsent(plan, ProviderChangeConsent{PlanID: plan.PlanID, First: true, Second: true, DeleteData: true}); err != nil {
						t.Fatal("explicit developer choice vetoed")
					}
					observationMissing := observation
					observationMissing.Providers = map[string]ProviderImpactFact{}
					unknown, err := DescribeProviderChanges(snapshot, union, []ProviderChangeRequest{request}, observationMissing)
					if err != nil || unknown.Complete || unknown.ConfirmationsRequired != 2 {
						t.Fatal("missing actual inventory declared safe")
					}
				})
			}
		})
	}
}

func TestProviderChangeMeaningfulTransientMaterialSurvival(t *testing.T) {
	s, u, o := impactFixture(t)
	for _, kind := range []string{"cache", "session", "job", "queue", "message", "in-flight", "telemetry-buffer", "lease", "token"} {
		t.Run(kind, func(t *testing.T) {
			original := o.Providers["secrets"]
			f := original
			f.Materials = append([]ProviderMaterial(nil), original.Materials...)
			f.Materials[0].Kind = kind
			o.Providers["secrets"] = f
			request := []ProviderChangeRequest{{InstanceID: "secrets", Operation: "stop"}}
			unknown, err := DescribeProviderChanges(s, u, request, o)
			if err != nil || unknown.Impacts[0].Materials[0].After.Disposition != "unknown" {
				t.Fatal("volume implied transient survival")
			}
			f.Materials[0].Retention = &MaterialRetention{SurvivesStop: true, SurvivesRuntimeRemoval: false, Verified: true, Evidence: "verified durable adapter retention"}
			o.Providers["secrets"] = f
			retained, err := DescribeProviderChanges(s, u, request, o)
			if err != nil || retained.Impacts[0].Materials[0].After.Disposition != "retained" {
				t.Fatal("verified stop preservation lost")
			}
			request[0].Operation = "remove-runtime"
			lost, err := DescribeProviderChanges(s, u, request, o)
			if err != nil || !lost.Destructive || lost.Impacts[0].Materials[0].After.Disposition != "nonrecoverable" {
				t.Fatal("known transient loss hidden")
			}
			if err := ValidateProviderChangeConsent(lost, ProviderChangeConsent{PlanID: lost.PlanID, First: true, Second: true}); err == nil {
				t.Fatal("irrecoverable material loss without distinct destructive consent")
			}
			o.Providers["secrets"] = original
		})
	}
}
