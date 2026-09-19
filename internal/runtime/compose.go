package runtime

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"strings"
)

var ErrRuntimeNotFound = errors.New("docker compose or podman compose not found")
var ErrResourceOwnership = errors.New("runtime resource ownership does not match the application project")

type ProjectResource struct {
	Kind string
	Name string
}


type ComposeContainer struct {
	Name    string
	Project string
	Service string
}

// Compose provides the small lifecycle surface BaseHarbor needs from a
// container runtime. Application code should not shell out to Docker/Podman
// directly.
type Compose struct {
	command string
	prefix  []string
}

// DetectCompose remains the compatibility entry point for existing v0.3
// callers. Provider selection itself is centralized in DetectProvider so new
// runtime implementations do not require application-contract changes.
func DetectCompose(ctx context.Context) (Compose, error) {
	provider, err := DetectProvider(ctx)
	if err != nil {
		return Compose{}, err
	}
	compose, ok := provider.(Compose)
	if !ok {
		return Compose{}, fmt.Errorf("selected runtime provider %q is not compatible with the Compose runtime path", provider.Kind())
	}
	return compose, nil
}

func detectCompose(ctx context.Context) (Compose, error) {
	if path, err := exec.LookPath("docker"); err == nil {
		cmd := exec.CommandContext(ctx, path, "compose", "version")
		if err := cmd.Run(); err == nil {
			return Compose{command: path, prefix: []string{"compose"}}, nil
		}
	}

	if path, err := exec.LookPath("podman"); err == nil {
		cmd := exec.CommandContext(ctx, path, "compose", "version")
		if err := cmd.Run(); err == nil {
			return Compose{command: path, prefix: []string{"compose"}}, nil
		}
	}

	return Compose{}, ErrRuntimeNotFound
}

func (c Compose) Up(ctx context.Context, composeFile, envFile string) error {
	return c.UpProject(ctx, "baseharbor", composeFile, envFile)
}

func (c Compose) Down(ctx context.Context, composeFile, envFile string) error {
	return c.DownProject(ctx, "baseharbor", composeFile, envFile)
}

func (c Compose) Status(ctx context.Context, composeFile, envFile string) (string, error) {
	return c.StatusProject(ctx, "baseharbor", composeFile, envFile)
}

func (c Compose) Config(ctx context.Context, composeFile, envFile string) error {
	return c.ConfigProject(ctx, "baseharbor", composeFile, envFile)
}

func (c Compose) UpProject(ctx context.Context, project, composeFile, envFile string) error {
	return c.runProject(ctx, project, composeFile, envFile, "up", "-d")
}

func (c Compose) DownProject(ctx context.Context, project, composeFile, envFile string) error {
	return c.runProject(ctx, project, composeFile, envFile, "down")
}

func (c Compose) DownProjectRemoveOrphans(ctx context.Context, project, composeFile, envFile string) error {
	return c.runProject(ctx, project, composeFile, envFile, "down", "--remove-orphans")
}

func (c Compose) DestroyProject(ctx context.Context, project, composeFile, envFile string) error {
	return c.runProject(ctx, project, composeFile, envFile, "down", "--volumes")
}

func (c Compose) DestroyProjectRemoveOrphans(ctx context.Context, project, composeFile, envFile string) error {
	return c.runProject(ctx, project, composeFile, envFile, "down", "--volumes", "--remove-orphans")
}

func (c Compose) StatusProject(ctx context.Context, project, composeFile, envFile string) (string, error) {
	return c.outputProject(ctx, project, composeFile, envFile, "ps")
}

func (c Compose) ConfigProject(ctx context.Context, project, composeFile, envFile string) error {
	return c.runProject(ctx, project, composeFile, envFile, "config", "--quiet")
}

func (c Compose) ExecProject(ctx context.Context, project, composeFile, envFile, service string, args ...string) (string, error) {
	cmdArgs := append([]string{"exec", "-T", service}, args...)
	return c.outputProject(ctx, project, composeFile, envFile, cmdArgs...)
}

// ExecProjectInput executes a command inside a Compose service while supplying
// stdin without placing that input in the host process argument list. It is
// intended for sensitive operator flows such as OpenBao unseal/authentication.
func (c Compose) ExecProjectInput(ctx context.Context, project, composeFile, envFile string, input []byte, service string, args ...string) (string, error) {
	cmdArgs := append([]string{"exec", "-T", service}, args...)
	return c.outputProjectInput(ctx, project, composeFile, envFile, input, cmdArgs...)
}

