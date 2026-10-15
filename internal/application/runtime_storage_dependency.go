package application

import (
	"github.com/mcpdev80/baseharbor/internal/capability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// RuntimeStorageDependencyID identifies the SQL storage actually used by the
// native OpenBao renderer, independently of app SQL resource bindings.
func RuntimeStorageDependencyID(namespace string) string {
	return "runtime/" + bhruntime.SharedProjectName(namespace) + "/control-plane/postgresql"
}

// RuntimeStorageDependency projects the existing native module owner into the
// resolver. Backing storage is not another selectable app SQL offer and is
// never persisted as a registry instance.
func RuntimeStorageDependency(namespace string) capability.ProviderInstance {
	return capability.ProviderInstance{ID: RuntimeStorageDependencyID(namespace), Provider: capability.Provider{Kind: capability.ProviderPostgreSQL}, Scope: capability.ScopeShared, Ownership: capability.OwnershipBaseHarbor}
}
