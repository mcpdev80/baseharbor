package application

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

func retainedIntentFixture(t *testing.T) (ResolutionSnapshot, ConsumerIntent) {
	t.Helper()
	s, _, _ := impactFixture(t)
	desired := New("one", "dev", true, false, false)
	desired.ApplicationID = contractAppID
	previous := New("one", "dev", false, false, false)
	previous.ApplicationID = desired.ApplicationID
	previous.Services.Identity = true
	resource, err := capability.Resolve("one", capability.Requirement{Kind: capability.Identity, Name: "default"}, capability.Keycloak)
	if err != nil {
		t.Fatal(err)
	}
	s.Registry.Bindings = append(s.Registry.Bindings, capability.ProviderBinding{ApplicationID: desired.ApplicationID, Environment: desired.Environment, Resource: resource, ProviderInstanceID: "identity"})
	return s, ConsumerIntent{Key: "application/one", Active: true, Manifest: desired, RetainedIntents: []Manifest{previous}, DependencyRoots: []string{}}
}

func TestRetainedCanonicalIntentPreservesOneConsumerBindingsAndDependencies(t *testing.T) {
	s, consumer := retainedIntentFixture(t)
	before, _ := json.Marshal(struct {
		S ResolutionSnapshot
		C ConsumerIntent
	}{s, consumer})
	rootsOnly := consumer
	rootsOnly.RetainedIntents = nil
	rootsOnly.DependencyRoots = []string{"identity"}
	rootPlan, err := ResolveProviderPlan(s, []ConsumerIntent{rootsOnly}, true)
	if err != nil || len(rootPlan.ReleasedBindings) != 1 {
		t.Fatal("fixture must demonstrate that runtime roots alone cannot retain capability bindings", err)
	}
	plan, err := ResolveProviderPlan(s, []ConsumerIntent{consumer}, true)
	if err != nil || !plan.Complete || len(plan.Consumers) != 1 || len(plan.ReleasedBindings) != 0 || !reflect.DeepEqual(plan.Required, []string{"identity", "sql"}) {
		t.Fatalf("retained consumer lost its binding or actual dependency: %+v %v", plan, err)
	}
	if len(plan.Consumers[consumer.Key].Requirements) != 2 {
		t.Fatal("retained capability requirements disappeared behind a runtime-only root")
	}
	for _, change := range plan.Changes {
		if (change.InstanceID == "identity" || change.InstanceID == "sql") && (change.Action != "reuse" || !reflect.DeepEqual(change.Consumers, []string{consumer.Key})) {
			t.Fatal("retained same-owner projection became a duplicate consumer or retirement", change)
		}
	}
	after, _ := json.Marshal(struct {
		S ResolutionSnapshot
		C ConsumerIntent
	}{s, consumer})
	if string(before) != string(after) {
		t.Fatal("pure retained-intent projection mutated an existing owner")
	}
	consumer.RetainedIntents = nil // Only after the existing lifecycle verifies completion.
	finished, err := ResolveProviderPlan(s, []ConsumerIntent{consumer}, true)
	if err != nil || len(finished.ReleasedBindings) != 1 {
		t.Fatal("completed pending intent could not release its old binding", err)
	}
	for _, change := range finished.Changes {
		if change.InstanceID == "identity" && change.Action != "stop" {
			t.Fatal("last-consumer convergence remained permanently pinned")
		}
	}
}

func TestRetainedIntentRejectsForeignOwnershipAndInactiveProjection(t *testing.T) {
	for _, condition := range []string{"different-app", "different-environment", "missing-owner", "inactive"} {
		t.Run(condition, func(t *testing.T) {
			s, consumer := retainedIntentFixture(t)
			switch condition {
			case "different-app":
				consumer.RetainedIntents[0].ApplicationID = contractOtherID
			case "different-environment":
				consumer.RetainedIntents[0].Environment = "test"
			case "missing-owner":
				consumer.Manifest.ApplicationID, consumer.RetainedIntents[0].ApplicationID = "", ""
			case "inactive":
				consumer.Active = false
			}
			if _, err := ResolveProviderPlan(s, []ConsumerIntent{consumer}, true); err == nil {
				t.Fatal("retained canonical intent manufactured foreign or inactive authority")
			}
		})
	}
}

func TestRetainedIntentDeduplicatesRootsAndPreservesExplicitPreferences(t *testing.T) {
	s, consumer := retainedIntentFixture(t)
	consumer.RetainedIntents[0].Name = "before-readable-rename"
	consumer.RetainedIntents = append(consumer.RetainedIntents, consumer.RetainedIntents[0], consumer.Manifest)
	consumer.Preferences = []ProviderPreference{{Requirement: capability.Requirement{Kind: capability.SQL, Name: "default"}, InstanceID: "sql"}, {Requirement: capability.Requirement{Kind: capability.Identity, Name: "default"}, InstanceID: "identity"}}
	plan, err := ResolveProviderPlan(s, []ConsumerIntent{consumer}, true)
	if err != nil || len(plan.Consumers[consumer.Key].Requirements) != 2 || len(plan.Consumers[consumer.Key].Existing) != 2 {
		t.Fatal("same-owner projections duplicated resources or lost explicit preference", err)
	}
	for _, change := range plan.Changes {
		if len(change.Consumers) > 1 {
			t.Fatal("one app counted as multiple shared consumers")
		}
	}
	consumer.Preferences[1].InstanceID = "sql"
	if _, err := ResolveProviderPlan(s, []ConsumerIntent{consumer}, true); err == nil {
		t.Fatal("retained capability bypassed incompatible explicit selection")
	}
}

func TestUnknownRetainedCapabilityKeepsOverallUnionIncomplete(t *testing.T) {
	s, consumer := retainedIntentFixture(t)
	consumer.RetainedIntents[0].Services.Cache = true
	plan, err := ResolveProviderPlan(s, []ConsumerIntent{consumer}, true)
	if err != nil || plan.Complete || len(plan.ReleasedBindings) != 0 {
		t.Fatal("unknown retained capability was silently discarded", err)
	}
	for _, change := range plan.Changes {
		if change.Action == "stop" || change.Action == "remove-runtime" {
			t.Fatal("incomplete retained inventory authorized automatic retirement")
		}
	}
}
