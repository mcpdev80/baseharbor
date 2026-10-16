package main

import (
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
)

func TestAppChangeResultPresenterPartialAndPending(t *testing.T) {
	plan := application.ProviderImpactPlan{Version: application.ProviderImpactVersion, PlanID: "plan-1", Target: "dev", Complete: true, Impacts: []application.ProviderImpact{
		{InstanceID: "a", Operation: "stop"},
		{InstanceID: "b", Operation: "destroy"},
		{InstanceID: "c", Operation: "replace"},
	}}
	recorded, err := application.RecordProviderChangeResult(plan, []application.ProviderExecutionOutcome{
		{InstanceID: "a", Operation: "stop", Status: "failed", ErrorCode: "permission-denied"},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := appChangeResultPresenter(plan, recorded)
	if err != nil {
		t.Fatal(err)
	}
	for _, part := range []string{"partial", "permission-denied", "pending", "a: stop", "b: destroy", "c: replace", "replan", "fresh consent"} {
		if !strings.Contains(got, part) {
			t.Fatalf("missing %q: %s", part, got)
		}
	}
}

func TestAppChangeResultPresenterCompleteAndNoSyntheticLocations(t *testing.T) {
	plan := application.ProviderImpactPlan{Version: application.ProviderImpactVersion, PlanID: "plan-2", Target: "dev", Complete: true, Impacts: []application.ProviderImpact{{InstanceID: "old", Operation: "destroy"}}}
	result, err := application.RecordProviderChangeResult(plan, []application.ProviderExecutionOutcome{{InstanceID: "old", Operation: "destroy", Status: "applied", Verified: true, Evidence: "signed-receipt"}})
	if err != nil {
		t.Fatal(err)
	}
	got, err := appChangeResultPresenter(plan, result)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "complete") || !strings.Contains(got, "No unfinished") {
		t.Fatal(got)
	}
	if strings.Contains(got, "signed-receipt") || strings.Contains(got, "backup") {
		t.Fatal("exposed evidence or invented recovery")
	}
}

func TestAppChangeResultPresenterRejectsStaleResultAndRedactsSecrets(t *testing.T) {
	plan := application.ProviderImpactPlan{Version: application.ProviderImpactVersion, PlanID: "current", Target: "dev", Impacts: []application.ProviderImpact{{InstanceID: "id", Operation: "stop"}}}
	if _, err := appChangeResultPresenter(plan, application.ProviderChangeResult{Version: application.ProviderImpactVersion, PlanID: "stale"}); err == nil {
		t.Fatal("stale result accepted")
	}
	result, err := application.RecordProviderChangeResult(plan, []application.ProviderExecutionOutcome{{InstanceID: "id", Operation: "stop", Status: "failed", ErrorCode: "failed", Evidence: "SUPER-SECRET"}})
	if err != nil {
		t.Fatal(err)
	}
	got, err := appChangeResultPresenter(plan, result)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "SUPER-SECRET") {
		t.Fatal("receipt evidence exposed")
	}
}

func TestAppChangeResultPresenterActualPartialSecurityAndBehavior(t *testing.T) {
	access := &application.ProviderAccess{
		Method: "secret-method-hidden", Endpoint: "https://password@example.invalid",
		Security: capability.SecureBinding{
			Secrets: []capability.SecretReference{{Name: "hidden-token", Reference: "baseharbor://secrets/token"}},
			Lifecycle: capability.SecurityLifecycleSupport{Renewable: false, Rotatable: true},
		},
	}
	plan := application.ProviderImpactPlan{Version: application.ProviderImpactVersion, PlanID: "real-partial", Target: "prod", Complete: true, Impacts: []application.ProviderImpact{
		{InstanceID: "identity", Operation: "stop"},
		{InstanceID: "broker", Operation: "replace"},
	}}
	result, err := application.RecordProviderChangeResult(plan, []application.ProviderExecutionOutcome{{
		InstanceID: "identity", Operation: "stop", Status: "failed", ErrorCode: "lease-still-active",
		After: &application.ProviderRealization{InstanceID: "identity", Kind: capability.ProviderKeycloak, Target: "prod", State: "running", Access: access, Verified: false},
		Functions: []application.ProviderFunctionOutcome{{Consumer: "web", InstanceID: "identity", Function: "identity.validate",
			Outcome: "remains", Behavior: "cached-until-expiry", ValidUntil: "2026-11-01T00:00:00Z", Verified: false}},
		Materials: []application.MaterialOutcome{{MaterialID: "token", Disposition: "retain", Access: access, Verified: false}},
	}})
	if err != nil { t.Fatal(err) }
	got, err := appChangeResultPresenter(plan, result)
	if err != nil { t.Fatal(err) }
	for _, want := range []string{
		"partial", "lease-still-active", "cached-until-expiry", "Actual provider access: reported",
		"Actual material access: reported", "secret references: 1", "renewable: false", "rotatable: true",
		"verified: false", "broker: replace", "fresh consent",
	} {
		if !strings.Contains(got, want) { t.Errorf("missing actual result %q: %s", want, got) }
	}
	for _, secret := range []string{"secret-method-hidden", "password@example", "hidden-token", "baseharbor://"} {
		if strings.Contains(got, secret) { t.Fatalf("sensitive receipt data leaked %q", secret) }
	}
}
