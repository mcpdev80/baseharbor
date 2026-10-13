package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/identityprovider"
)

// Project actual existing owners into the shared impact model. Missing adapter
// observations remain incomplete, never an invented safe disposition.
func observeApplicationProviderImpact(ctx context.Context, resolved resolvedApplication, snapshot application.ResolutionSnapshot) application.ProviderImpactObservation {
	observation := application.ProviderImpactObservation{ObservedAt: time.Now().UTC().Format(time.RFC3339), InventoryComplete: true, Providers: map[string]application.ProviderImpactFact{}, Functions: []application.ProviderFunctionUse{}}
	instances := append([]capability.ProviderInstance{}, snapshot.Registry.Instances...)
	instances = append(instances, snapshot.Ancillary...)
	for _, instance := range instances {
		owner := "target/" + resolved.Target.Name
		if instance.Scope == capability.ScopeApplication {
			owner = instance.OwnerApplicationID
		}
		if snapshot.Facts[instance.ID].Ancillary {
			owner = snapshot.Facts[instance.ID].OwnerReference
		}
		fact := application.ProviderImpactFact{Realization: application.ProviderRealization{InstanceID: instance.ID, Kind: instance.Provider.Kind, Target: resolved.Target.Name, Scope: instance.Scope, Ownership: instance.Ownership, Owner: owner, State: snapshot.Facts[instance.ID].State, Locations: []application.ResourceLocation{}}, Materials: []application.ProviderMaterial{}, Effects: []application.OperationalConsequence{}}
		fact.Realization.Ancillary = snapshot.Facts[instance.ID].Ancillary
		observeExistingCoreProviderMaterial(resolved, instance, &fact)
		if instance.Provider.Kind == capability.ProviderKeycloak && instance.Ownership == capability.OwnershipBaseHarbor && !isRemoteApplication(resolved) {
			observeIdentityImpact(ctx, resolved, instance, &fact)
		}
		if applicationNativeBackendInstance(instance, resolved.Manifest) && !isRemoteApplication(resolved) {
			observeNativeBackendImpact(ctx, resolved, instance, &fact)
		}
		if standaloneProviderKind(instance.Provider.Kind) && !isRemoteApplication(resolved) {
			observeStandaloneProviderImpact(ctx, resolved, instance, &fact)
		}
		if !fact.InventoryComplete {
			observation.InventoryComplete = false
		}
		observation.Providers[instance.ID] = fact
	}
	// The shared model projects authoritative capability bindings itself.
	// Specific token expiry/lease behavior stays unknown until actual consumer
	// adapters supply function evidence; product features are not inferred.
	// Bind consent to the current registry, dependency observations, actual
	// inventory and file revisions. Hash input stays internal and secret-free.
	fileRevision, fileErr := applicationImpactFileRevision(observation)
	data, err := applicationImpactRevisionInput(snapshot, observation)
	if fileErr != nil {
		observation.InventoryComplete = false
		data, err = applicationImpactRevisionInput(snapshot, observation)
	}
	if err == nil {
		digest := sha256.Sum256(append(data, []byte(fileRevision)...))
		observation.Revision = hex.EncodeToString(digest[:])
	} else {
		observation.InventoryComplete = false
	}
	return observation
}

func applicationImpactRevisionInput(snapshot application.ResolutionSnapshot, observation application.ProviderImpactObservation) ([]byte, error) {
	// Sampling counters are evidence for footprint display, not changes to the
	// approved ownership, dependency or data disposition. Do not invalidate
	// consent merely because memory usage was sampled again.
	facts := make(map[string]application.ProviderFact, len(snapshot.Facts))
	for id, fact := range snapshot.Facts {
		fact.MemoryBytes, fact.Containers = application.Quantity{}, application.Quantity{}
		facts[id] = fact
	}
	observation.Revision = ""
	// Absolute validity bounds and the domain's current expired/not-yet-valid
	// consequences remain bound. A fresh countdown sample is not new authority.
	observation.ObservedAt = ""
	return json.Marshal(struct {
		Registry    capability.Registry
		Ancillary   []capability.ProviderInstance
		Facts       map[string]application.ProviderFact
		Observation application.ProviderImpactObservation
	}{snapshot.Registry, snapshot.Ancillary, facts, observation})
}

func observeIdentityImpact(ctx context.Context, resolved resolvedApplication, instance capability.ProviderInstance, fact *application.ProviderImpactFact) {
	m := resolved.Manifest
	if instance.Scope == capability.ScopeApplication {
		m = application.Manifest{Version: application.CurrentVersion, ApplicationID: instance.OwnerApplicationID, Name: instance.OwnerApplication, Environment: instance.OwnerEnvironment}
	}
	files, expected, err := identityprovider.ExistingKeycloakRuntimeInventoryAt(m, providerInstancePlacement(instance), resolved.TargetStateRoot, resolved.Target.Name)
	if err != nil {
		return
	}
	provider, err := detectRuntimeForTarget(ctx, resolved.Target)
	if err != nil {
		return
	}
	resources, err := provider.InspectProjectResources(ctx, files.Project, expected)
	if err != nil {
		return
	}
	fact.Realization.Supported, fact.Realization.Verified = true, true
	fact.Realization.Evidence = "canonical protected Identity renderer and runtime ownership inspection"
	complete := true
	for _, resource := range resources {
		if resource.Kind != "volume" {
			continue
		}
		location := application.ResourceLocation{Owner: fact.Realization.Owner, Target: resolved.Target.Name, Kind: "volume", Reference: resource.Name}
		fact.Realization.Locations = append(fact.Realization.Locations, location)
		fact.Materials = append(fact.Materials, application.ProviderMaterial{ID: instance.ID + "/volume/" + resource.Name, Kind: "database", Ownership: capability.OwnershipBaseHarbor, Location: &location, Recovery: []application.ResourceLocation{}, Verified: true, Evidence: "owned persistent Identity/SQL volume; contents and backups not inspected"})
	}
	materials, filesComplete, err := observeOwnedProviderFiles(fact.Realization.Owner, resolved.Target.Name, instance.ID, files.Dir)
	fact.Materials = append(fact.Materials, materials...)
	complete = complete && filesComplete
	// Files and volumes are observed, but live session/token behavior and
	// SQL contents/backup verification have not been projected by this adapter.
	// Do not call the entire impact inventory complete from file ownership alone.
	fact.InventoryComplete = false
	if !complete || err != nil {
		fact.Realization.Evidence += "; protected file inventory incomplete"
	}
	sort.Slice(fact.Materials, func(i, j int) bool { return fact.Materials[i].ID < fact.Materials[j].ID })
}
