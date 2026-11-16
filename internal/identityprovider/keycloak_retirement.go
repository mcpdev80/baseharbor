package identityprovider

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"sort"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
	"go.yaml.in/yaml/v3"
)

// ExistingKeycloakRuntimeInventoryAt reconstructs only the canonical runtime
// shape from existing protected owner state. It neither issues credentials nor
// rewrites TLS projections. The union-plan caller must prove no consumers remain.
func ExistingKeycloakRuntimeInventoryAt(app application.Manifest, placement capability.ProviderPlacement, dataDir, namespace string) (KeycloakFiles, []bhruntime.ProjectResource, error) {
	if placement.Ownership != capability.OwnershipBaseHarbor || placement.Scope == capability.ScopeExternal {
		return KeycloakFiles{}, nil, errors.New("Identity retirement requires verified BaseHarbor ownership")
	}
	dir, project, err := keycloakStateIdentity(app, placement, dataDir, namespace)
	if err != nil {
		return KeycloakFiles{}, nil, err
	}
	for _, path := range []string{dir, filepath.Join(dir, "runtime.env"), filepath.Join(dir, "compose.yaml")} {
		info, err := os.Lstat(path)
		if err != nil {
			return KeycloakFiles{}, nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 || (path == dir && !info.IsDir()) || (path != dir && !info.Mode().IsRegular()) {
			return KeycloakFiles{}, nil, errors.New("Identity runtime owner state is unprotected or noncanonical")
		}
	}
	values, err := readProtectedEnv(filepath.Join(dir, "runtime.env"))
	if err != nil {
		return KeycloakFiles{}, nil, err
	}
	sql, err := existingKeycloakCoreSQL(values)
	if err != nil {
		return KeycloakFiles{}, nil, err
	}
	app.Availability = maps.Clone(app.Availability)
	ha := values["BASEHARBOR_KEYCLOAK_TOPOLOGY"] == "ha"
	switch values["BASEHARBOR_KEYCLOAK_TOPOLOGY"] {
	case "single":
		app = application.WithAvailabilityOverride(app, "identity", &ha, 1)
	case "ha":
		app = application.WithAvailabilityOverride(app, "identity", &ha, 3)
	default:
		return KeycloakFiles{}, nil, errors.New("Identity runtime topology is unknown")
	}
	if placement.Scope == capability.ScopeShared {
		placement.SharingBoundary = "core"
		app.Name, app.Environment = "core", "prod"
	}
	consumer, err := application.IdentityProviderNetworkNameForPlacement(app, namespace, placement)
	if err != nil {
		return KeycloakFiles{}, nil, err
	}
	access := filepath.Join(dir, "service-access")
	files := KeycloakFiles{Dir: dir, Compose: filepath.Join(dir, "compose.yaml"), Env: filepath.Join(dir, "runtime.env"), Project: project, SharedSQL: sql, ConsumerNetwork: consumer, InternalNetwork: consumer + "-internal", PublicAccess: serviceaccess.HTTPGatewayFiles{Dir: access, Caddyfile: filepath.Join(access, "config", "Caddyfile"), Material: serviceaccess.TLSMaterial{ServerName: keycloakPublicHost, ServerCertificate: filepath.Join(access, "runtime", "server.pem")}}}
	expected := keycloakCompose(app, files)
	actual, err := os.ReadFile(filepath.Join(dir, "compose.yaml"))
	if err != nil || !bytes.Equal(actual, []byte(expected)) {
		return KeycloakFiles{}, nil, errors.New("Identity runtime definition differs from its canonical owner shape")
	}
	var model struct {
		Services map[string]any `yaml:"services"`
		Volumes  map[string]struct {
			Name string `yaml:"name"`
		} `yaml:"volumes"`
	}
	if err := yaml.Unmarshal([]byte(expected), &model); err != nil {
		return KeycloakFiles{}, nil, err
	}
	resources := []bhruntime.ProjectResource{{Kind: "network", Name: consumer}, {Kind: "network", Name: consumer + "-internal"}}
	for name := range model.Services {
		resources = append(resources, bhruntime.ProjectResource{Kind: "container", Name: fmt.Sprintf("%s-%s-1", project, name)})
	}
	for name, volume := range model.Volumes {
		actual := volume.Name
		if actual == "" {
			actual = project + "_" + name
		}
		resources = append(resources, bhruntime.ProjectResource{Kind: "volume", Name: actual})
	}
	sort.Slice(resources, func(i, j int) bool { return resources[i].Kind+resources[i].Name < resources[j].Kind+resources[j].Name })
	return files, resources, nil
}

// RetireKeycloakRuntime preserves realms, users, databases, TLS/recovery state
// and credentials. Re-add uses the same existing realization and authority.
func RetireKeycloakRuntime(ctx context.Context, provider bhruntime.RuntimeRetirementProvider, app application.Manifest, placement capability.ProviderPlacement, dataDir, namespace string) (bhruntime.RuntimeRetirement, error) {
	files, resources, err := ExistingKeycloakRuntimeInventoryAt(app, placement, dataDir, namespace)
	if err != nil {
		return bhruntime.RuntimeRetirement{}, err
	}
	native, ok := provider.(bhruntime.RuntimeServiceRetirementProvider)
	if !ok {
		return bhruntime.RuntimeRetirement{}, errors.New("Identity retirement requires a data-preserving selected-container runtime adapter")
	}
	selected := []bhruntime.ProjectResource{}
	for _, resource := range resources {
		if resource.Kind == "container" {
			selected = append(selected, resource)
		}
	}
	// Shared modules use the same project. Retiring this realization must never
	// bring down sibling SQL/cache/control-plane or workload services.
	return bhruntime.RetireOwnedServices(ctx, native, files.Project, resources, selected)
}
