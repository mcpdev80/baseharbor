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
	add := func(owner resolvedApplication) {
		intent, _ := json.Marshal(owner.Manifest)
		key := owner.Store.Root + "/" + owner.Manifest.Name + "/" + owner.Manifest.Environment + "/" + string(intent)
		owners[key] = owner
	}
	add(resolved)
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
			if err := m.Validate(); err != nil {
				return nil, invalidProviderConsumerInventory(err)
			}
			owner := resolved
			owner.Manifest = m
			owner.Store = application.Store{Root: filepath.Join(root, "state"), Namespace: resolved.Target.Name}
			owner.DeploymentIdentity = id
			owner.DeploymentRecord = &record
			owner.DeploymentStateRoot = root
			add(owner)
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
