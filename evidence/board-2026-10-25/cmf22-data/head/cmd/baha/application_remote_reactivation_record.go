package main

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
)

// Only after actual immutable remote apply/verify. Reusing a protected
// publication never marks the caller's possibly edited Compose source applied.
func recordVerifiedRemoteReactivation(resolved resolvedApplication, intent application.Manifest) error {
	record, err := deployment.LoadDeploymentRecord(resolved.DeploymentIdentity)
	if err != nil {
		return err
	}
	if record.Identity != resolved.DeploymentIdentity || intent.ApplicationID != record.Identity.ApplicationID || intent.Name != record.Identity.Application || intent.Environment != record.Identity.Environment {
		return invalidProviderConsumerInventory(fmt.Errorf("verified remote intent differs from deployment owner"))
	}
	if err := application.ReconcileReferenceProviderRegistryAt(resolved.TargetStateRoot, intent); err != nil {
		return err
	}
	canonical, err := json.Marshal(intent)
	if err != nil {
		return err
	}
	record.Applied.Intent = canonical
	delete(record.Applied.GeneratedState, "previous_intent")
	delete(record.Applied.GeneratedState, "previous_backend_placements")
	record.Observed = deployment.ObservedDeployment{State: "ready", Ready: true, VerifiedAt: time.Now().UTC()}
	return deployment.SaveDeploymentRecord(record)
}
