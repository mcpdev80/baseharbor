package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
)

// Project existing deployment and installation owners into Session 1's pure
// union plan. Other apps keep their applied intent until their own apply; an
// un-applied edit in another repository cannot release a live consumer here.
func resolveApplicationProviderPlan(resolved resolvedApplication, observations map[string]application.ProviderFact) (application.ProviderPlan, error) {
	snapshot, preferences, err := applicationLifecycleSnapshot(resolved, observations)
	if err != nil {
		return application.ProviderPlan{}, err
	}
	return resolveApplicationProviderPlanFromSnapshot(resolved, snapshot, preferences)
}

func resolveApplicationProviderPlanFromSnapshot(resolved resolvedApplication, snapshot application.ResolutionSnapshot, preferences []application.ProviderPreference) (application.ProviderPlan, error) {
	consumers := []application.ConsumerIntent{{Key: "application/" + resolved.Manifest.ApplicationID + "/" + resolved.Manifest.Environment, Active: true, Manifest: resolved.Manifest, Preferences: preferences, DependencyRoots: []string{}}}
	records, err := deployment.ListDeployments(resolved.Target.Name)
	if err != nil {
		return application.ProviderPlan{}, invalidProviderConsumerInventory(err)
	}
	for _, record := range records {
		id := record.Identity
		if id.ApplicationID == resolved.Manifest.ApplicationID && id.Environment == resolved.Manifest.Environment {
			continue
		}
		var m application.Manifest
		if len(record.Applied.Intent) == 0 || json.Unmarshal(record.Applied.Intent, &m) != nil || m.ApplicationID != id.ApplicationID || m.Name != id.Application || m.Environment != id.Environment {
			return application.ProviderPlan{}, invalidProviderConsumerInventory(fmt.Errorf("deployment %s lacks matching canonical intent", id.DeploymentID))
		}
		consumers = append(consumers, application.ConsumerIntent{Key: "deployment/" + id.DeploymentID, Active: true, Manifest: m, DependencyRoots: []string{}})
		// Another application's interrupted apply has not verified release of
		// its previous provider. Preserve both authoritative intents until that
		// application's own convergence completes.
		if previous := record.Applied.GeneratedState["previous_intent"]; previous != "" {
			var prior application.Manifest
			if json.Unmarshal([]byte(previous), &prior) != nil || prior.ApplicationID != id.ApplicationID || prior.Environment != id.Environment {
				return application.ProviderPlan{}, invalidProviderConsumerInventory(fmt.Errorf("deployment %s has invalid previous intent", id.DeploymentID))
			}
			consumers[len(consumers)-1].RetainedIntents = []application.Manifest{prior}
		}
	}
	root, err := targetRuntimeStateRoot(resolved.Target)
	if err != nil {
		return application.ProviderPlan{}, err
	}
	state, err := coreinstallation.Load(root)
	if err == nil {
		if state.Spec.Target != resolved.Target.Name || state.Spec.Runtime != resolved.Target.RuntimeProvider {
			return application.ProviderPlan{}, invalidProviderConsumerInventory(errors.New("installation binding differs from selected Target"))
		}
		// The installation remains a consumer even while partially installed.
		// Protect its actual selected modules, including Console dependencies.
		roots, err := managementProviderDependencyRoots(resolved, snapshot)
		if err != nil {
			return application.ProviderPlan{}, err
		}
		m := application.Manifest{Version: application.CurrentVersion, ApplicationID: state.ID, Name: "management", Environment: "prod"}
		consumers = append(consumers, application.ConsumerIntent{Key: "installation/" + state.ID, Active: true, Manifest: m, DependencyRoots: roots})
	} else if !errors.Is(err, os.ErrNotExist) {
		return application.ProviderPlan{}, invalidProviderConsumerInventory(err)
	} else if _, statErr := os.Lstat(filepath.Join(root, "installation.json")); statErr == nil {
		return application.ProviderPlan{}, invalidProviderConsumerInventory(err)
	}
	for index := range consumers {
		if consumers[index].Key == "installation/"+state.ID {
			continue
		}
		intents := append([]application.Manifest{consumers[index].Manifest}, consumers[index].RetainedIntents...)
		for _, intent := range intents {
			roots, rootErr := application.RuntimeDependencyRoots(intent, snapshot.Registry)
			if rootErr != nil {
				return application.ProviderPlan{}, invalidProviderConsumerInventory(rootErr)
			}
			if roots == nil {
				// Unknown consumption dependencies stay unknown for the common
				// resolver; an interrupted owner cannot make them complete.
				consumers[index].DependencyRoots = nil
				break
			}
			consumers[index].DependencyRoots = append(consumers[index].DependencyRoots, roots...)
			if application.RequiresRuntimeBroker(intent) {
				consumers[index].DependencyRoots = append(consumers[index].DependencyRoots, application.RuntimeBrokerDependency(resolved.Target.Name, intent).ID)
			}
		}
	}
	plan, err := application.ResolveProviderPlan(snapshot, consumers, true)
	return plan, classifyApplicationResolutionError(err)
}

func invalidProviderConsumerInventory(err error) error {
	return &machine.Error{Code: machine.ErrorOwnershipAmbiguous, CauseCode: "provider_consumer_inventory_invalid", Message: "Provider consumer inventory is incomplete or damaged.", Next: "Recover the existing deployment/installation state before retrying; ownership records and runtime are preserved.", Cause: err}
}
