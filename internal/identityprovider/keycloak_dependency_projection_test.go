package identityprovider

import (
	"os"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
)

func TestIdentitySQLProjectionUsesActualRendererBoundaryWithoutWrites(t *testing.T) {
	root := t.TempDir() + "/not-created"
	m := application.New("one", "dev", false, false, false)
	shared := capability.ProviderPlacement{Scope: capability.ScopeShared, Ownership: capability.OwnershipBaseHarbor, SharingBoundary: "environment:dev"}
	embedded, owner, err := KeycloakSQLDependency(m, shared, root, "local", false)
	if err != nil || len(embedded.Provider.Capabilities) != 0 || owner == "" {
		t.Fatalf("embedded SQL projection: %+v %v", embedded, err)
	}
	core, _, err := KeycloakSQLDependency(m, shared, root, "local", true)
	if err != nil || core.ID != application.RuntimeStorageDependencyID("local") || core.ID == embedded.ID {
		t.Fatal("embedded versus existing native shared SQL dependency conflated")
	}
	isolated := capability.ProviderPlacement{Scope: capability.ScopeApplication, Ownership: capability.OwnershipBaseHarbor}
	appSQL, _, err := KeycloakSQLDependency(m, isolated, root, "local", true)
	if err != nil || appSQL.OwnerApplicationID != m.ApplicationID || appSQL.ID == core.ID || appSQL.ID == embedded.ID {
		t.Fatalf("app-scoped native renderer SQL ownership lost: %+v %v", appSQL, err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatal("dependency projection materialized runtime or allocated owner state")
	}
}
