package runtimebroker

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// RetireContainer removes only the broker declared by the existing application
// owner. Workloads and native backends share its project, so project-wide Down
// is not a broker lifecycle operation. The native adapter verifies ownership
// and never receives a volume or network deletion target.
func RetireContainer(ctx context.Context, provider bhruntime.RuntimeProvider, m application.Manifest, files application.RuntimeFiles) (bhruntime.RuntimeRetirement, error) {
	result := bhruntime.RuntimeRetirement{Removed: []bhruntime.ProjectResource{}, Retained: []bhruntime.RetainedRuntimeResource{}}
	if !application.RequiresRuntimeBroker(m) {
		return result, errors.New("broker retirement requires its previously declared application intent")
	}
	native, ok := provider.(bhruntime.RuntimeServiceRetirementProvider)
	if !ok {
		return result, errors.New("runtime has no data-preserving broker container retirement adapter")
	}
	project := ProjectNameForRuntime(m, files)
	container := bhruntime.ProjectResource{Kind: "container", Name: project + "-" + ServiceName + "-1"}
	expected, err := OwnedRuntimeResources(m, files)
	if err != nil {
		return result, err
	}
	return bhruntime.RetireOwnedServices(ctx, native, project, expected, []bhruntime.ProjectResource{container})
}

// OwnedRuntimeResources uses the names of the existing broker renderer. Shared
// secrets/control/telemetry networks and application-owned backend networks
// are external to this owner and cannot become broker deletion targets.
func OwnedRuntimeResources(m application.Manifest, files application.RuntimeFiles) ([]bhruntime.ProjectResource, error) {
	project := ProjectNameForRuntime(m, files)
	resources := []bhruntime.ProjectResource{{Kind: "container", Name: project + "-" + ServiceName + "-1"}}
	if len(m.Runtime.Permissions) > 0 {
		resources = append(resources, bhruntime.ProjectResource{Kind: "volume", Name: project + "_runtime-operations"})
	}
	if application.HasRuntimeMetricsPermissions(m) {
		resources = append(resources, bhruntime.ProjectResource{Kind: "volume", Name: application.MetricsRuntimeTargetVolumeNameForNamespace(m, files.Namespace)})
	}
	if !application.HasApplicationScopedRuntimeServices(m) {
		name := application.ApplicationBackendNetworkName(m)
		resourceProject := strings.TrimSpace(files.ResourceProject)
		if resourceProject == "" {
			resourceProject = application.RuntimeProjectName(m)
		}
		if scoped := application.ApplicationBackendNetworkNameForProject(resourceProject); scoped != "" {
			name = scoped
		}
		resources = append(resources, bhruntime.ProjectResource{Kind: "network", Name: name})
	}
	policy, err := application.MetricsPolicy(m)
	if err != nil {
		return nil, err
	}
	if policy.Enabled && policy.Collect[application.MetricsSourceApplicationProvider] {
		resources = append(resources, bhruntime.ProjectResource{Kind: "network", Name: ObservabilityNetworkNameForRuntime(m, files)})
	}
	sort.Slice(resources, func(i, j int) bool { return resources[i].Kind+resources[i].Name < resources[j].Kind+resources[j].Name })
	return resources, nil
}
