package application

import (
	"encoding/json"
	"errors"
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

func TestProviderUnionDependencyClosurePreservesSharedSQL(t *testing.T) {
	r := capability.NewRegistry()
	for _, i := range []capability.ProviderInstance{
		{ID: "sql/shared", Provider: capability.PostgreSQL, Scope: capability.ScopeShared, Ownership: capability.OwnershipBaseHarbor},
		{ID: "identity/shared", Provider: capability.Provider{Kind: capability.ProviderKeycloak, Capabilities: []capability.Kind{capability.Identity}}, Scope: capability.ScopeShared, Ownership: capability.OwnershipBaseHarbor},
	} {
		if err := r.Register(i); err != nil {
			t.Fatal(err)
		}
	}
	s := ResolutionSnapshot{Target: "test", Registry: r, Facts: map[string]ProviderFact{
		"sql/shared":      {Target: "test", State: ProviderRunning, Core: CoreNotRequired, Dependencies: []string{}},
		"identity/shared": {Target: "test", State: ProviderRunning, Core: CoreNotRequired, Dependencies: []string{"sql/shared"}},
	}}
	sql := New("app", "test", true, false, false)
	id := New("console", "test", false, false, false)
	id.Services.Identity = true
	consumers := []ConsumerIntent{{Key: "app", Active: true, Manifest: sql}, {Key: "console", Active: true, Manifest: id}}
	p, err := ResolveProviderPlan(s, consumers, true)
	if err != nil || !p.Complete || len(p.Required) != 2 {
		t.Fatalf("%+v %v", p, err)
	}
	consumers[0].Active = false
	p, err = ResolveProviderPlan(s, consumers, true)
	if err != nil || len(p.Required) != 2 {
		t.Fatalf("dependency removed: %+v %v", p, err)
	}
	consumers[0].Active = true
	consumers[1].Active = false
	p, err = ResolveProviderPlan(s, consumers, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range p.Changes {
		if c.InstanceID == "sql/shared" && c.Action != "reuse" {
			t.Fatal("shared SQL stopped")
		}
		if c.InstanceID == "identity/shared" && c.Action != "stop" {
			t.Fatal("unused Identity retained running")
		}
	}
	// Never trust a complete caller inventory when a registered owner is omitted.
	resource, _ := capability.Resolve("foreign", capability.Requirement{Kind: capability.SQL, Name: "default"}, capability.PostgreSQL)
	s.Registry.Bindings = append(s.Registry.Bindings, capability.ProviderBinding{ApplicationID: contractOtherID, Environment: "test", Resource: resource, ProviderInstanceID: "sql/shared"})
	p, err = ResolveProviderPlan(s, nil, true)
	if err != nil || p.Complete {
		t.Fatalf("unaccounted owner accepted: %+v %v", p, err)
	}
	for _, c := range p.Changes {
		if c.Action != "preserve" {
			t.Fatal("unaccounted inventory authorizes mutation")
		}
	}
}

func TestExplicitProviderTypeNegotiationUsesExistingRegistry(t *testing.T) {
	s := contractSnapshot()
	m := contractSQL()
	req := capability.Requirement{Kind: capability.SQL, Name: "default"}
	for _, tc := range []struct {
		kind capability.ProviderKind
		code string
	}{{capability.ProviderPostgreSQL, ""}, {capability.ProviderValkey, "missing"}} {
		_, err := ResolveFootprint(m, s, []ProviderPreference{{Requirement: req, ProviderKind: tc.kind}})
		if tc.code == "" && err != nil {
			t.Fatal(err)
		}
		if tc.code != "" {
			var re *ResolutionError
			if !errors.As(err, &re) || re.Code != tc.code {
				t.Fatalf("%v", err)
			}
		}
	}
}

func TestVerifiedConsumptionRootsUseSameDependencyResolver(t *testing.T) {
	m := WithWorkloadComponents(Manifest{Version: 1, ApplicationID: contractAppID, Name: "myapp", Environment: "dev"}, "api")
	m.Consumes = []ConsumptionRequirement{{Name: "api", ApplicationID: contractOtherID, Component: "api", Interface: "http", Protocol: "http"}}
	s := contractSnapshot()
	s.DependencyRoots = []string{"sql/shared"}
	fp, err := ResolveFootprint(m, s, nil)
	if err != nil || !fp.Complete || len(fp.Existing) != 1 {
		t.Fatalf("verified closure missing: %+v %v", fp, err)
	}
	s.DependencyRoots = []string{}
	fp, err = ResolveFootprint(m, s, nil)
	if err != nil || !fp.Complete || len(fp.Existing) != 0 || fp.Core != CoreNotRequired {
		t.Fatalf("verified direct consumption: %+v %v", fp, err)
	}
	s.DependencyRoots = []string{"missing"}
	if _, err = ResolveFootprint(m, s, nil); err == nil {
		t.Fatal("invalid extra root accepted")
	}
}

func TestProviderUnionInvalidUnusedStateBlocksAllRetirement(t *testing.T) {
	s := contractSnapshot()
	// Inactive or unbound providers still need a valid ownership/state observation.
	fact := s.Facts["sql/shared"]
	fact.State = "stale"
	s.Facts["sql/shared"] = fact
	p, err := ResolveProviderPlan(s, nil, true)
	if err != nil || p.Complete {
		t.Fatalf("invalid inventory accepted: %+v %v", p, err)
	}
	for _, change := range p.Changes {
		if change.Action != "preserve" {
			t.Fatal("invalid state authorizes removal")
		}
	}
}
