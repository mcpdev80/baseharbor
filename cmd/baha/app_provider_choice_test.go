package main

import (
	"os/exec"
	"reflect"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
)

const providerChoiceAppID = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"

func providerChoiceManifest(environment string, names ...string) application.Manifest {
	m := application.New("catalog", environment, false, false, false)
	m.ApplicationID = providerChoiceAppID
	return application.WithSQLInstances(m, names...)
}

func providerChoiceShared(id, boundary string) capability.ProviderInstance {
	return capability.ProviderInstance{ID: id, Provider: capability.PostgreSQL,
		Scope: capability.ScopeShared, Ownership: capability.OwnershipBaseHarbor, SharingBoundary: boundary}
}

func providerChoiceSnapshot(target string, instances ...capability.ProviderInstance) application.ResolutionSnapshot {
	reg := capability.NewRegistry()
	reg.Instances = instances
	return application.ResolutionSnapshot{Target: target, Registry: reg, Facts: map[string]application.ProviderFact{}}
}

func providerChoiceSelection(names ...string) appCapabilitySelectionModel {
	reqs := make([]capability.Requirement, 0, len(names))
	for _, name := range names {
		reqs = append(reqs, capability.Requirement{Kind: capability.SQL, Name: name})
	}
	return newAppCapabilitySelection(appCapabilitySelectionInput{Current: reqs})
}

func TestAppProviderChoiceAutomaticAndExplicit(t *testing.T) {
	m := providerChoiceManifest("dev", "primary")
	snapshot := providerChoiceSnapshot("target-dev", providerChoiceShared("sql-a", ""))
	if err := snapshot.Registry.Validate(); err != nil {
		t.Fatal(err)
	}
	choices, err := appProviderChoices(m, snapshot, providerChoiceSelection("primary"))
	if err != nil || len(choices) != 1 {
		t.Fatalf("choices=%+v err=%v", choices, err)
	}
	c := choices[0]
	if c.Preference().InstanceID != "" || c.ProposedInstanceID != "sql-a" {
		t.Fatalf("automatic=%+v", c)
	}
	if !strings.Contains(c.View(), "Automatic") || !strings.Contains(c.View(), "sql-a") {
		t.Fatal(c.View())
	}
	c, err = c.Select("sql-a")
	if err != nil || c.Preference().Requirement.Name != "primary" {
		t.Fatalf("explicit=%+v err=%v", c, err)
	}
	if !strings.Contains(c.View(), "Explicit") || !strings.Contains(c.View(), "Selected: sql-a") {
		t.Fatal(c.View())
	}
	c, err = c.Select("")
	if err != nil || !strings.Contains(c.View(), "Automatic") || c.Preference().InstanceID != "" {
		t.Fatal(c.View())
	}
}

func TestAppProviderChoiceMultipleNamesHaveIndependentPreferences(t *testing.T) {
	m := providerChoiceManifest("dev", "primary", "analytics")
	snapshot := providerChoiceSnapshot("dev", providerChoiceShared("a", "alpha"), providerChoiceShared("b", "beta"))
	if err := snapshot.Registry.Validate(); err != nil {
		t.Fatal(err)
	}
	choices, err := appProviderChoices(m, snapshot, providerChoiceSelection("primary", "analytics"))
	if err != nil || len(choices) != 2 {
		t.Fatalf("choices=%+v err=%v", choices, err)
	}
	first, err := choices[0].Select("a")
	if err != nil {
		t.Fatal(err)
	}
	second, err := choices[1].Select("b")
	if err != nil {
		t.Fatal(err)
	}
	if first.Preference().Requirement.Name != "primary" || second.Preference().Requirement.Name != "analytics" ||
		first.Preference().InstanceID != "a" || second.Preference().InstanceID != "b" ||
		first.Preference().Target != "dev" || second.Preference().Target != "dev" {
		t.Fatalf("independent preferences %+v %+v", first.Preference(), second.Preference())
	}
	if _, err := choices[0].Effective(); err == nil {
		t.Fatal("ambiguous Automatic silently substituted")
	}
}

