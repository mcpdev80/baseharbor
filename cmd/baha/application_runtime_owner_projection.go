package main

import (
	"encoding/json"
	"fmt"
	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"path/filepath"
	"sort"
)

// Observe existing canonical owners, never a sibling through the current Store.
func applicationRuntimeOwners(resolved resolvedApplication) ([]resolvedApplication, error) {
	owners := map[string]resolvedApplication{}
	add := func(owner resolvedApplication) error {
		intent, _ := json.Marshal(owner.Manifest)
		placements, err := desiredBackendPlacements(owner.Manifest)
		if err != nil {
			return err
		}
		projection, _ := json.Marshal(placements)
		key := owner.Store.Root + "/" + owner.Manifest.Name + "/" + owner.Manifest.Environment + "/" + string(intent) + "/" + string(projection)
		owners[key] = owner
		return nil
	}
	if err := add(resolved); err != nil {
		return nil, err
	}
	records, err := deployment.ListDeployments(resolved.Target.Name)
	if err != nil {
		return nil, invalidProviderConsumerInventory(err)
	}
	for _, record := range records {
		root, err := deployment.DeploymentRoot(record.Identity)
		if err != nil {
			return nil, invalidProviderConsumerInventory(err)
		}
		intents := []json.RawMessage{record.Applied.Intent}
		if previous := record.Applied.GeneratedState["previous_intent"]; previous != "" {
			intents = append(intents, json.RawMessage(previous))
		}
		if retained := record.Applied.GeneratedState["retired_intent"]; retained != "" {
			intents = append(intents, json.RawMessage(retained))
		}
		for index, intent := range intents {
			var m application.Manifest
			id := record.Identity
			if len(intent) == 0 || json.Unmarshal(intent, &m) != nil || m.ApplicationID != id.ApplicationID || m.Environment != id.Environment || (index == 0 && m.Name != id.Application) {
				return nil, invalidProviderConsumerInventory(fmt.Errorf("runtime intent differs from deployment owner"))
			}
			if err := m.ValidateIntent(); err != nil {
				return nil, invalidProviderConsumerInventory(err)
			}
			owner := resolved
			placementKey := "backend_placements"
			if index > 0 && string(intent) == record.Applied.GeneratedState["previous_intent"] {
				placementKey = "previous_backend_placements"
			} else if index > 0 {
				placementKey = "retired_backend_placements"
			}
			m, err = retainedBackendPlacementProjection(resolved, m, record.Applied.GeneratedState[placementKey])
			if err != nil {
				return nil, err
			}
			owner.Manifest = m
			owner.Store = application.Store{Root: filepath.Join(root, "state"), Namespace: resolved.Target.Name}
			owner.DeploymentIdentity = id
			owner.DeploymentRecord = &record
			owner.DeploymentStateRoot = root
			if err := add(owner); err != nil {
				return nil, err
			}
		}
	}
	keys := []string{}
	for key := range owners {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := []resolvedApplication{}
	for _, key := range keys {
		result = append(result, owners[key])
	}
	return result, nil
}
