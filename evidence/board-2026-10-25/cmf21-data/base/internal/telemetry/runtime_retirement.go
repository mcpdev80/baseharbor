package telemetry

import (
	"context"
	"path/filepath"

	"github.com/mcpdev80/baseharbor/internal/providertopology"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

// traceNetwork is the retained selected consumer binding, not a network
// discovered/adopted from arbitrary native labels or a modified Compose file.
func ExistingRuntimeInventoryAt(dataDir, namespace, traceNetwork string) (ProviderFiles, []bhruntime.ProjectResource, error) {
	files, err := ExistingProviderFilesAt(dataDir, namespace)
	if err != nil {
		return files, nil, err
	}
	members, err := providertopology.ResolveMembers(files.Compose, "otel-collector", 2)
	if err != nil {
		return files, nil, err
	}
	dir := filepath.Join(files.Dir, "service-access")
	access := serviceaccess.HTTPGatewayFiles{Dir: dir, Caddyfile: filepath.Join(dir, "config", "Caddyfile"), Material: serviceaccess.TLSMaterial{ServerName: "otel-collector", ServerCertificate: filepath.Join(dir, "runtime", "server.pem")}}
	canonical := providerComposeYAMLWithTraceNetworkAndAccessForNetwork(traceNetwork, access, files.Network, members)
	resources, err := bhruntime.ExistingCanonicalProjectInventory(files.Project, files.Compose, canonical, files.Env)
	return files, resources, err
}

// Shared Core project siblings and attached consumer networks are not removal
// targets. Data/configuration/TLS and registration owners remain for re-add.
func RetireRuntimeAt(ctx context.Context, provider bhruntime.RuntimeServiceRetirementProvider, dataDir, namespace, traceNetwork string) (bhruntime.RuntimeRetirement, error) {
	files, resources, err := ExistingRuntimeInventoryAt(dataDir, namespace, traceNetwork)
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
