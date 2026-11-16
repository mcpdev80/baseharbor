package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
)

func TestRetiredOwnerChainPreservesBrokerArchiveAndAcceptsInactiveBackendIntent(t *testing.T) {
	target := configureTestTarget(t)
	prior := application.WithWorkloadComponents(application.New("retired-secrets", "dev", false, false, true), "api")
	retired := applicationRuntimeRetirementIntent(prior)
	record := registerTestDeployment(t, target, retired, t.TempDir(), "baseharbor.yaml")
	encoded, err := json.Marshal(prior)
	if err != nil {
		t.Fatal(err)
	}
	record.Applied.GeneratedState = map[string]string{"retired_intent": string(encoded)}
	record.Observed.State = "runtime-retired"
	if err := deployment.SaveDeploymentRecord(record); err != nil {
		t.Fatal(err)
	}
	root, err := deployment.DeploymentRoot(record.Identity)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "deployment.json")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	resolved := resolvedApplication{Target: target, Manifest: prior, Store: application.Store{Root: filepath.Join(root, "state"), Namespace: target.Name}, DeploymentIdentity: record.Identity}
	inactive, err := retainedApplicationBackendIntent(resolved)
	if err != nil || inactive.ApplicationID != prior.ApplicationID || inactive.Services.Secrets || application.HasExplicitWorkload(inactive) {
		t.Fatal("inactive canonical owner rejected or archived live intent made active", err)
	}
	brokers, err := applicationBrokerOwners(resolved)
	if err != nil || len(brokers) != 1 || !brokers[0].existing || brokers[0].manifest.ApplicationID != prior.ApplicationID || brokers[0].store.Root != resolved.Store.Root {
		t.Fatal("retired canonical intent obscures archived actual broker owner", err)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("read-only broker/backend projection altered recovery owner")
	}
}

func TestRetiredPendingRetryNeverReplacesArchivedLiveOwnerWithEmptyIntent(t *testing.T) {
	target := configureTestTarget(t)
	prior := application.New("retired-repeat", "dev", true, false, false)
	record := registerTestDeployment(t, target, prior, t.TempDir(), "baseharbor.yaml")
	resolved := resolvedApplication{Target: target, Manifest: prior, DeploymentIdentity: record.Identity}
	plan := application.ProviderImpactPlan{Version: application.ProviderImpactVersion, Target: target.Name, PlanID: "retired-retry", Complete: false, Impacts: []application.ProviderImpact{}}
	if _, err := recordPendingApplicationRuntimeRetirement(resolved, plan); err != nil {
		t.Fatal(err)
	}
	if err := recordVerifiedApplicationRuntimeRetirement(resolved, plan, nil, nil); err != nil {
		t.Fatal(err)
	}
	retired, err := deployment.LoadDeploymentRecord(record.Identity)
	if err != nil {
		t.Fatal(err)
	}
	archived := retired.Applied.GeneratedState["retired_intent"]
	for range 3 {
		pending, err := recordPendingApplicationRuntimeRetirement(resolved, plan)
		if err != nil || pending.Applied.GeneratedState["retired_intent"] != archived || pending.Applied.GeneratedState["previous_intent"] != string(retired.Applied.Intent) {
			t.Fatal("reviewed repeat/interrupted retry lost archived live ownership", err)
		}
	}
	if err := recordVerifiedApplicationRuntimeRetirement(resolved, plan, nil, nil); err != nil {
		t.Fatal(err)
	}
	final, err := deployment.LoadDeploymentRecord(record.Identity)
	if err != nil || final.Applied.GeneratedState["retired_intent"] != archived || final.Applied.GeneratedState["previous_intent"] != "" || final.Identity != record.Identity {
		t.Fatal("verified repeat retirement changed canonical recovery identity", err)
	}
}
