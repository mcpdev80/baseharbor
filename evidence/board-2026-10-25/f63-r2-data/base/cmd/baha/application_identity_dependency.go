package main

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/identityprovider"
)

func appendIdentitySQLDependency(resolved resolvedApplication, instance capability.ProviderInstance, fact application.ProviderFact, observations map[string]application.ProviderFact, snapshot *application.ResolutionSnapshot) (application.ProviderFact, error) {
	m := resolved.Manifest
	if instance.Scope == capability.ScopeApplication {
		m = application.Manifest{Version: application.CurrentVersion, ApplicationID: instance.OwnerApplicationID, Name: instance.OwnerApplication, Environment: instance.OwnerEnvironment}
	}
	coreSQL := false
	if instance.Scope == capability.ScopeShared {
		if _, err := os.Lstat(filepath.Join(resolved.TargetStateRoot, "service-access", "issuer")); errors.Is(err, os.ErrNotExist) {
			_, coreErr := os.Lstat(filepath.Join(resolved.TargetStateRoot, "runtime"))
			coreSQL = coreErr == nil || application.RequiresRuntimeBroker(resolved.Manifest)
		}
	}
	for _, dep := range fact.Dependencies {
		coreSQL = coreSQL || dep == application.RuntimeStorageDependencyID(resolved.Target.Name)
	}
	module, owner, err := identityprovider.KeycloakSQLDependency(m, providerInstancePlacement(instance), resolved.TargetStateRoot, resolved.Target.Name, coreSQL)
	if err != nil {
		return fact, err
	}
	fact.Dependencies = []string{module.ID}
	if coreSQL {
		fact.Core = application.CoreRequired
		return fact, nil
	}
	for _, existing := range snapshot.Ancillary {
		if existing.ID == module.ID {
			return fact, nil
		}
	}
	snapshot.Ancillary = append(snapshot.Ancillary, module)
	sqlFact := application.ProviderFact{Target: resolved.Target.Name, State: fact.State, Core: application.CoreNotRequired, Dependencies: []string{}, Ancillary: true, OwnerReference: owner}
	if observation, ok := observations[module.ID]; ok {
		sqlFact = observation
	}
	snapshot.Facts[module.ID] = sqlFact
	return fact, nil
}
