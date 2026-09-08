package runtime

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

var ErrRuntimeNotFound = errors.New("docker compose or podman compose not found")
var ErrResourceOwnership = errors.New("runtime resource ownership does not match the application project")

type ProjectResource struct {
	Kind string
	Name string
}

// Compose provides the small lifecycle surface BaseHarbor needs from a
// container runtime. Application code should not shell out to Docker/Podman
// directly.
type Compose struct {
	command string
	prefix  []string
}

func DetectCompose(ctx context.Context) (Compose, error) {
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

func (c Compose) DestroyProject(ctx context.Context, project, composeFile, envFile string) error {
	return c.runProject(ctx, project, composeFile, envFile, "down", "--volumes")
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

// ExecProjectStream executes a command inside a Compose service while
// connecting the supplied streams directly to the child process. This keeps
// large backup/restore payloads out of memory and avoids placing payload data
// in command arguments.
func (c Compose) ExecProjectStream(ctx context.Context, project, composeFile, envFile string, stdin io.Reader, stdout io.Writer, service string, args ...string) error {
	cmdArgs := append([]string{"exec", "-T", service}, args...)
	return c.streamProject(ctx, project, composeFile, envFile, stdin, stdout, cmdArgs...)
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
	var stdout bytes.Buffer
	var stdin io.Reader
	if input != nil {
		stdin = bytes.NewReader(input)
	}
	if err := c.streamProject(ctx, project, composeFile, envFile, stdin, &stdout, args...); err != nil {
		return stdout.String(), err
	}
	return stdout.String(), nil
}

func (c Compose) streamProject(ctx context.Context, project, composeFile, envFile string, stdin io.Reader, stdout io.Writer, args ...string) error {
	if c.command == "" {
		return ErrRuntimeNotFound
	}
	if strings.TrimSpace(project) == "" {
		return errors.New("compose project name is required")
	}

	fullArgs := append([]string{}, c.prefix...)
	fullArgs = append(fullArgs, "--project-name", project, "--file", composeFile, "--env-file", envFile)
	fullArgs = append(fullArgs, args...)

	cmd := exec.CommandContext(ctx, c.command, fullArgs...)
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return fmt.Errorf("compose %s: %s", strings.Join(args, " "), message)
	}
	return nil
}
