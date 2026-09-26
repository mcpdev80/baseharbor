package runtime

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
)

func (c Compose) RunningServicesProject(ctx context.Context, project, composeFile, envFile string) ([]string, error) {
	if c.command == "" {
		return nil, ErrRuntimeNotFound
	}
	project = strings.TrimSpace(project)
	if project == "" {
		return nil, errors.New("compose project name is required")
	}

	var moduleServices map[string]struct{}
	if consolidatedProject(project) && strings.TrimSpace(composeFile) != "" {
		moduleServices = map[string]struct{}{}
		if c.quadlet {
			q, err := quadletRenderProject(composeFile, envFile, project)
			if err != nil {
				return nil, err
			}
			for service := range q.ServiceUnits {
				moduleServices[service] = struct{}{}
			}
		} else {
			out, err := c.outputProject(ctx, project, composeFile, envFile, "config", "--services")
			if err != nil {
				return nil, err
			}
			for _, line := range strings.Split(out, "\n") {
				if service := strings.TrimSpace(line); service != "" {
					moduleServices[service] = struct{}{}
				}
			}
		}
	}

	containers, err := c.ListComposeContainers(ctx)
	if err != nil {
		return nil, err
	}

	seen := map[string]struct{}{}
	var services []string
	for _, container := range containers {
		if container.Project != project || !container.Running {
			continue
		}
		if moduleServices != nil {
			if _, ok := moduleServices[container.Service]; !ok {
				continue
			}
		}
		if _, ok := seen[container.Service]; ok {
			continue
		}
		seen[container.Service] = struct{}{}
		services = append(services, container.Service)
	}
	sort.Strings(services)
	return services, nil
}

// InspectProjectResource verifies an exact runtime resource name before a
// destructive operation. A resource with the expected name but a different
// Compose project label is an ownership conflict, never an implicit match.
func (c Compose) InspectProjectResource(ctx context.Context, project string, resource ProjectResource) (bool, error) {
	existing, err := c.InspectProjectResources(ctx, project, []ProjectResource{resource})
	if err != nil {
		return false, err
	}
	return len(existing) == 1, nil
}

// InspectProjectResources verifies runtime resource existence and ownership in
// batches by kind. This keeps ownership fail-closed while avoiding repeated
// list/inspect round-trips on Podman.
func (c Compose) InspectProjectResources(ctx context.Context, project string, resources []ProjectResource) ([]ProjectResource, error) {
	if c.command == "" {
		return nil, ErrRuntimeNotFound
	}
	project = strings.TrimSpace(project)
	if project == "" {
		return nil, errors.New("project is required")
	}
	if len(resources) == 0 {
		return nil, nil
	}

	requested := map[string]map[string]struct{}{}
	for _, resource := range resources {
		name := strings.TrimSpace(resource.Name)
		if name == "" {
			return nil, errors.New("resource name is required")
		}
		switch resource.Kind {
		case "container", "network", "volume":
		default:
			return nil, fmt.Errorf("unsupported runtime resource kind %q", resource.Kind)
		}
		if requested[resource.Kind] == nil {
			requested[resource.Kind] = map[string]struct{}{}
		}
		requested[resource.Kind][name] = struct{}{}
	}

	found := map[string]struct{}{}
	for _, kind := range []string{"container", "network", "volume"} {
		wanted := requested[kind]
		if len(wanted) == 0 {
			continue
		}

		var listArgs []string
		var inspectPrefix []string
		var inspectTemplate string
		switch kind {
		case "container":
			listArgs = []string{"container", "ls", "-a", "--format", "{{.Names}}"}
			inspectPrefix = []string{"container", "inspect", "--format"}
			inspectTemplate = `{{.Name}}|{{ index .Config.Labels "com.docker.compose.project" }}|{{ index .Config.Labels "io.podman.compose.project" }}`
		case "network":
			listArgs = []string{"network", "ls", "--format", "{{.Name}}"}
			inspectPrefix = []string{"network", "inspect", "--format"}
			inspectTemplate = `{{.Name}}|{{ index .Labels "com.docker.compose.project" }}|{{ index .Labels "io.podman.compose.project" }}`
		case "volume":
			listArgs = []string{"volume", "ls", "--format", "{{.Name}}"}
			inspectPrefix = []string{"volume", "inspect", "--format"}
			inspectTemplate = `{{.Name}}|{{ index .Labels "com.docker.compose.project" }}|{{ index .Labels "io.podman.compose.project" }}`
		}

		existingNames := map[string]struct{}{}
		if c.quadlet {
			for name := range wanted {
				exists, err := quadletRuntimeResourceExists(ctx, kind, name)
				if err != nil {
					return nil, err
				}
				if exists {
					existingNames[name] = struct{}{}
				}
			}
		} else {
			listed, err := c.directOutput(ctx, listArgs...)
			if err != nil {
				return nil, err
			}
			for _, line := range strings.Split(listed, "\n") {
				if name := strings.TrimSpace(line); name != "" {
					existingNames[name] = struct{}{}
				}
			}
		}

		var names []string
		for _, resource := range resources {
			if resource.Kind != kind {
				continue
			}
			name := strings.TrimSpace(resource.Name)
			if _, ok := existingNames[name]; !ok {
				continue
			}
			if _, duplicate := found[kind+"\x00"+name]; duplicate {
				continue
			}
			names = append(names, name)
		}
		if len(names) == 0 {
			continue
		}

		args := append([]string{}, inspectPrefix...)
		args = append(args, inspectTemplate)
		args = append(args, names...)
		inspected, err := c.directOutput(ctx, args...)
		if err != nil {
			return nil, fmt.Errorf("inspect %s ownership: %w", kind, err)
		}

		owners := map[string]string{}
		for _, line := range strings.Split(inspected, "\n") {
			parts := strings.Split(strings.TrimSpace(line), "|")
			if len(parts) != 3 {
				continue
			}
			name := strings.TrimPrefix(strings.TrimSpace(parts[0]), "/")
			owners[name] = firstRuntimeLabel(parts[1], parts[2])
		}

		for _, name := range names {
			owner, ok := owners[name]
			if !ok {
				return nil, fmt.Errorf("inspect %s %s ownership returned no result", kind, name)
			}
			if owner != project {
				return nil, fmt.Errorf("%w: %s %s is not owned by project %s", ErrResourceOwnership, kind, name, project)
			}
			found[kind+"\x00"+name] = struct{}{}
		}
	}

	existing := make([]ProjectResource, 0, len(found))
	for _, resource := range resources {
		if _, ok := found[resource.Kind+"\x00"+strings.TrimSpace(resource.Name)]; ok {
			existing = append(existing, resource)
		}
	}
	return existing, nil
}