func (c Compose) RunningServicesProject(ctx context.Context, project, composeFile, envFile string) ([]string, error) {
	out, err := c.outputProject(ctx, project, composeFile, envFile, "ps", "--services", "--status", "running")
	if err != nil {
		return nil, err
	}
	var services []string
	for _, line := range strings.Split(out, "\n") {
		if value := strings.TrimSpace(line); value != "" {
			services = append(services, value)
		}
	}
	return services, nil
}

// InspectProjectResource verifies an exact runtime resource name before a
// destructive operation. A resource with the expected name but a different
// Compose project label is an ownership conflict, never an implicit match.
func (c Compose) InspectProjectResource(ctx context.Context, project string, resource ProjectResource) (bool, error) {
	if c.command == "" {
		return false, ErrRuntimeNotFound
	}
	if strings.TrimSpace(project) == "" || strings.TrimSpace(resource.Name) == "" {
		return false, errors.New("project and resource name are required")
	}

	listArgs, inspectArgs, err := resourceCommands(resource)
	if err != nil {
		return false, err
	}
	out, err := c.directOutput(ctx, listArgs...)
	if err != nil {
		return false, err
	}
	exists := false
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) == resource.Name {
			exists = true
			break
		}
	}
	if !exists {
		return false, nil
	}

	label, err := c.directOutput(ctx, inspectArgs...)
	if err != nil {
		return true, fmt.Errorf("inspect %s %s ownership: %w", resource.Kind, resource.Name, err)
	}
	if strings.TrimSpace(label) != project {
		return true, fmt.Errorf("%w: %s %s is not owned by project %s", ErrResourceOwnership, resource.Kind, resource.Name, project)
	}
	return true, nil
}

func resourceCommands(resource ProjectResource) ([]string, []string, error) {
	switch resource.Kind {
	case "container":
		return []string{"container", "ls", "-a", "--format", "{{.Names}}"}, []string{"container", "inspect", "--format", `{{ index .Config.Labels "com.docker.compose.project" }}`, resource.Name}, nil
	case "network":
		return []string{"network", "ls", "--format", "{{.Name}}"}, []string{"network", "inspect", "--format", `{{ index .Labels "com.docker.compose.project" }}`, resource.Name}, nil
	case "volume":
		return []string{"volume", "ls", "--format", "{{.Name}}"}, []string{"volume", "inspect", "--format", `{{ index .Labels "com.docker.compose.project" }}`, resource.Name}, nil
	default:
		return nil, nil, fmt.Errorf("unsupported runtime resource kind %q", resource.Kind)
	}
}

func (c Compose) ListComposeContainers(ctx context.Context) ([]ComposeContainer, error) {
	out, err := c.directOutput(ctx, "container", "ls", "-a", "--format", "{{.Names}}")
	if err != nil {
		return nil, err
	}
	var result []ComposeContainer
	for _, line := range strings.Split(out, "\n") {
		name := strings.TrimSpace(line)
		if name == "" {
			continue
		}
		labels, err := c.directOutput(ctx, "container", "inspect", "--format", `{{ index .Config.Labels "com.docker.compose.project" }}|{{ index .Config.Labels "com.docker.compose.service" }}`, name)
		if err != nil {
			return nil, err
		}
		project, service, ok := strings.Cut(strings.TrimSpace(labels), "|")
		if !ok || strings.TrimSpace(project) == "" || strings.TrimSpace(service) == "" {
			continue
		}
		result = append(result, ComposeContainer{Name: name, Project: strings.TrimSpace(project), Service: strings.TrimSpace(service)})
	}
	return result, nil
}

func (c Compose) ContainerNetworks(ctx context.Context, container string) ([]string, error) {
	out, err := c.directOutput(ctx, "container", "inspect", "--format", `{{range $name, $_ := .NetworkSettings.Networks}}{{$name}}{{"\n"}}{{end}}`, strings.TrimSpace(container))
	if err != nil {
		return nil, err
	}
	var result []string
	for _, line := range strings.Split(out, "\n") {
		if value := strings.TrimSpace(line); value != "" {
			result = append(result, value)
		}
	}
	return result, nil
}

