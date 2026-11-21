package main

import (
	"encoding/json"
	"fmt"
	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"reflect"
	"time"
)

// The existing deployment retains BOTH active applied and previous intents
// during interruption. Only verified physical absence releases its consumer.
func recordPendingApplicationRuntimeRetirement(resolved resolvedApplication, plan application.ProviderImpactPlan) (deployment.DeploymentRecord, error) {
	return recordPendingApplicationRuntimeRetirementWithIntent(resolved, plan, resolved.Manifest)
}

func recordPendingApplicationRuntimeRetirementWithIntent(resolved resolvedApplication, plan application.ProviderImpactPlan, intent application.Manifest) (deployment.DeploymentRecord, error) {
	record, err := deployment.LoadDeploymentRecord(resolved.DeploymentIdentity)
	if err != nil {
		return record, err
	}
	// A verified retirement stores a capability-free projection in Applied.Intent.
	// A later app up can reactivate the repository manifest without immediately
	// replacing that deployment projection, so the current resolved manifest is
	// the authoritative owner when beginning another retirement.
	currentIntent, err := json.Marshal(intent)
	if err != nil {
		return record, invalidProviderConsumerInventory(fmt.Errorf("retirement intent differs from deployment owner"))
	}
	// An interrupted retirement already froze the physical owner before any
	// mutation. Preserve that owner across retries even if the repository source
	// changed afterward; only a completed retirement followed by `app up` has no
	// pending owner and should rebind from the currently resolved manifest.
	frozenPendingOwner := false
	if record.Observed.State == "retiring" {
		if pending := record.Applied.GeneratedState["previous_intent"]; pending != "" {
			currentIntent = json.RawMessage(pending)
			frozenPendingOwner = true
		} else {
			return record, invalidProviderConsumerInventory(fmt.Errorf("pending retirement has no frozen deployment owner"))
		}
	}
	var prior application.Manifest
	id := record.Identity
	if json.Unmarshal(currentIntent, &prior) != nil || prior.ApplicationID != id.ApplicationID || prior.Environment != id.Environment || prior.Name != id.Application {
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
	record.Applied.GeneratedState["previous_intent"] = string(currentIntent)
	// Rebind from the current manifest when it differs from the prior retired
	// owner. This also handles app up after retirement, where Applied.Intent and
	// Observed.State can still describe the old runtime projection.
	retiredIntent := record.Applied.GeneratedState["retired_intent"]
	retiredIntentMatches := retiredIntent != "" && applicationIntentsEqual([]byte(retiredIntent), currentIntent)
	placementsMatch := record.Applied.GeneratedState["retired_backend_placements"] == record.Applied.GeneratedState["backend_placements"]
	if !frozenPendingOwner && (retiredIntent == "" || !retiredIntentMatches || !placementsMatch) {
		if retiredIntent != "" && (!retiredIntentMatches || !placementsMatch) {
			if err := archiveReplacedRetiredRuntime(&record); err != nil {
				return record, err
			}
		}
		record.Applied.GeneratedState["retired_intent"] = string(currentIntent)
		record.Applied.GeneratedState["retired_backend_placements"] = record.Applied.GeneratedState["backend_placements"]
	}
	record.Applied.GeneratedState["provider_change_result"] = string(encoded)
	record.Observed = deployment.ObservedDeployment{State: "retiring"}
	return record, deployment.SaveDeploymentRecord(record)
}

func isApplicationRuntimeRetirementIntent(raw json.RawMessage) bool {
	var intent application.Manifest
	if len(raw) == 0 || json.Unmarshal(raw, &intent) != nil {
		return false
	}
	return reflect.DeepEqual(intent, applicationRuntimeRetirementIntent(intent))
}

func applicationIntentsEqual(leftRaw, rightRaw []byte) bool {
	var left, right application.Manifest
	if json.Unmarshal(leftRaw, &left) != nil || json.Unmarshal(rightRaw, &right) != nil {
		return false
	}
	return reflect.DeepEqual(left, right)
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
	delete(record.Applied.GeneratedState, "previous_backend_placements")
	record.Observed = deployment.ObservedDeployment{State: "runtime-retired", VerifiedAt: time.Now().UTC()}
	return deployment.SaveDeploymentRecord(record)
}
