package artifact

import (
	"fmt"
	runtimemodel "github.com/mcpdev80/baseharbor/internal/runtime/model"
	"github.com/mcpdev80/baseharbor/internal/workload"
	"sort"
	"strings"
)

// RuntimePlan projects already resolved OCI artifacts into the existing runtime
// contract. Source/build inputs and overridden repository defaults never cross
// the runtime boundary. Secret bindings remain opaque references.
func RuntimePlan(application, environment, scope string, source workload.Model, images map[string]string, bindings []runtimemodel.Binding) (runtimemodel.WorkloadPlan, error) {
	plan := runtimemodel.WorkloadPlan{Application: application, Environment: environment, Target: runtimemodel.Target{Scope: scope}, Bindings: append([]runtimemodel.Binding(nil), bindings...)}
	bound := map[string]bool{}
	for _, binding := range bindings {
		bound[binding.Name] = true
	}
	for _, service := range source.Services {
		image := strings.TrimSpace(images[service.Name])
		if image == "" {
			image = strings.TrimSpace(service.Image)
		}
		if image == "" {
			return plan, fmt.Errorf("workload service %q: resolve it to an OCI image artifact before runtime projection", service.Name)
		}
		projected := runtimemodel.Service{Name: service.Name, Image: image, Args: append([]string(nil), service.Args...)}
		keys := make([]string, 0, len(service.Environment))
		for key := range service.Environment {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			if bound[key] {
				continue
			}
			value := service.Environment[key]
			if strings.Contains(value, "${") {
				return plan, fmt.Errorf("workload environment %s requires a resolved runtime binding", key)
			}
			projected.Environment = append(projected.Environment, runtimemodel.EnvironmentVariable{Name: key, Value: value})
		}
		for _, port := range service.Ports {
			projected.Ports = append(projected.Ports, runtimemodel.ContainerPort{Container: port.Container, Protocol: port.Protocol})
		}
		plan.Services = append(plan.Services, projected)
	}
	return plan, plan.Validate()
}
