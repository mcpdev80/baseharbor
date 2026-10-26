package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
)

func TestRetirementRecordRetainsActiveOwnershipUntilVerifiedAndRejectsStaleResult(t *testing.T) {
	target := configureTestTarget(t)
	m := application.New("retained", "dev", true, false, false)
	record := registerTestDeployment(t, target, m, t.TempDir(), "baseharbor.yaml")
	record.Applied.GeneratedState = map[string]string{"recovery_reference": "owned/recovery"}
	if err := deployment.SaveDeploymentRecord(record); err != nil {
		t.Fatal(err)
	}
	record, err := deployment.LoadDeploymentRecord(record.Identity)
	if err != nil {
		t.Fatal(err)
	}
	resolved := resolvedApplication{Target: target, Manifest: m, DeploymentIdentity: record.Identity}
	plan := application.ProviderImpactPlan{Version: application.ProviderImpactVersion, Target: target.Name, PlanID: "current-plan", Complete: false, Impacts: []application.ProviderImpact{}}
	for attempt := 0; attempt < 2; attempt++ {
		pending, err := recordPendingApplicationRuntimeRetirement(resolved, plan)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(pending.Applied.Intent, record.Applied.Intent) || pending.Applied.GeneratedState["recovery_reference"] != "owned/recovery" || pending.Observed.Ready {
			t.Fatal("interrupted retirement released active ownership or recovery")
		}
	}
	root, _ := deployment.DeploymentRoot(record.Identity)
	before, _ := os.ReadFile(filepath.Join(root, "deployment.json"))
	stale := plan
	stale.PlanID = "stale-plan"
	if recordVerifiedApplicationRuntimeRetirement(resolved, stale, nil, nil) == nil {
		t.Fatal("stale receipt finalized another plan")
	}
	after, _ := os.ReadFile(filepath.Join(root, "deployment.json"))
	if !reflect.DeepEqual(before, after) {
		t.Fatal("stale finalization mutated recovery state")
	}
	if err := recordVerifiedApplicationRuntimeRetirement(resolved, plan, nil, nil); err != nil {
		t.Fatal(err)
	}
	final, err := deployment.LoadDeploymentRecord(record.Identity)
	if err != nil {
		t.Fatal(err)
	}
	var intent application.Manifest
	if json.Unmarshal(final.Applied.Intent, &intent) != nil || intent.ApplicationID != m.ApplicationID || intent.Services.SQL || final.Observed.Ready || final.Observed.State != "runtime-retired" || final.Applied.GeneratedState["previous_intent"] != "" || final.Applied.GeneratedState["retired_intent"] == "" {
		t.Fatal("verified runtime result lost owner or retained active consumer")
	}
	result, err := applicationDestroyResult(resolved, false)
	if err != nil || !result.OwnerPreserved || result.DataDeleted == nil || *result.DataDeleted || result.ProviderChangeResult == nil || result.ProviderChangeResult.Status != "partial" {
		t.Fatalf("retirement result manufactured complete consequences: %+v %v", result, err)
	}
	final.Applied.GeneratedState["provider_change_result"] = `{"version":"` + application.ProviderImpactVersion + `","plan_id":"password=private-value","status":"partial","outcomes":[]}`
	if err := deployment.SaveDeploymentRecord(final); err != nil {
		t.Fatal(err)
	}
	if _, err := applicationDestroyResult(resolved, false); err == nil || strings.Contains(err.Error(), "private-value") {
		t.Fatal("invalid stored metadata accepted or reflected")
	}
}