func TestAppProviderChoiceExistingBindingWinsAndConflictsFail(t *testing.T) {
	m := providerChoiceManifest("dev", "primary")
	s := providerChoiceSnapshot("dev", providerChoiceShared("a", "alpha"), providerChoiceShared("b", "beta"))
	resource, err := capability.Resolve(m.Name, capability.Requirement{Kind: capability.SQL, Name: "primary"}, capability.PostgreSQL)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Registry.BindDeployment(resource, m.ApplicationID, m.Environment, "b"); err != nil {
		t.Fatal(err)
	}
	if err := s.Registry.Validate(); err != nil {
		t.Fatal(err)
	}
	fp, err := application.ResolveFootprint(m, s, nil)
	if err != nil || len(fp.Requirements) != 1 || fp.Requirements[0].InstanceID != "b" {
		t.Fatalf("authority=%+v err=%v", fp, err)
	}
	choices, err := appProviderChoices(m, s, providerChoiceSelection("primary"))
	if err != nil || choices[0].ProposedInstanceID != "b" {
		t.Fatalf("binding not retained: %+v %v", choices, err)
	}
	if _, err := choices[0].Select("a"); err == nil {
		t.Fatal("binding conflict accepted")
	}
	if chosen, err := choices[0].Effective(); err != nil || chosen.ID != "b" {
		t.Fatalf("effective=%+v err=%v", chosen, err)
	}
}

func TestAppProviderChoiceEnvironmentIsolationAndTargetSwitch(t *testing.T) {
	m := providerChoiceManifest("dev", "primary")
	instance := capability.ProviderInstance{ID: "prod", Provider: capability.PostgreSQL,
		Scope: capability.ScopeApplication, Ownership: capability.OwnershipBaseHarbor,
		OwnerApplicationID: m.ApplicationID, OwnerApplication: m.Name, OwnerEnvironment: "prod"}
	shared := providerChoiceShared("shared", "")
	s := providerChoiceSnapshot("dev-target", instance, shared)
	if err := s.Registry.Validate(); err != nil {
		t.Fatal(err)
	}
	choices, err := appProviderChoices(m, s, providerChoiceSelection("primary"))
	if err != nil || len(choices[0].Candidates) != 1 || choices[0].Candidates[0].Instance.ID != "shared" {
		t.Fatalf("dev target leaked prod instance: %+v %v", choices, err)
	}
	m.Environment = "prod"
	s.Target = "prod-target"
	prod, err := appProviderChoices(m, s, providerChoiceSelection("primary"))
	if err != nil || len(prod[0].Candidates) != 2 {
		t.Fatalf("prod choices=%+v err=%v", prod, err)
	}
	if prod[0].Preference().Target != "prod-target" || choices[0].Preference().Target != "dev-target" {
		t.Fatal("target preferences leaked")
	}
}

func TestAppProviderChoiceRegistryValidityAndDuplicateIDs(t *testing.T) {
	m := providerChoiceManifest("dev", "primary")
	s := providerChoiceSnapshot("dev", providerChoiceShared("a", ""))
	s.Registry.Instances[0].Ownership = capability.OwnershipExternal
	if err := s.Registry.Validate(); err == nil {
		t.Fatal("invalid registry test fixture")
	}
	if _, err := appProviderChoices(m, s, providerChoiceSelection("primary")); err == nil {
		t.Fatal("invalid registry accepted")
	}
	s = providerChoiceSnapshot("dev", providerChoiceShared("duplicate", "a"), providerChoiceShared("duplicate", "b"))
	if err := s.Registry.Validate(); err == nil {
		t.Fatal("duplicate IDs not rejected by authority")
	}
	if _, err := appProviderChoices(m, s, providerChoiceSelection("primary")); err == nil {
		t.Fatal("duplicate IDs accepted")
	}
}

