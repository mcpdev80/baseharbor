package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/delivery"
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
	if retained := record.Applied.GeneratedState["backend_placements"]; retained != "" {
		var prior map[capability.ProviderKind]capability.ProviderPlacement
		if json.Unmarshal([]byte(retained), &prior) != nil || prior == nil {
			return invalidProviderConsumerInventory(fmt.Errorf("invalid retained backend selection"))
		}
		if _, err := application.WithBackendPlacementProjection(resolved.Manifest, prior); err != nil {
			return invalidProviderConsumerInventory(err)
		}
		for kind, placement := range prior {
			if _, selected := desired[kind]; !selected {
				desired[kind] = placement // Disabled capability retains its selection.
			}
		}
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
		// Replacing an application backend preserves its original definition and
		// data. Later shared-consumer withdrawal must still be able to inspect
		// that canonical owner after previous_intent has converged away.
		for kind, placement := range placements {
			if placement.Scope == capability.ScopeApplication && desired[kind].Scope != capability.ScopeApplication {
				owner := retiredRuntimeOwner{Intent: json.RawMessage(record.Applied.GeneratedState["previous_intent"]), BackendPlacements: string(data)}
				if err := archiveRetainedRuntimeOwner(record, owner); err != nil {
					return err
				}
				break
			}
		}
	}
	record.Applied.GeneratedState["backend_placements"] = string(encoded)
	// One-writer guard (ADR 0022 §2): make SaveDeploymentRecord's apply-path
	// guard observe a real, authoritative per-resource owner instead of an
	// absent field. recordBackendPlacementOwner is the card's owner-wiring
	// point and the only place the persisted deployment record is read for
	// this deployment, so it is the authoritative observation source.
	if err := observeReconciliationOwner(record, resolved); err != nil {
		return err
	}
	return nil
}

// observeReconciliationOwner records the current reconciliation owner on the
// record's Observed block so internal/deployment refuses the mutation through
// SaveDeploymentRecord's one-writer guard instead of silently accepting it.
//
// The persisted record's previous reconciliation owner is authoritative: it is
// the real external/baseharbor owner observed before this apply. It wins over
// the caller-supplied (attacker-controllable) client value. When no prior
// record exists, the applied delivery mode's owner is the fallback. A direct
// (BaseHarbor-owned) apply that meets a persisted external owner is flagged as
// conflicting so the guard fails closed.
func observeReconciliationOwner(record *deployment.DeploymentRecord, resolved resolvedApplication) error {
	prior, loadErr := deployment.LoadDeploymentRecord(resolved.DeploymentIdentity)
	hasPrior := loadErr == nil
	if !hasPrior && loadErr != nil && !errors.Is(loadErr, os.ErrNotExist) {
		return loadErr
	}

	mode := record.Applied.Delivery.Mode
	var owner delivery.ReconciliationOwner
	if mode == delivery.ModeDirect {
		owner = delivery.OwnerBaseHarbor
	} else if mode == delivery.ModeDelegated {
		owner = delivery.OwnerExternal
	}
	// Persisted authoritative owner wins over the applied mode's owner.
	if hasPrior && prior.Observed.Owner != "" {
		owner = prior.Observed.Owner
	}

	record.Observed.Owner = owner
	// Direct mutation while an external reconciler already owns the resource:
	// the guard refuses it. Absent prior observation is the normal direct happy
	// path and passes.
	if mode == delivery.ModeDirect && hasPrior && prior.Observed.Owner == delivery.OwnerExternal {
		record.Observed.Conflict = true
	}
	return nil
}

// Automatic/default selection reuses the deployment's chosen realization,
// including interruption or a disabled capability. Explicit operator policy is
// the existing replacement mechanism; no additional portable intent field.
func resolveAppliedBackendSelection(resolved resolvedApplication) (application.Manifest, error) {
	// Preserve applied-owner observations; layer confirmed desired choices only
	// on the execution projection returned to this caller.
	projectDesired := func(m application.Manifest) (application.Manifest, error) {
		return application.WithDesiredProviderPlacementsAt(resolved.TargetStateRoot, m)
	}
	m := resolved.Manifest
	if resolved.TargetStateRoot == "" || (!m.Services.SQL && !m.Services.Cache && !m.Services.KeyValue) {
		return projectDesired(m)
	}
	encoded := ""
	record, err := deployment.LoadDeploymentRecord(resolved.DeploymentIdentity)
	if err == nil {
		encoded = record.Applied.GeneratedState["backend_placements"]
	} else if !errors.Is(err, os.ErrNotExist) {
		return application.Manifest{}, err
	}
	applied, err := retainedBackendPlacementProjection(resolved, m, encoded)
	if err != nil {
		return application.Manifest{}, err
	}
	placements := map[capability.ProviderKind]capability.ProviderPlacement{}
	for _, kind := range []capability.ProviderKind{capability.ProviderPostgreSQL, capability.ProviderValkey} {
		if (kind == capability.ProviderPostgreSQL && !m.Services.SQL) || (kind == capability.ProviderValkey && !m.Services.Cache && !m.Services.KeyValue) {
			continue
		}
		explicit, err := application.HasExplicitProviderPlacement(m, kind)
		if err != nil {
			return application.Manifest{}, err
		}
		placement, err := application.ResolveProviderPlacement(applied, kind)
		if explicit {
			placement, err = application.ResolveConfiguredProviderPlacement(m, kind)
		}
		if err != nil {
			return application.Manifest{}, err
		}
		placements[kind] = placement
	}
	if len(placements) == 0 {
		return projectDesired(m)
	}
	projected, err := application.WithBackendPlacementProjection(m, placements)
	if err != nil {
		return application.Manifest{}, err
	}
	return projectDesired(projected)
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
