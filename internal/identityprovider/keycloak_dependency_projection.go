package identityprovider

import (
	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"path/filepath"
)

// KeycloakSQLDependency projects the SQL module selected by the existing native
// renderer. It offers no application capability and allocates no identity or
// runtime state. A caller supplies the already selected issuer/storage choice.
func KeycloakSQLDependency(app application.Manifest, placement capability.ProviderPlacement, dataDir, namespace string, coreSQL bool) (capability.ProviderInstance, string, error) {
	if placement.Scope == capability.ScopeShared && coreSQL {
		module := application.RuntimeStorageDependency(namespace)
		return module, filepath.Join(dataDir, "runtime"), nil
	}
	dir, project, err := keycloakStateIdentity(app, placement, dataDir, namespace)
	if err != nil {
		return capability.ProviderInstance{}, "", err
	}
	module := capability.ProviderInstance{ID: "runtime/" + project + "/keycloak/postgresql", Provider: capability.Provider{Kind: capability.ProviderPostgreSQL}, Scope: placement.Scope, Ownership: capability.OwnershipBaseHarbor}
	if placement.Scope == capability.ScopeApplication {
		module.OwnerApplicationID, module.OwnerApplication, module.OwnerEnvironment = app.ApplicationID, app.Name, app.Environment
	}
	return module, dir, nil
}