func TestAppProviderChoiceNoProviderAndPortableRequirements(t *testing.T) {
	m := providerChoiceManifest("dev", "primary")
	s := providerChoiceSnapshot("dev")
	selection := providerChoiceSelection("primary")
	before := selection.Requirements()
	choices, err := appProviderChoices(m, s, selection)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := choices[0].Effective(); err == nil || !strings.Contains(err.Error(), "No compatible") {
		t.Fatalf("missing provider: %v", err)
	}
	if _, err := choices[0].Select("unknown"); err == nil {
		t.Fatal("unknown provider accepted")
	}
	if !reflect.DeepEqual(before, selection.Requirements()) {
		t.Fatal("portable requirements mutated")
	}
}

func TestAppProviderChoiceNewNeedOverSQLFreeManifest(t *testing.T) {
	m := application.New("catalog", "dev", false, false, false)
	m.ApplicationID = providerChoiceAppID
	s := providerChoiceSnapshot("dev", providerChoiceShared("sql", ""))
	selection := providerChoiceSelection("default")
	before := m
	choices, err := appProviderChoices(m, s, selection)
	if err != nil || len(choices) != 1 || choices[0].ProposedInstanceID != "sql" {
		t.Fatalf("new SQL need: %+v %v", choices, err)
	}
	if !reflect.DeepEqual(m, before) {
		t.Fatal("manifest mutated")
	}
}

func TestAppProviderChoiceDeselectedAmbiguousOldNeed(t *testing.T) {
	m := providerChoiceManifest("dev", "primary")
	m = application.WithCacheInstances(m, "default")
	s := providerChoiceSnapshot("dev", providerChoiceShared("a", "alpha"), providerChoiceShared("b", "beta"),
		capability.ProviderInstance{ID: "cache", Provider: capability.Valkey, Scope: capability.ScopeShared, Ownership: capability.OwnershipBaseHarbor})
	selection := newAppCapabilitySelection(appCapabilitySelectionInput{Current: []capability.Requirement{{Kind: capability.KeyValue, Name: "default"}}})
	choices, err := appProviderChoices(m, s, selection)
	if err != nil || len(choices) != 1 || choices[0].ProposedInstanceID != "cache" {
		t.Fatalf("deselected SQL blocked cache: %+v %v", choices, err)
	}
}

func TestAppProviderChoiceMixedAmbiguityOrderIndependent(t *testing.T) {
	m := application.New("catalog", "dev", false, false, false)
	m.ApplicationID = providerChoiceAppID
	s := providerChoiceSnapshot("dev", providerChoiceShared("a", "alpha"), providerChoiceShared("b", "beta"),
		capability.ProviderInstance{ID: "cache", Provider: capability.Valkey, Scope: capability.ScopeShared, Ownership: capability.OwnershipBaseHarbor})
	sql := capability.Requirement{Kind: capability.SQL, Name: "default"}
	cache := capability.Requirement{Kind: capability.KeyValue, Name: "default"}
	for _, order := range [][]capability.Requirement{{sql, cache}, {cache, sql}} {
		selection := newAppCapabilitySelection(appCapabilitySelectionInput{Current: order})
		original := selection.Requirements()
		snapBefore := s
		manifestBefore := m
		choices, err := appProviderChoices(m, s, selection)
		if err != nil || len(choices) != 2 {
			t.Fatalf("order %v: %+v %v", order, choices, err)
		}
		result := map[capability.Kind]string{}
		for _, choice := range choices {
			result[choice.Requirement.Kind] = choice.ProposedInstanceID
		}
		if result[capability.SQL] != "" || result[capability.KeyValue] != "cache" {
			t.Fatalf("independent proposals: %v", result)
		}
		if !reflect.DeepEqual(selection.Requirements(), original) || !reflect.DeepEqual(m, manifestBefore) || !reflect.DeepEqual(s, snapBefore) {
			t.Fatal("original input mutated")
		}
	}
}

func TestAppProviderChoiceFormatting(t *testing.T) {
	for _, name := range []string{"app_provider_choice.go", "app_provider_choice_test.go"} {
		diff, err := exec.Command("gofmt", "-d", name).CombinedOutput()
		if err != nil || len(diff) > 0 {
			t.Fatalf("gofmt mismatch in %s: %s (%v)", name, diff, err)
		}
	}
}
