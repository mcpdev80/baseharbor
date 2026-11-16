package podman

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

func podmanNetworkHasActiveConsumers(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "network is being used") ||
		strings.Contains(message, "has associated containers") ||
		strings.Contains(message, "active endpoints")
}

func (p PodmanProvider) ContainerLogConfigProjectService(context.Context, string, string) (string, string, error) {
	return "", "", nil
}

func (p PodmanProvider) ProjectServiceLogDriver(ctx context.Context, project, service string) (string, error) {
	project = strings.TrimSpace(project)
	service = strings.TrimSpace(service)
	if project == "" || service == "" {
		return "", errors.New("project and service are required")
	}
	containers, err := p.ListRuntimeContainers(ctx)
	if err != nil {
		return "", err
	}
	for _, container := range containers {
		if container.Project == project && container.Service == service && container.Running {
			return "journald", nil
		}
	}
	return "", fmt.Errorf("running service %s/%s was not found", project, service)
}

func (p PodmanProvider) providerSourceFiles(workdir string, composeFiles []string) ([]string, error) {
	return quadletResolveComposeFiles(workdir, composeFiles)
}

func (p PodmanProvider) workdirFor(composeFile string) string {
	if strings.TrimSpace(composeFile) == "" {
		return ""
	}
	return filepath.Dir(composeFile)
}
