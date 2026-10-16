package main

import (
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
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
