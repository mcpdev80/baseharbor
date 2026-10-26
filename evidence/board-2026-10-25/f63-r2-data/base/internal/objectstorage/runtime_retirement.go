package objectstorage

import (
	"context"
	"errors"
	"path/filepath"

	"github.com/mcpdev80/baseharbor/internal/providertopology"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

// Observe the retained canonical owner without provisioning, issuing TLS,
// changing topology, credentials or management registrations.
func ExistingRuntimeInventoryAt(dataDir, namespace string) (ProviderFiles, []bhruntime.ProjectResource, error) {
	files, err := ExistingProviderFilesAt(dataDir, namespace)
	if err != nil {
		return files, nil, err
	}
	members, err := providertopology.ResolveMembers(files.Compose, "seaweedfs-node", 3)
	if err != nil {
		return files, nil, err
	}
	if members != 1 && (members < 3 || members%2 == 0) {
		return files, nil, errors.New("retained SeaweedFS quorum is unverifiable")
	}
	access := retainedSeaweedGatewayFiles(files.Dir, "seaweedfs")
	canonical := providerComposeYAMLWithAccessAndNetwork(access, files.Network, members)
	protected := []string{files.Env}
	management, err := managementUIRequested(files.Dir)
	if err != nil {
		return files, nil, err
	}
	if management {
		protected = append(protected, filepath.Join(files.Dir, managementUIConsumersFile))
		canonical = providerComposeWithManagementUI(canonical, retainedSeaweedGatewayFiles(filepath.Join(files.Dir, "management-ui"), "localhost"), members)
	}
	resources, err := bhruntime.ExistingCanonicalProjectInventory(files.Project, files.Compose, canonical, protected...)
	return files, resources, err
}

func retainedSeaweedGatewayFiles(providerDir, serverName string) serviceaccess.HTTPGatewayFiles {
	dir := filepath.Join(providerDir, "service-access")
	return serviceaccess.HTTPGatewayFiles{Dir: dir, Caddyfile: filepath.Join(dir, "config", "Caddyfile"), Material: serviceaccess.TLSMaterial{ServerName: serverName, ServerCertificate: filepath.Join(dir, "runtime", "server.pem")}}
}

// The common union/consent authority decides when withdrawal is requested.
// Only selected module containers are retired: shared project siblings,
// persistent volumes, networks, credentials and recovery owners remain.
func RetireRuntimeAt(ctx context.Context, provider bhruntime.RuntimeServiceRetirementProvider, dataDir, namespace string) (bhruntime.RuntimeRetirement, error) {
	files, resources, err := ExistingRuntimeInventoryAt(dataDir, namespace)
	if err != nil {
		return bhruntime.RuntimeRetirement{}, err
	}
	selected := []bhruntime.ProjectResource{}
	for _, resource := range resources {
		if resource.Kind == "container" {
			selected = append(selected, resource)
		}
	}
	return bhruntime.RetireOwnedServices(ctx, provider, files.Project, resources, selected)
}
