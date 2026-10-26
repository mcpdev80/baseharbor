package application

import (
	"github.com/mcpdev80/baseharbor/internal/capability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// RuntimeExecutorDependency identifies the single existing physical executor on
// a Target. All application/management roots reference this same shared owner;
// it is neither an application alias nor a selectable Capability provider.
// The caller must separately observe ownership, dependencies and runtime facts.
func RuntimeExecutorDependency(namespace string) capability.ProviderInstance {
	return capability.ProviderInstance{ID: "runtime/" + bhruntime.SharedProjectName(namespace) + "/runtime-executor", Provider: capability.Provider{Kind: capability.ProviderRuntimeExecutor}, Scope: capability.ScopeShared, Ownership: capability.OwnershipBaseHarbor}
}
