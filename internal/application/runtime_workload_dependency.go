package application

import (
	"fmt"

	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/stableid"
)

// RuntimeWorkloadDependency projects one declared component's existing owner.
// This read-only declaration does not provision runtime services or claim
// native ownership, readiness, retention or downstream function observations.
func RuntimeWorkloadDependency(namespace string, m Manifest, component string) (capability.ProviderInstance, error) {
	if err := m.Validate(); err != nil {
		return capability.ProviderInstance{}, err
	}
	if err := stableid.ValidateUUIDv4("application", m.ApplicationID); err != nil {
		return capability.ProviderInstance{}, err
	}
	for _, declared := range WorkloadComponentNames(m) {
		if declared == component {
			project := WorkloadProjectNameForRuntime(m, RuntimeFilesFor(Store{Namespace: namespace}, m))
			return capability.ProviderInstance{ID: "runtime/" + project + "/workload/" + component, Provider: capability.Provider{Kind: capability.ProviderRuntimeWorkload}, Scope: capability.ScopeApplication, Ownership: capability.OwnershipBaseHarbor, OwnerApplicationID: m.ApplicationID, OwnerApplication: m.Name, OwnerEnvironment: m.Environment}, nil
		}
	}
	return capability.ProviderInstance{}, fmt.Errorf("runtime workload component is not declared by its application owner")
}
