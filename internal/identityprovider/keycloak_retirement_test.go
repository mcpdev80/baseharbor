package identityprovider

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

type keycloakRetirementRuntime struct {
	removed  bool
	down     bool
	selected []bhruntime.ProjectResource
}

func (r *keycloakRetirementRuntime) InspectProjectResources(_ context.Context, _ string, expected []bhruntime.ProjectResource) ([]bhruntime.ProjectResource, error) {
	var observed []bhruntime.ProjectResource
	for _, resource := range expected {
		if !r.removed || resource.Kind != "container" {
			observed = append(observed, resource)
		}
	}
	return observed, nil
}

func (r *keycloakRetirementRuntime) DownProject(context.Context, string, string, string) error {
	r.down = true
	return nil
}

func (r *keycloakRetirementRuntime) ListRuntimeContainers(context.Context) ([]bhruntime.RuntimeContainer, error) {
	return nil, nil
}
func (r *keycloakRetirementRuntime) RetireOwnedProjectContainers(_ context.Context, _ string, selected []bhruntime.ProjectResource) error {
	for _, resource := range selected {
		if resource.Kind != "container" {
			return bhruntime.ErrResourceOwnership
		}
	}
	r.selected = append([]bhruntime.ProjectResource{}, selected...)
	r.removed = true
	return nil
}

func TestStandaloneIdentityRetirementAndReAddPreserveAuthorityAndCredentials(t *testing.T) {
	for _, ha := range []bool{false, true} {
		t.Run(map[bool]string{false: "single", true: "ha"}[ha], func(t *testing.T) {
			root := t.TempDir()
			issuer, err := serviceaccess.EnsureLocalIssuer(filepath.Join(root, "authority"))
			if err != nil {
				t.Fatal(err)
			}
			app := application.WithHA(application.New("demo", "prod", false, false, false), ha)
			app.Services.Identity = true
			placement, err := application.ResolveProviderPlacement(app, capability.ProviderKeycloak)
			if err != nil {
				t.Fatal(err)
			}
			files, err := EnsureKeycloakFilesAt(context.Background(), app, issuer, root, "isolated")
			if err != nil {
				t.Fatal(err)
			}
			if files.SharedSQL != nil {
				t.Fatal("standalone Identity acquired Core SQL")
			}
			before, err := os.ReadFile(files.Env)
			if err != nil {
				t.Fatal(err)
			}
			_, inventory, err := ExistingKeycloakRuntimeInventoryAt(app, placement, root, "isolated")
			if err != nil {
				t.Fatal(err)
			}
			runtime := &keycloakRetirementRuntime{}
			result, err := RetireKeycloakRuntime(context.Background(), runtime, app, placement, root, "isolated")
			if err != nil || len(result.Retained) < 2 || len(result.Removed) == 0 {
				t.Fatalf("retirement did not preserve Identity data: %+v %v", result, err)
			}
			if runtime.down || len(runtime.selected) == 0 {
				t.Fatal("Identity retirement used whole-project shutdown or omitted selection")
			}
			if len(inventory) == 0 {
				t.Fatal("missing actual module inventory")
			}
			again, err := EnsureKeycloakFilesAt(context.Background(), app, issuer, root, "isolated")
			if err != nil {
				t.Fatal(err)
			}
			after, err := os.ReadFile(again.Env)
			if err != nil || string(before) != string(after) {
				t.Fatal("re-add replaced protected Identity credentials", err)
			}
			if err := os.WriteFile(files.Compose, []byte("services:\n  foreign: {}\n"), 0600); err != nil {
				t.Fatal(err)
			}
			runtime.removed = false
			if _, err := RetireKeycloakRuntime(context.Background(), runtime, app, placement, root, "isolated"); err == nil || runtime.removed {
				t.Fatal("changed runtime definition authorized removal")
			}
		})
	}
}
