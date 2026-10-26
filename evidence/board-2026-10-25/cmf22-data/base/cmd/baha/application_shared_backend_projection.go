package main

import (
	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
)

func appendSharedBackendDependencyObservations(resolved resolvedApplication, observations map[string]application.ProviderFact, snapshot *application.ResolutionSnapshot) {
	for _, kind := range []capability.ProviderKind{capability.ProviderPostgreSQL, capability.ProviderValkey} {
		module := application.SharedBackendRuntimeDependency(resolved.Target.Name, kind)
		needed, existing := false, false
		for _, fact := range snapshot.Facts {
			for _, dependency := range fact.Dependencies {
				if dependency == module.ID {
					needed = true
					existing = existing || fact.State != application.ProviderNewRequired
				}
			}
		}
		if !needed {
			continue
		}
		state := application.ProviderNewRequired
		if existing {
			state = application.ProviderUnverifiable
		}
		files := application.SharedBackendFilesAt(resolved.TargetStateRoot, resolved.Target.Name, "core")
		fact := application.ProviderFact{Target: resolved.Target.Name, State: state, Core: application.CoreNotRequired, Dependencies: []string{}, Ancillary: true, OwnerReference: "runtime/" + files.Project}
		if observed, ok := observations[module.ID]; ok {
			fact = observed
		}
		snapshot.Ancillary = append(snapshot.Ancillary, module)
		snapshot.Facts[module.ID] = fact
	}
}
