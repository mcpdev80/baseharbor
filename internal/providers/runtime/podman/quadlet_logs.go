package podman

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"sort"
	"strings"
)

func quadletLogs(ctx context.Context, runtimeCommand string, project QuadletProject, services []string) (string, error) {
	if len(services) == 0 {
		for service := range project.Containers {
			services = append(services, service)
		}
	}
	sort.Strings(services)
	var result strings.Builder
	for _, service := range services {
		container, ok := project.Containers[service]
		if !ok {
			return "", fmt.Errorf("Quadlet service %q is not part of project %s", service, project.Project)
		}
		cmd := exec.CommandContext(ctx, runtimeCommand, "logs", "--tail", "120", container)
		cmd.Env = runtimeCommandEnv(runtimeCommand)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			return result.String(), fmt.Errorf("podman logs %s: %s", container, strings.TrimSpace(stderr.String()))
		}
		if result.Len() > 0 {
			result.WriteByte('\n')
		}
		fmt.Fprintf(&result, "==> %s <==\n%s", service, stdout.String())
	}
	return result.String(), nil
}
