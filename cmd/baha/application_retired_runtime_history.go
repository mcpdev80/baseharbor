package main

import (
	"encoding/json"
	"fmt"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
)

// Older retired definitions still own retained resources after reactivation
// with fewer capabilities. Keep their original placement beside the intent in
// the existing protected deployment record, without creating active consumers.
type retiredRuntimeOwner struct {
	Intent            json.RawMessage `json:"intent"`
	BackendPlacements string          `json:"backend_placements"`
}

func retiredRuntimeHistory(record deployment.DeploymentRecord) ([]retiredRuntimeOwner, error) {
	encoded := record.Applied.GeneratedState["retired_runtime_history"]
	if encoded == "" {
		return nil, nil
	}
	var history []retiredRuntimeOwner
	if len(encoded) > 1<<20 || json.Unmarshal([]byte(encoded), &history) != nil || history == nil || len(history) > 64 {
		return nil, invalidProviderConsumerInventory(fmt.Errorf("invalid retired runtime owner history"))
	}
	for _, owner := range history {
		var m application.Manifest
		if json.Unmarshal(owner.Intent, &m) != nil || m.ApplicationID != record.Identity.ApplicationID || m.Environment != record.Identity.Environment || m.ValidateIntent() != nil {
			return nil, invalidProviderConsumerInventory(fmt.Errorf("retired runtime history differs from deployment owner"))
		}
		if owner.BackendPlacements == "" {
			continue
		}
		if _, err := retainedBackendPlacementProjection(resolvedApplication{}, m, owner.BackendPlacements); err != nil {
			return nil, err
		}
	}
	return history, nil
}

func archiveReplacedRetiredRuntime(record *deployment.DeploymentRecord) error {
	history, err := retiredRuntimeHistory(*record)
	if err != nil {
		return err
	}
	old := record.Applied.GeneratedState["retired_intent"]
	if old == "" {
		return nil
	}
	candidate := retiredRuntimeOwner{Intent: json.RawMessage(old), BackendPlacements: record.Applied.GeneratedState["retired_backend_placements"]}
	for _, owner := range history {
		if string(owner.Intent) == old && owner.BackendPlacements == candidate.BackendPlacements {
			return nil
		}
	}
	history = append(history, candidate)
	data, err := json.Marshal(history)
	if err != nil {
		return invalidProviderConsumerInventory(fmt.Errorf("invalid retired runtime owner history"))
	}
	record.Applied.GeneratedState["retired_runtime_history"] = string(data)
	_, err = retiredRuntimeHistory(*record)
	return err
}
