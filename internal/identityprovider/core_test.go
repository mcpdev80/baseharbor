package identityprovider

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
)

func TestCoreIdentityExistingDiscoveryRetainsHTTPSPort(t *testing.T) {
	root := t.TempDir()
	spec := application.Manifest{Version: application.CurrentVersion, Name: "core", Environment: "prod", Services: application.Services{Identity: true}}
	placement := capability.ProviderPlacement{Scope: capability.ScopeShared, Ownership: capability.OwnershipBaseHarbor, SharingBoundary: "core"}
	files, err := ensureKeycloakFilesForPlacement(context.Background(), spec, serviceissuer.New(t), root, "local", placement)
	if err != nil {
		t.Fatal(err)
	}
	existing, err := existingCoreKeycloakFiles(files.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if existing.PublicPort != files.PublicPort || existing.AdminPort != files.AdminPort || existing.PublicURL != files.PublicURL || existing.AdminURL != files.AdminURL {
		t.Fatalf("readiness targets a different authority: existing=%+v provisioned=%+v", existing, files)
	}
	if files.Dir != filepath.Join(root, "providers", "keycloak", "shared", "core") {
		t.Fatalf("Core Identity lost installation placement: %s", files.Dir)
	}
	if files.Project == bhruntime.SharedProjectName("local") || files.Project != bhruntime.SharedProjectName("local-core") {
		t.Fatal("application provider reconciliation can replace Core Identity containers")
	}
}
