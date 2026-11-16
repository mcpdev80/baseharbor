package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

// ContainerMatchesProjectResource preserves the existing native naming
// conventions: Compose has ordinal -1; first-party Quadlet uses project-service.
// Both require the actual project and service labels, not a name prefix.
func ContainerMatchesProjectResource(kind ProviderKind, container RuntimeContainer, project string, resource ProjectResource) bool {
	if resource.Kind != "container" || container.Project != project || container.Service == "" || resource.Name != project+"-"+container.Service+"-1" {
		return false
	}
	return container.Name == resource.Name || kind == ProviderPodman && container.Name == project+"-"+container.Service
}

// RuntimeConfigProvider exposes the rendered Compose model so the quiesce path
// can derive the planned native container names (container_name /
// deploy.replicas per service) instead of assuming the default
// <project>-<service>-1 scheme. Compose implements it; a provider that cannot
// render the model keeps the single canonical ordinal fallback.
type RuntimeConfigProvider interface {
	ConfigJSONProjectFilesEnv(context.Context, string, string, map[string]string, ...string) (string, error)
}

// expectedContainerNameSet returns the exact set of native container names the
// deployment plans for one service: the default <project>-<service>-1..-N
// ordinal names when the workload leaves container_name unset (compose scales
// with deploy.replicas), or the single pinned container_name when the workload
// pins one (compose rejects container_name together with replicas, so a pinned
// service is a single container). The rendered Compose model is the source of
// truth, never the default scheme alone, because a pinned container_name lives
// outside <project>-<service>-1 and every quiesce identity check must accept it.
func expectedContainerNameSet(ctx context.Context, provider RuntimeConfigProvider, workdir, project, service string, composeFiles []string) ([]string, error) {
	if provider == nil {
		return nil, errors.New("expected container names require a render-capable provider")
	}
	rendered, err := provider.ConfigJSONProjectFilesEnv(ctx, project, workdir, nil, composeFiles...)
	if err != nil {
		return nil, err
	}
	names, err := parsedExpectedContainerNames(rendered, project, service)
	if err != nil {
		return nil, err
	}
	if len(names) == 0 {
		return []string{project + "-" + service + "-1"}, nil
	}
	return names, nil
}

// parsedExpectedContainerNames reads the rendered Compose config for one service
// and derives its planned native container names. Without a pinned
// container_name the service scales with deploy.replicas into
// <project>-<service>-1..-N; with a pinned name it is a single container
// (compose forbids container_name + replicas).
// ParsedExpectedContainerNames reads the rendered Compose config for one service
// and derives its planned native container names. Without a pinned
// container_name the service scales with deploy.replicas into
// <project>-<service>-1..-N; with a pinned name it is a single container
// (compose forbids container_name + replicas). Exposed so the retirement and
// control-plane stop paths share this exact planned-set contract.
func ParsedExpectedContainerNames(rendered, project, service string) ([]string, error) {
	return parsedExpectedContainerNames(rendered, project, service)
}
func parsedExpectedContainerNames(rendered, project, service string) ([]string, error) {
	var config struct {
		Services map[string]struct {
			ContainerName string `json:"container_name"`
			Deploy        struct {
				Replicas int `json:"replicas"`
			} `json:"deploy"`
		} `json:"services"`
	}
	if rendered == "" {
		return nil, nil
	}
	if err := json.Unmarshal([]byte(rendered), &config); err != nil {
		return nil, fmt.Errorf("decode rendered Compose config for expected container names: %w", err)
	}
	def, ok := config.Services[service]
	if !ok {
		return nil, nil
	}
	if containerName := containerNameLiteral(def.ContainerName); containerName != "" {
		return []string{containerName}, nil
	}
	replicas := def.Deploy.Replicas
	if replicas < 1 {
		replicas = 1
	}
	names := make([]string, 0, replicas)
	for i := 1; i <= replicas; i++ {
		names = append(names, fmt.Sprintf("%s-%s-%d", project, service, i))
	}
	return names, nil
}

// containerNameLiteral returns the effective pinned container name or "" when
// the compose field is unset/blank.
func containerNameLiteral(raw string) string {
	return raw
}

// ContainerNameInPlannedSet reports whether container.name is one of the names
// the deployment plans for its service, staying inside the project scope that
// the caller already validated. Exposed so the retirement and control-plane
// stop paths share this exact planned-set matcher.
func ContainerNameInPlannedSet(kind ProviderKind, container RuntimeContainer, project string, planned []string) bool {
	return containerNameInPlannedSet(kind, container, project, planned)
}

// containerNameInPlannedSet reports whether container.name is one of the names
// the deployment plans for its service, staying inside the project scope that
// the caller already validated.
func containerNameInPlannedSet(kind ProviderKind, container RuntimeContainer, project string, planned []string) bool {
	if container.Project != project || container.Service == "" {
		return false
	}
	for _, name := range planned {
		if container.Name == name {
			return true
		}
	}
	return false
}
