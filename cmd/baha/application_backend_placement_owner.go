package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/deployment"
)

func desiredBackendPlacements(m application.Manifest) (map[capability.ProviderKind]capability.ProviderPlacement, error) {
	placements := map[capability.ProviderKind]capability.ProviderPlacement{}
	for _, kind := range []capability.ProviderKind{capability.ProviderPostgreSQL, capability.ProviderValkey} {
		if (kind == capability.ProviderPostgreSQL && !m.Services.SQL) || (kind == capability.ProviderValkey && !m.Services.Cache && !m.Services.KeyValue) {
			continue
		}
		placement, err := application.ResolveProviderPlacement(m, kind)
		if err != nil {
			return nil, err
		}
		placements[kind] = placement
	}
	return placements, nil
}

// Placement is realized deployment metadata, never portable application intent.
// Keep pending and applied projections in the existing authoritative owner.
func recordBackendPlacementOwner(record *deployment.DeploymentRecord, resolved resolvedApplication, applying bool) error {
	desired, err := desiredBackendPlacements(resolved.Manifest)
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(desired)
	if err != nil {
		return err
	}
	if applying && record.Applied.GeneratedState["previous_intent"] != "" && record.Applied.GeneratedState["previous_backend_placements"] == "" {
		var prior application.Manifest
		if json.Unmarshal([]byte(record.Applied.GeneratedState["previous_intent"]), &prior) != nil {
			return invalidProviderConsumerInventory(fmt.Errorf("invalid previous backend intent"))
		}
		prior, err = retainedBackendPlacementProjection(resolved, prior, record.Applied.GeneratedState["backend_placements"])
		if err != nil {
			return err
		}
		placements, err := desiredBackendPlacements(prior)
		if err != nil {
			return err
		}
		data, err := json.Marshal(placements)
		if err != nil {
			return err
		}
		record.Applied.GeneratedState["previous_backend_placements"] = string(data)
	}
	record.Applied.GeneratedState["backend_placements"] = string(encoded)
	return nil
}

func retainedBackendPlacementProjection(resolved resolvedApplication, m application.Manifest, encoded string) (application.Manifest, error) {
	if encoded == "" && !m.Services.SQL && !m.Services.Cache && !m.Services.KeyValue {
		return m, nil
	}
	placements := map[capability.ProviderKind]capability.ProviderPlacement{}
	if encoded != "" {
		if json.Unmarshal([]byte(encoded), &placements) != nil || placements == nil {
			return application.Manifest{}, invalidProviderConsumerInventory(fmt.Errorf("invalid retained backend placement owner"))
		}
	} else {
		registry, err := (capability.RegistryStore{Path: filepath.Join(resolved.TargetStateRoot, "provider-registry.json")}).Load()
		if err != nil {
			return application.Manifest{}, err
		}
		for _, binding := range registry.Bindings {
			if binding.ApplicationID != m.ApplicationID || (binding.Environment != "" && binding.Environment != m.Environment) {
				continue
			}
			for _, instance := range registry.Instances {
				kind := instance.Provider.Kind
				if instance.ID != binding.ProviderInstanceID || (kind != capability.ProviderPostgreSQL && kind != capability.ProviderValkey) {
					continue
				}
				placement := providerInstancePlacement(instance)
				if prior, exists := placements[kind]; exists && !reflect.DeepEqual(prior, placement) {
					return application.Manifest{}, invalidProviderConsumerInventory(fmt.Errorf("ambiguous retained backend placement"))
				}
				placements[kind] = placement
			}
		}
	}
	projected, err := application.WithBackendPlacementProjection(m, placements)
	if err != nil {
		return application.Manifest{}, invalidProviderConsumerInventory(err)
	}
	return projected, nil
}
