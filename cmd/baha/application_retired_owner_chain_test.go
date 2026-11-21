package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
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
	currentIntent, err := json.Marshal(prior)
	if err != nil {
		t.Fatal(err)
	}
	for range 3 {
		pending, err := recordPendingApplicationRuntimeRetirement(resolved, plan)
		if err != nil || pending.Applied.GeneratedState["retired_intent"] != archived || !applicationIntentsEqual([]byte(pending.Applied.GeneratedState["previous_intent"]), currentIntent) {
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

func TestReactivatedIntentReplacesRetiredOwnerAndArchivesPreviousIntent(t *testing.T) {
	target := configureTestTarget(t)
	previousOwner := application.New("retired-reactivated", "dev", true, false, false)
	retired := applicationRuntimeRetirementIntent(previousOwner)
	record := registerTestDeployment(t, target, retired, t.TempDir(), "baseharbor.yaml")
	previousIntent, err := json.Marshal(previousOwner)
	if err != nil {
		t.Fatal(err)
	}
	activeIntent := previousOwner
	activeIntent.Services.Cache = true
	activeIntentBytes, err := json.Marshal(activeIntent)
	if err != nil {
		t.Fatal(err)
	}
	previousPlacements, err := json.Marshal(map[capability.ProviderKind]capability.ProviderPlacement{
		capability.ProviderPostgreSQL: {Scope: capability.ScopeShared, Ownership: capability.OwnershipBaseHarbor},
	})
	if err != nil {
		t.Fatal(err)
	}
	currentPlacements, err := json.Marshal(map[capability.ProviderKind]capability.ProviderPlacement{
		capability.ProviderPostgreSQL: {Scope: capability.ScopeApplication, Ownership: capability.OwnershipBaseHarbor},
	})
	if err != nil {
		t.Fatal(err)
	}
	if record.Applied.GeneratedState == nil {
		record.Applied.GeneratedState = map[string]string{}
	}
	record.Applied.GeneratedState["retired_intent"] = string(previousIntent)
	record.Applied.GeneratedState["backend_placements"] = string(currentPlacements)
	record.Applied.GeneratedState["retired_backend_placements"] = string(previousPlacements)
	// `app up` after verified retirement can start the current repository
	// manifest while the persisted Applied.Intent remains the old capability-
	// free projection and Observed.State remains runtime-retired.
	record.Observed.State = "runtime-retired"
	if err := deployment.SaveDeploymentRecord(record); err != nil {
		t.Fatal(err)
	}

	resolved := resolvedApplication{Target: target, Manifest: activeIntent, DeploymentIdentity: record.Identity}
	plan := application.ProviderImpactPlan{Version: application.ProviderImpactVersion, Target: target.Name, PlanID: "reactivated-retirement", Complete: false, Impacts: []application.ProviderImpact{}}
	pending, err := recordPendingApplicationRuntimeRetirement(resolved, plan)
	if err != nil {
		t.Fatal(err)
	}
	if !applicationIntentsEqual([]byte(pending.Applied.GeneratedState["retired_intent"]), activeIntentBytes) ||
		!applicationIntentsEqual([]byte(pending.Applied.GeneratedState["previous_intent"]), activeIntentBytes) {
		t.Fatalf("reactivated active intent was not bound as the new pending retirement owner: retired_matches=%t previous_matches=%t",
			applicationIntentsEqual([]byte(pending.Applied.GeneratedState["retired_intent"]), activeIntentBytes),
			applicationIntentsEqual([]byte(pending.Applied.GeneratedState["previous_intent"]), activeIntentBytes))
	}
	history, err := retiredRuntimeHistory(pending)
	if err != nil || len(history) != 1 || !applicationIntentsEqual(history[0].Intent, previousIntent) {
		t.Fatalf("prior retained owner was not archived before replacing it: history=%d err=%v", len(history), err)
	}
	if history[0].BackendPlacements != string(previousPlacements) || pending.Applied.GeneratedState["retired_backend_placements"] != string(currentPlacements) {
		t.Fatal("retired owner replacement lost the previous or current backend placement projection")
	}
}

func TestInterruptedRetirementKeepsFrozenOwnerWhenRepositoryIntentChanges(t *testing.T) {
	target := configureTestTarget(t)
	prior := application.New("retired-interrupted", "dev", true, false, false)
	record := registerTestDeployment(t, target, prior, t.TempDir(), "baseharbor.yaml")
	priorBytes, err := json.Marshal(prior)
	if err != nil {
		t.Fatal(err)
	}
	if record.Applied.GeneratedState == nil {
		record.Applied.GeneratedState = map[string]string{}
	}
	record.Applied.GeneratedState["previous_intent"] = string(priorBytes)
	record.Applied.GeneratedState["retired_intent"] = string(priorBytes)
	record.Applied.GeneratedState["backend_placements"] = `{}`
	record.Applied.GeneratedState["retired_backend_placements"] = `{}`
	record.Observed.State = "retiring"
	if err := deployment.SaveDeploymentRecord(record); err != nil {
		t.Fatal(err)
	}

	changed := prior
	changed.Services.Cache = true
	resolved := resolvedApplication{Target: target, Manifest: changed, DeploymentIdentity: record.Identity}
	plan := application.ProviderImpactPlan{Version: application.ProviderImpactVersion, Target: target.Name, PlanID: "interrupted-owner", Complete: false, Impacts: []application.ProviderImpact{}}
	pending, err := recordPendingApplicationRuntimeRetirement(resolved, plan)
	if err != nil {
		t.Fatal(err)
	}
	if pending.Applied.GeneratedState["previous_intent"] != string(priorBytes) || pending.Applied.GeneratedState["retired_intent"] != string(priorBytes) {
		t.Fatal("interrupted retirement replaced the frozen physical owner with changed repository intent")
	}
	if got := pending.Applied.GeneratedState["retired_backend_placements"]; got != `{}` {
		t.Fatalf("interrupted retirement lost frozen backend placement: %q", got)
	}
}
