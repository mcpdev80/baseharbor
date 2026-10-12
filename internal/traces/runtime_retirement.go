package traces

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/objectstorage"
	"github.com/mcpdev80/baseharbor/internal/providertopology"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

func ExistingRuntimeInventoryAt(dataDir, namespace string, m application.Manifest, placement capability.ProviderPlacement) (ProviderFiles, Placement, []bhruntime.ProjectResource, error) {
	files, p, resources, _, err := existingRuntimeInventoryAt(dataDir, namespace, m, placement)
	return files, p, resources, err
}

// Read the existing protected renderer. A retained previous image is a planned
// provider update, never an implicit change authorized by a generic apply.
func ExistingRuntimeUpgradeRequiredAt(dataDir, namespace string, m application.Manifest, placement capability.ProviderPlacement) (bool, error) {
	_, _, _, previous, err := existingRuntimeInventoryAt(dataDir, namespace, m, placement)
	return previous, err
}

func existingRuntimeInventoryAt(dataDir, namespace string, m application.Manifest, placement capability.ProviderPlacement) (ProviderFiles, Placement, []bhruntime.ProjectResource, bool, error) {
	if placement.Ownership != capability.OwnershipBaseHarbor || placement.Scope == capability.ScopeExternal {
		return ProviderFiles{}, Placement{}, nil, false, errors.New("Tempo runtime owner is external or unverified")
	}
	p, err := retainedPlacementIdentityAt(dataDir, namespace, m, placement)
	if err != nil {
		return ProviderFiles{}, p, nil, false, err
	}
	files := ProviderFiles{Dir: p.Dir, Compose: filepath.Join(p.Dir, "compose.yaml"), Env: filepath.Join(p.Dir, "runtime.env"), Config: filepath.Join(p.Dir, "tempo.yaml")}
	services, err := providertopology.ServiceNames(files.Compose)
	if err != nil {
		return files, p, nil, false, err
	}
	dir := filepath.Join(files.Dir, "service-access")
	access := serviceaccess.HTTPGatewayFiles{Dir: dir, Caddyfile: filepath.Join(dir, "config", "Caddyfile"), Material: serviceaccess.TLSMaterial{ServerName: "localhost", ServerCertificate: filepath.Join(dir, "runtime", "server.pem")}}
	canonical := composeYAMLWithAccess(p, access)
	if _, ha := services["tempo-distributor-1"]; ha {
		storage, err := objectstorage.ExistingProviderFilesAt(dataDir, namespace)
		if err != nil {
			return files, p, nil, false, err
		}
		canonical = tempoHACompose(p, access, objectstorage.PlatformBucket{Network: storage.Network})
	}
	previous := false
	resources, err := bhruntime.ExistingCanonicalProjectInventory(p.Project, files.Compose, canonical, files.Env)
	if err != nil {
		// Recognize only the exact previously shipped renderer. This is read-only
		// retirement authority, never an implicit upgrade or arbitrary image adoption.
		legacy := strings.ReplaceAll(canonical, "image: "+ProviderImage+"\n", "image: docker.io/grafana/tempo:3.0.2\n")
		resources, err = bhruntime.ExistingCanonicalProjectInventory(p.Project, files.Compose, legacy, files.Env)
		previous = err == nil
	}
	return files, p, resources, previous, err
}

func RetireRuntimeAt(ctx context.Context, provider bhruntime.RuntimeServiceRetirementProvider, dataDir, namespace string, m application.Manifest, placement capability.ProviderPlacement) (bhruntime.RuntimeRetirement, error) {
	_, p, resources, err := ExistingRuntimeInventoryAt(dataDir, namespace, m, placement)
	if err != nil {
		return bhruntime.RuntimeRetirement{}, err
	}
	selected := []bhruntime.ProjectResource{}
	for _, resource := range resources {
		if resource.Kind == "container" {
			selected = append(selected, resource)
		}
	}
	return bhruntime.RetireOwnedServices(ctx, provider, p.Project, resources, selected)
}
