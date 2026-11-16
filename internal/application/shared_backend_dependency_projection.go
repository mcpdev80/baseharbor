package application

import (
	"github.com/mcpdev80/baseharbor/internal/capability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// Logical environment bindings share the existing canonical physical owner.
// Project that module once into the shared resolver; it is not a registry offer.
func SharedBackendRuntimeDependency(namespace string, kind capability.ProviderKind) capability.ProviderInstance {
	return capability.ProviderInstance{ID: "runtime/" + bhruntime.SharedProjectName(namespace) + "/shared-backends/" + string(kind), Provider: capability.Provider{Kind: kind}, Scope: capability.ScopeShared, Ownership: capability.OwnershipBaseHarbor}
}
