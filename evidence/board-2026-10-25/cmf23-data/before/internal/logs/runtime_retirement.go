package logs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/objectstorage"
	"github.com/mcpdev80/baseharbor/internal/providertopology"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

func ExistingRuntimeInventoryAt(dataDir, namespace string, m application.Manifest, placement capability.ProviderPlacement, mode bhruntime.LogCollectionMode) (ProviderFiles, Placement, []bhruntime.ProjectResource, error) {
	if placement.Ownership != capability.OwnershipBaseHarbor || placement.Scope == capability.ScopeExternal {
		return ProviderFiles{}, Placement{}, nil, errors.New("Loki runtime owner is external or unverified")
	}
	if mode != bhruntime.LogCollectionSyslog && mode != bhruntime.LogCollectionJournald {
		return ProviderFiles{}, Placement{}, nil, errors.New("retained Loki collection mode is unknown")
	}
	p, err := retainedPlacementIdentityAt(dataDir, namespace, m, placement)
	if err != nil {
		return ProviderFiles{}, p, nil, err
	}
	files := providerFiles(p)
	registrations, err := readRegistrations(files.Registrations)
	if err != nil {
		return files, p, nil, err
	}
	services, err := providertopology.ServiceNames(files.Compose)
	if err != nil {
		return files, p, nil, err
	}
	storageNetwork := ""
	if _, ha := services["loki-1"]; ha {
		storage, err := objectstorage.ExistingProviderFilesAt(dataDir, namespace)
		if err != nil {
			return files, p, nil, err
		}
		storageNetwork = storage.Network
	}
	values, err := os.ReadFile(files.Env)
	if err != nil {
		return files, p, nil, err
	}
	platformPort := 0
	for _, line := range strings.Split(string(values), "\n") {
		if strings.HasPrefix(line, "BASEHARBOR_PLATFORM_PROVIDER_SYSLOG_PORT=") {
			platformPort, err = persistedPort(files.Env, "BASEHARBOR_PLATFORM_PROVIDER_SYSLOG_PORT")
			if err != nil {
				return files, p, nil, err
			}
		}
	}
	dir := filepath.Join(p.Dir, "service-access")
	access := serviceaccess.HTTPGatewayFiles{Caddyfile: filepath.Join(dir, "config", "Caddyfile"), Material: serviceaccess.TLSMaterial{ServerName: "localhost", ServerCertificate: filepath.Join(dir, "runtime", "server.pem")}}
	canonical := providerComposeYAMLForModeAndAccess(p, registrations, mode, access, platformPort, storageNetwork)
	resources, err := bhruntime.ExistingCanonicalProjectInventory(p.Project, files.Compose, canonical, files.Env, files.Registrations)
	return files, p, resources, err
}

func RetireRuntimeAt(ctx context.Context, provider bhruntime.RuntimeServiceRetirementProvider, dataDir, namespace string, m application.Manifest, placement capability.ProviderPlacement, mode bhruntime.LogCollectionMode) (bhruntime.RuntimeRetirement, error) {
	_, p, resources, err := ExistingRuntimeInventoryAt(dataDir, namespace, m, placement, mode)
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
