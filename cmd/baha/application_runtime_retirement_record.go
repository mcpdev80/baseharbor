package main

import (
	"encoding/json"
	"fmt"
	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"time"
)

// The existing deployment retains BOTH active applied and previous intents
// during interruption. Only verified physical absence releases its consumer.
func recordPendingApplicationRuntimeRetirement(resolved resolvedApplication, plan application.ProviderImpactPlan) (deployment.DeploymentRecord, error) {
	record, err := deployment.LoadDeploymentRecord(resolved.DeploymentIdentity)
	if err != nil {
		return record, err
	}
	previous := record.Applied.Intent
	if pending := record.Applied.GeneratedState["previous_intent"]; pending != "" {
		previous = json.RawMessage(pending)
	}
	var prior application.Manifest
	id := record.Identity
	if len(previous) == 0 || json.Unmarshal(previous, &prior) != nil || prior.ApplicationID != id.ApplicationID || prior.Environment != id.Environment {
		return record, invalidProviderConsumerInventory(fmt.Errorf("retirement intent differs from deployment owner"))
	}
	if err := prior.ValidateIntent(); err != nil {
		return record, invalidProviderConsumerInventory(err)
	}
	result, err := application.RecordProviderChangeResult(plan, nil)
	if err != nil {
		return record, err
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return record, err
	}
	if record.Applied.GeneratedState == nil {
		record.Applied.GeneratedState = map[string]string{}
	}
	record.Applied.GeneratedState["previous_intent"] = string(previous)
	if record.Applied.GeneratedState["retired_intent"] == "" || (prior.Validate() == nil && record.Observed.State != "runtime-retired") {
		record.Applied.GeneratedState["retired_intent"] = string(record.Applied.Intent)
	}
	record.Applied.GeneratedState["provider_change_result"] = string(encoded)
	record.Observed = deployment.ObservedDeployment{State: "retiring"}
	return record, deployment.SaveDeploymentRecord(record)
}

// Call only after actual native retirement and selected reference release.
// Incomplete function/access/recovery knowledge remains a partial receipt.
func recordVerifiedApplicationRuntimeRetirement(resolved resolvedApplication, plan application.ProviderImpactPlan, outcomes []application.ProviderExecutionOutcome, retained []application.ProviderMaterial) error {
	result, err := application.RecordProviderChangeResult(plan, outcomes)
	if err != nil {
		return err
	}
	record, err := deployment.LoadDeploymentRecord(resolved.DeploymentIdentity)
	if err != nil {
		return err
	}
	if record.Observed.State != "retiring" || record.Applied.GeneratedState["previous_intent"] == "" {
		return invalidProviderConsumerInventory(fmt.Errorf("retirement has no matching pending deployment owner"))
	}
	var pending application.ProviderChangeResult
	if json.Unmarshal([]byte(record.Applied.GeneratedState["provider_change_result"]), &pending) != nil || pending.Version != application.ProviderImpactVersion || pending.PlanID != plan.PlanID {
		return classifyProviderChangeConsent(&application.ProviderChangeError{Code: "stale-impact-result"})
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return err
	}
	material, err := json.Marshal(retained)
	if err != nil {
		return err
	}
	var applied application.Manifest
	if json.Unmarshal(record.Applied.Intent, &applied) != nil || applied.ApplicationID != record.Identity.ApplicationID || applied.Environment != record.Identity.Environment || applied.Name != record.Identity.Application {
		return invalidProviderConsumerInventory(fmt.Errorf("retirement applied intent differs from canonical owner"))
	}
	intent, err := json.Marshal(applicationRuntimeRetirementIntent(applied))
	if err != nil {
		return err
	}
	record.Applied.Intent = intent
	record.Applied.GeneratedState["provider_change_result"] = string(encoded)
	record.Applied.GeneratedState["retired_materials"] = string(material)
	delete(record.Applied.GeneratedState, "previous_intent")
	record.Observed = deployment.ObservedDeployment{State: "runtime-retired", VerifiedAt: time.Now().UTC()}
	return deployment.SaveDeploymentRecord(record)
}
