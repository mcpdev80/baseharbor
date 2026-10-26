package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
)

func TestOtherInterruptedApplyRetainsOneConsumerAndPreviousBindings(t *testing.T) {
	for _, rename := range []bool{false, true} {
		t.Run(map[bool]string{false: "same-name", true: "readable-rename"}[rename], func(t *testing.T) {
			qualifyInterruptedSiblingConsumer(t, rename)
		})
	}
}

func TestOtherInterruptedApplyRejectsDamagedOrForeignPreviousIntent(t *testing.T) {
	for _, kind := range []string{"damaged", "foreign-app", "other-environment"} {
		t.Run(kind, func(t *testing.T) {
			target := configureTestTarget(t)
			current := application.New("current", "dev", false, false, false)
			sibling := application.New("sibling", "dev", false, false, false)
			record := registerTestDeployment(t, target, sibling, t.TempDir(), "baseharbor.yaml")
			prior := sibling
			switch kind {
			case "foreign-app":
				prior.ApplicationID = application.MustNewApplicationID()
			case "other-environment":
				prior.Environment = "prod"
			}
			previous, err := json.Marshal(prior)
			if err != nil {
				t.Fatal(err)
			}
			if kind == "damaged" {
				previous = []byte("{")
			}
			record.Applied.GeneratedState = map[string]string{"previous_intent": string(previous)}
			if err := deployment.SaveDeploymentRecord(record); err != nil {
				t.Fatal(err)
			}
			root, err := deployment.DeploymentRoot(record.Identity)
			if err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(filepath.Join(root, "deployment.json"))
			if err != nil {
				t.Fatal(err)
			}
			snapshot := application.ResolutionSnapshot{Target: target.Name, Registry: capability.NewRegistry(), Facts: map[string]application.ProviderFact{}}
			_, err = resolveApplicationProviderPlanFromSnapshot(resolvedApplication{Target: target, Manifest: current}, snapshot, nil)
			classified := machine.Classify(err)
			if err == nil || classified.Code != machine.ErrorOwnershipAmbiguous || classified.CauseCode != "provider_consumer_inventory_invalid" {
				t.Fatalf("unverified retained intent did not preserve ownership semantics: %v", err)
			}
			after, err := os.ReadFile(filepath.Join(root, "deployment.json"))
			if err != nil || string(before) != string(after) {
				t.Fatal("failed plan changed authoritative recovery record")
			}
		})
	}
}

func qualifyInterruptedSiblingConsumer(t *testing.T, rename bool) {
	target := configureTestTarget(t)
	current := application.New("current", "dev", false, false, false)
	prior := application.WithIdentity(application.New("sibling", "dev", false, false, false))
	desired := prior
	if rename {
		desired.Name = "sibling-renamed"
	}
	desired.Services.Identity = false
	desired.Identity = application.IdentityRequirements{}
	record := registerTestDeployment(t, target, desired, t.TempDir(), "baseharbor.yaml")
	previous, err := json.Marshal(prior)
	if err != nil {
		t.Fatal(err)
	}
	record.Applied.GeneratedState = map[string]string{"previous_intent": string(previous)}
	record.Observed.State = "applying"
	if err := deployment.SaveDeploymentRecord(record); err != nil {
		t.Fatal(err)
	}
	registry := capability.NewRegistry()
	for _, instance := range []capability.ProviderInstance{
		{ID: "identity/shared", Provider: capability.Keycloak, Scope: capability.ScopeShared, Ownership: capability.OwnershipBaseHarbor},
		{ID: "sql/shared", Provider: capability.PostgreSQL, Scope: capability.ScopeShared, Ownership: capability.OwnershipBaseHarbor},
	} {
		if err := registry.Register(instance); err != nil {
			t.Fatal(err)
		}
	}
	resource, err := capability.Resolve(prior.Name, capability.Requirement{Kind: capability.Identity, Name: "default"}, capability.Keycloak)
	if err != nil {
		t.Fatal(err)
	}
	registry.Bindings = append(registry.Bindings, capability.ProviderBinding{ApplicationID: prior.ApplicationID, Environment: prior.Environment, Resource: resource, ProviderInstanceID: "identity/shared"})
	snapshot := application.ResolutionSnapshot{Target: target.Name, Registry: registry, Facts: map[string]application.ProviderFact{
		"identity/shared": {Target: target.Name, State: application.ProviderRunning, Core: application.CoreNotRequired, Dependencies: []string{"sql/shared"}},
		"sql/shared":      {Target: target.Name, State: application.ProviderRunning, Core: application.CoreNotRequired, Dependencies: []string{}},
	}}
	before, _ := json.Marshal(snapshot)
	for attempt := 0; attempt < 2; attempt++ {
		plan, err := resolveApplicationProviderPlanFromSnapshot(resolvedApplication{Target: target, Manifest: current}, snapshot, nil)
		if err != nil || !plan.Complete {
			t.Fatalf("interrupted sibling must remain one authoritative consumer: %v", err)
		}
		if len(plan.Consumers) != 2 || len(plan.ReleasedBindings) != 0 || !reflect.DeepEqual(plan.Required, []string{"identity/shared", "sql/shared"}) {
			t.Fatalf("lost pending ownership or transitive requirements: %+v", plan)
		}
		for _, change := range plan.Changes {
			if change.Action != "reuse" || !reflect.DeepEqual(change.Consumers, []string{"deployment/" + record.Identity.DeploymentID}) {
				t.Fatalf("pending sibling provider was released: %+v", change)
			}
		}
	}
	after, _ := json.Marshal(snapshot)
	if string(before) != string(after) {
		t.Fatal("planning changed provider snapshot")
	}
	retained, err := deployment.LoadDeploymentRecord(record.Identity)
	if err != nil || retained.Applied.GeneratedState["previous_intent"] != string(previous) {
		t.Fatal("another app's plan changed interrupted ownership")
	}
}
