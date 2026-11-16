package application

import (
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

func TestProviderUnionRegisteredExternalReferenceDoesNotInventNativeOwnership(t *testing.T) {
	registry := capability.NewRegistry()
	for _, instance := range []capability.ProviderInstance{
		{ID: "collector/shared", Provider: capability.OTelCollector, Scope: capability.ScopeShared, Ownership: capability.OwnershipBaseHarbor},
		{ID: "tempo/shared", Provider: capability.Tempo, Scope: capability.ScopeShared, Ownership: capability.OwnershipBaseHarbor},
		{ID: "receiver/external", Provider: capability.ExternalOTLP, Scope: capability.ScopeExternal, Ownership: capability.OwnershipExternal, Reference: "native-receiver"},
	} {
		if err := registry.Register(instance); err != nil {
			t.Fatal(err)
		}
	}
	m := WithOTLPTelemetry(WithWorkloadComponents(New("consumer", "dev", false, false, false), "api"), "traces")
	snapshot := ResolutionSnapshot{Target: "local", Registry: registry, Facts: map[string]ProviderFact{
		"collector/shared":  {Target: "local", State: ProviderRunning, Core: CoreNotRequired, Dependencies: []string{"tempo/shared"}},
		"tempo/shared":      {Target: "local", State: ProviderRunning, Core: CoreNotRequired, Dependencies: []string{}},
		"receiver/external": {Target: "local", State: ProviderUnverifiable, Core: CoreNotRequired, Dependencies: []string{}},
	}}
	consumer := ConsumerIntent{Key: "app/consumer", Active: true, Manifest: m, DependencyRoots: []string{}, Preferences: []ProviderPreference{{Requirement: capability.Requirement{Kind: capability.TelemetryOTLP, Name: "default"}, InstanceID: "receiver/external"}}}
	plan, err := ResolveProviderPlan(snapshot, []ConsumerIntent{consumer}, true)
	if err != nil || !plan.Complete {
		t.Fatalf("registered external reference with known local dependency boundary blocked owned retirement: %+v %v", plan, err)
	}
	for _, change := range plan.Changes {
		if change.InstanceID == "receiver/external" {
			if change.Action != "preserve" || change.Ownership != capability.OwnershipExternal {
				t.Fatal("external runtime adopted or selected for mutation")
			}
		} else if change.Action != "stop" || !change.PreserveData {
			t.Fatalf("verified unused own provider not safely retired: %+v", change)
		}
	}
	fp := plan.Consumers[consumer.Key]
	for _, group := range [][]ProviderFootprint{fp.Existing, fp.Additional} {
		for _, provider := range group {
			if provider.InstanceID == "receiver/external" && (provider.State != ProviderUnverifiable || provider.Containers.Value != nil) {
				t.Fatal("external runtime readiness or native footprint fabricated")
			}
		}
	}
	for _, broken := range []string{"external-dependencies", "missing-external-fact", "owned-state", "consumer-inventory"} {
		t.Run(broken, func(t *testing.T) {
			copy := snapshot
			copy.Facts = map[string]ProviderFact{}
			for id, fact := range snapshot.Facts {
				copy.Facts[id] = fact
			}
			complete := true
			switch broken {
			case "external-dependencies":
				f := copy.Facts["receiver/external"]
				f.Dependencies = nil
				copy.Facts["receiver/external"] = f
			case "missing-external-fact":
				delete(copy.Facts, "receiver/external")
			case "owned-state":
				f := copy.Facts["collector/shared"]
				f.State = ProviderUnverifiable
				copy.Facts["collector/shared"] = f
			case "consumer-inventory":
				complete = false
			}
			p, err := ResolveProviderPlan(copy, []ConsumerIntent{consumer}, complete)
			if err != nil || p.Complete {
				t.Fatalf("unknown authority/dependencies accepted: %+v %v", p, err)
			}
			for _, change := range p.Changes {
				if change.Action == "stop" || change.Action == "remove-runtime" {
					t.Fatal("incomplete owned inventory permits retirement")
				}
			}
		})
	}
}
