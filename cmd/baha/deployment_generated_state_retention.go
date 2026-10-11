package main

import (
	"errors"
	"os"

	"github.com/mcpdev80/baseharbor/internal/deployment"
)

// A reactivation updates known realization references, not the developer's
// retained recovery inventory. Keep existing GeneratedState in its canonical
// owner, including references whose provider is currently disabled.
func retainDeploymentGeneratedState(record *deployment.DeploymentRecord, verified bool) error {
	current, err := deployment.LoadDeploymentRecord(record.Identity)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	generated := map[string]string{}
	for key, value := range current.Applied.GeneratedState {
		generated[key] = value
	}
	for key, value := range record.Applied.GeneratedState {
		generated[key] = value
	}
	if verified {
		delete(generated, "previous_intent")
	}
	record.Applied.GeneratedState = generated
	return nil
}