func (c Compose) NetworkProjectOwner(ctx context.Context, network string) (string, error) {
	out, err := c.directOutput(ctx, "network", "inspect", "--format", `{{ index .Labels "com.docker.compose.project" }}`, strings.TrimSpace(network))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func (c Compose) ContainerExposedTCPPorts(ctx context.Context, container string) ([]int, error) {
	out, err := c.directOutput(ctx, "container", "inspect", "--format", `{{range $port, $_ := .Config.ExposedPorts}}{{$port}}{{"\n"}}{{end}}`, strings.TrimSpace(container))
	if err != nil {
		return nil, err
	}
	var ports []int
	seen := map[int]struct{}{}
	for _, line := range strings.Split(out, "\n") {
		value := strings.TrimSpace(line)
		if value == "" || !strings.HasSuffix(value, "/tcp") {
			continue
		}
		raw := strings.TrimSuffix(value, "/tcp")
		port, err := strconv.Atoi(raw)
		if err != nil || port < 1 || port > 65535 {
			continue
		}
		if _, ok := seen[port]; ok {
			continue
		}
		seen[port] = struct{}{}
		ports = append(ports, port)
	}
	sort.Ints(ports)
	return ports, nil
}

func (c Compose) EnsureManagedNetwork(ctx context.Context, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("managed network name is required")
	}
	out, err := c.directOutput(ctx, "network", "ls", "--format", "{{.Name}}")
	if err != nil {
		return err
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) != name {
			continue
		}
		label, err := c.directOutput(ctx, "network", "inspect", "--format", `{{ index .Labels "io.baseharbor.managed" }}`, name)
		if err != nil {
			return err
		}
		if strings.TrimSpace(label) != "connectivity" {
			return fmt.Errorf("runtime network %s already exists but is not BaseHarbor connectivity-owned", name)
		}
		return nil
	}
	_, err = c.directOutput(ctx, "network", "create", "--label", "io.baseharbor.managed=connectivity", name)
	return err
}

func (c Compose) ConnectManagedNetwork(ctx context.Context, network, container, alias string) error {
	args := []string{"network", "connect"}
	if alias = strings.TrimSpace(alias); alias != "" {
		args = append(args, "--alias", alias)
	}
	args = append(args, strings.TrimSpace(network), strings.TrimSpace(container))
	_, err := c.directOutput(ctx, args...)
	if err != nil {
		message := strings.ToLower(err.Error())
		if strings.Contains(message, "already exists") || strings.Contains(message, "already connected") {
			return nil
		}
	}
	return err
}

func (c Compose) DisconnectManagedNetwork(ctx context.Context, network, container string) error {
	_, err := c.directOutput(ctx, "network", "disconnect", strings.TrimSpace(network), strings.TrimSpace(container))
	if err != nil {
		message := strings.ToLower(err.Error())
		if strings.Contains(message, "not connected") || strings.Contains(message, "is not connected") {
			return nil
		}
	}
	return err
}

func (c Compose) RemoveManagedNetwork(ctx context.Context, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("managed network name is required")
	}
	out, err := c.directOutput(ctx, "network", "ls", "--format", "{{.Name}}")
	if err != nil {
		return err
	}
	found := false
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) == name {
			found = true
			break
		}
	}
	if !found {
		return nil
	}
	label, err := c.directOutput(ctx, "network", "inspect", "--format", `{{ index .Labels "io.baseharbor.managed" }}`, name)
	if err != nil {
		return err
	}
	if strings.TrimSpace(label) != "connectivity" {
		return fmt.Errorf("refusing to remove runtime network %s because it is not BaseHarbor connectivity-owned", name)
	}
	_, err = c.directOutput(ctx, "network", "rm", name)
	return err
}

func (c Compose) directOutput(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, c.command, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return stdout.String(), fmt.Errorf("runtime %s: %s", strings.Join(args, " "), message)
	}
	return stdout.String(), nil
}

func (c Compose) runProject(ctx context.Context, project, composeFile, envFile string, args ...string) error {
	_, err := c.outputProject(ctx, project, composeFile, envFile, args...)
	return err
}

func (c Compose) outputProject(ctx context.Context, project, composeFile, envFile string, args ...string) (string, error) {
	return c.outputProjectInput(ctx, project, composeFile, envFile, nil, args...)
}

func (c Compose) outputProjectInput(ctx context.Context, project, composeFile, envFile string, input []byte, args ...string) (string, error) {
	if c.command == "" {
		return "", ErrRuntimeNotFound
	}
	if strings.TrimSpace(project) == "" {
		return "", errors.New("compose project name is required")
	}

	fullArgs := append([]string{}, c.prefix...)
	fullArgs = append(fullArgs, "--project-name", project, "--file", composeFile, "--env-file", envFile)
	fullArgs = append(fullArgs, args...)

	cmd := exec.CommandContext(ctx, c.command, fullArgs...)
	if input != nil {
		cmd.Stdin = bytes.NewReader(input)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return stdout.String(), fmt.Errorf("compose %s: %s", strings.Join(args, " "), message)
	}
	return stdout.String(), nil
}
