package metrics

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/observability"
	"github.com/mcpdev80/baseharbor/internal/providertopology"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

// Inventory uses the retained placement and registrations without issuing
// certificates, rewriting targets or adopting an arbitrary Compose definition.
func ExistingRuntimeInventoryAt(dataDir, namespace string, m application.Manifest, placement capability.ProviderPlacement) (ProviderFiles, Placement, []bhruntime.ProjectResource, error) {
	if placement.Ownership != capability.OwnershipBaseHarbor || placement.Scope == capability.ScopeExternal {
		return ProviderFiles{}, Placement{}, nil, errors.New("Prometheus runtime owner is external or unverified")
	}
	p, err := placementFromProviderPlacementAt(dataDir, namespace, m, placement)
	if err != nil {
		return ProviderFiles{}, p, nil, err
	}
	files, err := existingProviderFilesForPlacement(p)
	if err != nil {
		return files, p, nil, err
	}
	p.Members, err = providertopology.ResolveMembers(files.Compose, "prometheus", 2)
	if err != nil {
		return files, p, nil, err
	}
	registrations := []sourceRegistration{registrationForAt(m, namespace)}
	protected := []string{files.Env}
	if p.Scope == capability.ScopeShared {
		registrations, err = readRegistrations(files.Registrations)
		if err != nil {
			return files, p, nil, err
		}
		protected = append(protected, files.Registrations)
	}
	policy, err := application.MetricsPolicy(m)
	if err != nil {
		return files, p, nil, err
	}
	apps := []string{}
	for _, registration := range registrations {
		apps = append(apps, registration.Application)
	}
	sources, retained, err := readRuntimeSourceSnapshot(files, p)
	if !retained && err == nil {
		// Legacy owners still require an exact match with the live renderer;
		// absence never authorizes adoption of an arbitrary old definition.
		sources, err = observability.ListMetrics(placement, apps, policy.Collect[application.MetricsSourceApplicationProvider], policy.Collect[application.MetricsSourcePlatformProvider])
	}
	if err != nil {
		return files, p, nil, err
	}
	if retained {
		protected = append(protected, filepath.Join(files.Dir, "runtime-sources.json"))
	}
	_, caErr := os.Stat(files.RuntimeCA)
	if caErr != nil && !errors.Is(caErr, os.ErrNotExist) {
		return files, p, nil, caErr
	}
	dir := filepath.Join(p.Dir, "service-access")
	values, err := readPrometheusEnvironment(files.Env)
	if err != nil {
		return files, p, nil, err
	}
	access := serviceaccess.HTTPGatewayFiles{Caddyfile: filepath.Join(dir, "config", "Caddyfile"), Material: serviceaccess.TLSMaterial{ServerName: "prometheus", ServerCertificate: filepath.Join(dir, "runtime", "server.pem")}}
	if values["BASEHARBOR_PROMETHEUS_HEALTH_AUTHORIZATION"] != "" {
		access.HealthAuthorizationEnv = "BASEHARBOR_PROMETHEUS_HEALTH_AUTHORIZATION"
	}
	canonical := providerComposeYAMLWithProviderNetworksAndAccess(p, registrations, providerMetricNetworks(sources), caErr == nil, hasSecureProviderMetrics(sources), access, sources)
	resources, err := bhruntime.ExistingCanonicalProjectInventory(p.Project, files.Compose, canonical, protected...)
	// Consumer networks also host workloads. They are access dependencies, not
	// exclusively owned Prometheus resources even when Compose creates them.
	consumerNetworks := map[string]bool{}
	for _, registration := range registrations {
		consumerNetworks[registration.Network] = true
	}
	owned := []bhruntime.ProjectResource{}
	for _, resource := range resources {
		if resource.Kind != "network" || !consumerNetworks[resource.Name] {
			owned = append(owned, resource)
		}
	}
	return files, p, owned, err
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
