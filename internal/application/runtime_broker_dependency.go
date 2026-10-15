package application

import (
	"github.com/mcpdev80/baseharbor/internal/capability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// RuntimeBrokerDependency projects the existing per-application native owner.
// It creates neither a selectable Capability nor a persisted registry entry.
func RuntimeBrokerDependency(namespace string, m Manifest) capability.ProviderInstance {
	return capability.ProviderInstance{ID: "runtime/" + bhruntime.ApplicationProjectName(namespace, m.Name, m.Environment) + "/broker", Provider: capability.Provider{Kind: capability.ProviderRuntimeBroker}, Scope: capability.ScopeApplication, Ownership: capability.OwnershipBaseHarbor, OwnerApplicationID: m.ApplicationID, OwnerApplication: m.Name, OwnerEnvironment: m.Environment}
}
