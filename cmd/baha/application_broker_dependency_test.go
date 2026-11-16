package main

import (
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
)

func TestRemovingApplicationBrokerRequiresOwnImpactWhileSharedSecretsRemain(t *testing.T) {
	first := application.WithWorkloadComponents(application.New("first", "dev", false, false, true), "api")
	second := application.WithWorkloadComponents(application.New("second", "dev", false, false, true), "api")
	registry := capability.NewRegistry()
	if err := registry.Register(capability.ProviderInstance{ID: "openbao/control-plane", Provider: capability.OpenBao, Scope: capability.ScopeShared, Ownership: capability.OwnershipBaseHarbor}); err != nil {
		t.Fatal(err)
	}
	storage := application.RuntimeStorageDependency("isolated")
	broker1, broker2 := application.RuntimeBrokerDependency("isolated", first), application.RuntimeBrokerDependency("isolated", second)
	snapshot := application.ResolutionSnapshot{Target: "isolated", Registry: registry, Ancillary: []capability.ProviderInstance{storage, broker1, broker2}, Facts: map[string]application.ProviderFact{}}
	snapshot.Facts["openbao/control-plane"] = application.ProviderFact{Target: "isolated", State: application.ProviderRunning, Core: application.CoreRequired, Dependencies: []string{storage.ID}}
	snapshot.Facts[storage.ID] = application.ProviderFact{Target: "isolated", State: application.ProviderRunning, Core: application.CoreRequired, Ancillary: true, OwnerReference: "runtime-owner", Dependencies: []string{}}
	for _, broker := range []capability.ProviderInstance{broker1, broker2} {
		snapshot.Facts[broker.ID] = application.ProviderFact{Target: "isolated", State: application.ProviderRunning, Core: application.CoreRequired, Ancillary: true, OwnerReference: broker.OwnerApplicationID, Dependencies: []string{"openbao/control-plane"}}
	}
	first.Services.Secrets = false
	plan, err := application.ResolveProviderPlan(snapshot, []application.ConsumerIntent{{Key: "first", Active: true, Manifest: first, DependencyRoots: []string{}}, {Key: "second", Active: true, Manifest: second, DependencyRoots: []string{broker2.ID}, Preferences: []application.ProviderPreference{{Requirement: capability.Requirement{Kind: capability.Secrets, Name: "default"}, InstanceID: "openbao/control-plane"}}}}, true)
	if err != nil || !plan.Complete {
		t.Fatalf("consumer union: %+v %v", plan, err)
	}
	requests := applicationProviderChangeRequests(plan)
	if len(requests) != 1 || requests[0].InstanceID != broker1.ID || requests[0].Operation != "stop" {
		t.Fatalf("broker disappearance skipped impact or stopped shared secrets: %+v", requests)
	}
	impact, err := application.DescribeProviderChanges(snapshot, plan, requests, application.ProviderImpactObservation{InventoryComplete: false, Revision: "actual-owner-revision", Providers: map[string]application.ProviderImpactFact{}})
	if err != nil || impact.ConfirmationsRequired != 2 || impact.Attention != "Achtung" {
		t.Fatalf("unknown broker effects lack two acknowledgements: %+v %v", impact, err)
	}
	if err := application.ValidateProviderChangeConsent(impact, application.ProviderChangeConsent{PlanID: impact.PlanID, First: true}); err == nil {
		t.Fatal("one acknowledgement authorized unknown broker effects")
	}
	if err := application.ValidateProviderChangeConsent(impact, application.ProviderChangeConsent{PlanID: impact.PlanID, First: true, Second: true}); err != nil {
		t.Fatalf("informed developer was blocked: %v", err)
	}
}
