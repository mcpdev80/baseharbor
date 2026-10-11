package application

import (
	"encoding/json"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"reflect"
	"testing"
)

func TestProviderUnionAddRemoveReaddAndManagement(t *testing.T) {
	registry := capability.NewRegistry()
	sql := capability.ProviderInstance{ID: "sql/shared", Provider: capability.Provider{Kind: capability.ProviderPostgreSQL, Capabilities: []capability.Kind{capability.SQL}}, Scope: capability.ScopeShared, Ownership: capability.OwnershipBaseHarbor}
	if err := registry.Register(sql); err != nil {
		t.Fatal(err)
	}
	snapshot := ResolutionSnapshot{Target: "local", Registry: registry, Facts: map[string]ProviderFact{sql.ID: {Target: "local", State: ProviderRunning, Core: CoreNotRequired, Dependencies: []string{}}}}
	m := New("one", "dev", true, false, false)
	other := New("two", "dev", true, false, false)
	consumers := []ConsumerIntent{{Key: "app/one", Active: true, Manifest: m}, {Key: "management", Active: true, Manifest: other}}
	before, _ := json.Marshal(snapshot)
	plan, err := ResolveProviderPlan(snapshot, consumers, true)
	if err != nil {
		t.Fatal(err)
	}
	// An automatic compatible selection is complete even before binding persists.
	if !plan.Complete || plan.Changes[0].Action != "reuse" || len(plan.Changes[0].Consumers) != 2 {
		t.Fatalf("%+v", plan)
	}
	consumers[0].Active = false
	plan, err = ResolveProviderPlan(snapshot, consumers, true)
	if err != nil || plan.Changes[0].Action != "reuse" {
		t.Fatalf("%+v %v", plan, err)
	}
	consumers[1].Active = false
	plan, err = ResolveProviderPlan(snapshot, consumers, true)
	if err != nil || plan.Changes[0].Action != "stop" || !plan.Changes[0].PreserveData {
		t.Fatalf("%+v %v", plan, err)
	}
	fact := snapshot.Facts[sql.ID]
	fact.State = ProviderStoppedOwned
	snapshot.Facts[sql.ID] = fact
	consumers[0].Active = true
	plan, err = ResolveProviderPlan(snapshot, consumers, true)
	if err != nil || plan.Changes[0].Action != "restart" {
		t.Fatalf("%+v %v", plan, err)
	}
	fact.State = ProviderRunning
	snapshot.Facts[sql.ID] = fact
	after, _ := json.Marshal(snapshot)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("snapshot mutated")
	}
}

func TestProviderUnionUnknownInventoryCannotRemove(t *testing.T) {
	registry := capability.NewRegistry()
	instance := capability.ProviderInstance{ID: "sql/shared", Provider: capability.Provider{Kind: capability.ProviderPostgreSQL, Capabilities: []capability.Kind{capability.SQL}}, Scope: capability.ScopeShared, Ownership: capability.OwnershipBaseHarbor}
	if err := registry.Register(instance); err != nil {
		t.Fatal(err)
	}
	snapshot := ResolutionSnapshot{Target: "local", Registry: registry, Facts: map[string]ProviderFact{instance.ID: {Target: "local", State: ProviderRunning, Core: CoreNotRequired, Dependencies: []string{}}}}
	p, err := ResolveProviderPlan(snapshot, nil, false)
	if err != nil || p.Complete || p.Changes[0].Action != "preserve" {
		t.Fatalf("%+v %v", p, err)
	}
}