func (c Compose) DestroyOwnedProjectResources(ctx context.Context, project string, resources []ProjectResource) error {
	if c.command == "" {
		return ErrRuntimeNotFound
	}
	project = strings.TrimSpace(project)
	if project == "" {
		return errors.New("project is required")
	}
	existing, err := c.InspectProjectResources(ctx, project, resources)
	if err != nil {
		return err
	}
	removeKind := func(kind string) error {
		for _, resource := range existing {
			if resource.Kind != kind {
				continue
			}
			var args []string
			switch resource.Kind {
			case "container":
				args = []string{"container", "rm", "-f", resource.Name}
			case "network":
				args = []string{"network", "rm", resource.Name}
			case "volume":
				args = []string{"volume", "rm", resource.Name}
			default:
				return fmt.Errorf("unsupported runtime resource kind %q", resource.Kind)
			}
			if _, err := c.directOutput(ctx, args...); err != nil {
				return fmt.Errorf("remove owned %s %s: %w", resource.Kind, resource.Name, err)
			}
		}
		return nil
	}
	if err := removeKind("container"); err != nil {
		return err
	}
	if err := removeKind("network"); err != nil {
		return err
	}
	if err := removeKind("volume"); err != nil {
		return err
	}
	return nil
}

func (c Compose) ListComposeContainers(ctx context.Context) ([]ComposeContainer, error) {
	out, err := c.directOutput(ctx, "container", "ls", "-aq")
	if err != nil {
		return nil, err
	}

	var ids []string
	for _, line := range strings.Split(out, "\n") {
		if id := strings.TrimSpace(line); id != "" {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil, nil
	}

	args := []string{
		"container", "inspect", "--format",
		`{{.Name}}|{{ index .Config.Labels "com.docker.compose.project" }}|{{ index .Config.Labels "io.podman.compose.project" }}|{{ index .Config.Labels "com.docker.compose.service" }}|{{ index .Config.Labels "io.podman.compose.service" }}|{{.State.Running}}|{{if .State.Health}}{{.State.Health.Status}}{{end}}`,
	}
	args = append(args, ids...)
	inspected, err := c.directOutput(ctx, args...)
	if err != nil {
		return nil, err
	}

	var result []ComposeContainer
	for _, line := range strings.Split(inspected, "\n") {
		parts := strings.Split(strings.TrimSpace(line), "|")
		if len(parts) != 7 {
			continue
		}
		name := strings.TrimPrefix(strings.TrimSpace(parts[0]), "/")
		project := firstRuntimeLabel(parts[1], parts[2])
		service := firstRuntimeLabel(parts[3], parts[4])
		if name == "" || project == "" || service == "" {
			continue
		}
		result = append(result, ComposeContainer{
			Name:    name,
			Project: project,
			Service: service,
			Running: strings.EqualFold(strings.TrimSpace(parts[5]), "true"),
			Health:  strings.TrimSpace(parts[6]),
		})
	}
	return result, nil
}

func (c Compose) ProjectServiceImageIdentity(ctx context.Context, project, service string) (ImageIdentity, error) {
	project = strings.TrimSpace(project)
	service = strings.TrimSpace(service)
	if project == "" || service == "" {
		return ImageIdentity{}, errors.New("project and service are required for image identity")
	}
	containers, err := c.ListComposeContainers(ctx)
	if err != nil {
		return ImageIdentity{}, err
	}
	containerName := ""
	for _, container := range containers {
		if container.Project == project && container.Service == service {
			containerName = container.Name
			break
		}
	}
	if containerName == "" {
		return ImageIdentity{}, fmt.Errorf("service %s is not realized in project %s", service, project)
	}
	out, err := c.directOutput(ctx, "container", "inspect", "--format", `{{.Config.Image}}|{{.Image}}`, containerName)
	if err != nil {
		return ImageIdentity{}, fmt.Errorf("inspect image identity for %s/%s: %w", project, service, err)
	}
	parts := strings.SplitN(strings.TrimSpace(out), "|", 2)
	if len(parts) != 2 {
		return ImageIdentity{}, errors.New("runtime returned invalid container image identity")
	}
	identity := ImageIdentity{Reference: strings.TrimSpace(parts[0]), ImageID: strings.TrimSpace(parts[1])}
	if identity.ImageID == "" {
		return ImageIdentity{}, errors.New("runtime returned empty container image identity")
	}
	digests, err := c.directOutput(ctx, "image", "inspect", "--format", `{{range .RepoDigests}}{{.}}{{"\n"}}{{end}}`, identity.ImageID)
	if err == nil {
		for _, line := range strings.Split(digests, "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				identity.Digest = line
				break
			}
		}
	}
	return identity, nil
}

func firstRuntimeLabel(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && value != "<no value>" {
			return value
		}
	}
	return ""
}
