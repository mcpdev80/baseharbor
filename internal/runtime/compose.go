package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

var ErrRuntimeNotFound = errors.New("container runtime orchestration not found")
var ErrResourceOwnership = errors.New("runtime resource ownership does not match the application project")

type ProjectResource struct {
	Kind string
	Name string
}

type ComposeContainer struct {
	Name    string
	Project string
	Service string
	Running bool
	Health  string
}

type ImageIdentity struct {
	Reference string
	ImageID   string
	Digest    string
}

// Compose provides the small lifecycle surface BaseHarbor needs from a
// container runtime. Application code should not shell out to Docker/Podman
// directly.
type Compose struct {
	command  string
	prefix   []string
	quadlet  bool
	provider ProviderKind
}

func (c Compose) Engine() string {
	base := filepath.Base(strings.TrimSpace(c.command))
	switch base {
	case "podman":
		return "podman"
	case "docker":
		return "docker"
	default:
		return base
	}
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
	if docker, err := detectDockerCompose(ctx); err == nil {
		return docker, nil
	}
	if podman, err := detectPodmanCompose(ctx); err == nil {
		return podman, nil
	}
	return Compose{}, ErrRuntimeNotFound
}

func detectDockerCompose(ctx context.Context) (Compose, error) {
	path, err := exec.LookPath("docker")
	if err != nil {
		return Compose{}, ErrRuntimeNotFound
	}
	cmd := exec.CommandContext(ctx, path, "compose", "version")
	if err := cmd.Run(); err != nil {
		return Compose{}, ErrRuntimeNotFound
	}
	return Compose{command: path, prefix: []string{"compose"}, provider: ProviderDocker}, nil
}

func detectPodmanCompose(ctx context.Context) (Compose, error) {
	path, err := exec.LookPath("podman")
	if err != nil {
		return Compose{}, ErrRuntimeNotFound
	}
	if QuadletAvailable(ctx) {
		return Compose{command: path, quadlet: true, provider: ProviderPodman}, nil
	}
	cmd := exec.CommandContext(ctx, path, "compose", "version")
	if err := cmd.Run(); err != nil {
		return Compose{}, ErrRuntimeNotFound
	}
	return Compose{command: path, prefix: []string{"compose"}, provider: ProviderPodman}, nil
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
	return c.UpProjectProgress(ctx, project, composeFile, envFile, nil)
}

func (c Compose) UpProjectProgress(ctx context.Context, project, composeFile, envFile string, onProgress func(string)) error {
	if c.quadlet {
		q, err := quadletRenderProject(composeFile, envFile, project)
		if err != nil {
			return err
		}
		if onProgress != nil {
			onProgress("rendered Podman Quadlet runtime")
		}
		if err := quadletStartProject(ctx, q, nil); err != nil {
			return err
		}
		if onProgress != nil {
			onProgress("started Podman Quadlet services")
		}
		return nil
	}
	_, err := c.outputProjectInputProgress(ctx, project, composeFile, envFile, nil, onProgress, "up", "-d")
	return err
}

func (c Compose) DownProject(ctx context.Context, project, composeFile, envFile string) error {
	if c.quadlet {
		q, err := quadletRenderProject(composeFile, envFile, project)
		if err != nil {
			return err
		}
		return quadletRemoveProject(ctx, q, false)
	}
	if consolidatedProject(project) {
		return c.removeComposeModule(ctx, project, composeFile, envFile, false)
	}
	return c.runProject(ctx, project, composeFile, envFile, "down")
}

func (c Compose) StopProject(ctx context.Context, project, composeFile, envFile string) error {
	if c.quadlet {
		q, err := quadletRenderProject(composeFile, envFile, project)
		if err != nil {
			return err
		}
		return quadletStopProject(ctx, q, nil)
	}
	return c.runProject(ctx, project, composeFile, envFile, "stop")
}

func (c Compose) DownProjectRemoveOrphans(ctx context.Context, project, composeFile, envFile string) error {
	if c.quadlet || consolidatedProject(project) {
		return c.DownProject(ctx, project, composeFile, envFile)
	}
	return c.runProject(ctx, project, composeFile, envFile, "down", "--remove-orphans")
}

func (c Compose) DestroyProject(ctx context.Context, project, composeFile, envFile string) error {
	if c.quadlet {
		q, err := quadletRenderProject(composeFile, envFile, project)
		if err != nil {
			return err
		}
		return quadletRemoveProject(ctx, q, true)
	}
	if consolidatedProject(project) {
		return c.removeComposeModule(ctx, project, composeFile, envFile, true)
	}
	return c.runProject(ctx, project, composeFile, envFile, "down", "--volumes")
}

func (c Compose) DestroyProjectRemoveOrphans(ctx context.Context, project, composeFile, envFile string) error {
	if c.quadlet || consolidatedProject(project) {
		return c.DestroyProject(ctx, project, composeFile, envFile)
	}
	return c.runProject(ctx, project, composeFile, envFile, "down", "--volumes", "--remove-orphans")
}

func consolidatedProject(project string) bool {
	return strings.HasPrefix(strings.TrimSpace(project), "bh-")
}

type composeModuleModel struct {
	Services map[string]json.RawMessage `json:"services"`
	Volumes  map[string]struct {
		Name string `json:"name"`
	} `json:"volumes"`
	Networks map[string]struct {
		Name     string `json:"name"`
		External bool   `json:"external"`
	} `json:"networks"`
}

func (c Compose) removeComposeModule(ctx context.Context, project, composeFile, envFile string, destroy bool) error {
	rendered, err := c.outputProject(ctx, project, composeFile, envFile, "config", "--format", "json")
	if err != nil {
		return err
	}
	var model composeModuleModel
	if err := json.Unmarshal([]byte(rendered), &model); err != nil {
		return fmt.Errorf("decode Compose module model: %w", err)
	}
	services := make([]string, 0, len(model.Services))
	for service := range model.Services {
		services = append(services, service)
	}
	sort.Strings(services)
	if len(services) > 0 {
		args := append([]string{"rm", "-f", "-s"}, services...)
		if err := c.runProject(ctx, project, composeFile, envFile, args...); err != nil {
			return err
		}
	}
	if destroy {
		for _, volume := range model.Volumes {
			if strings.TrimSpace(volume.Name) == "" {
				continue
			}
			if _, err := c.directOutput(ctx, "volume", "rm", "-f", volume.Name); err != nil && !strings.Contains(strings.ToLower(err.Error()), "no such volume") {
				return fmt.Errorf("remove Compose module volume %s: %w", volume.Name, err)
			}
		}
	}
	for _, network := range model.Networks {
		if network.External || strings.TrimSpace(network.Name) == "" {
			continue
		}
		if _, err := c.directOutput(ctx, "network", "rm", network.Name); err != nil {
			lower := strings.ToLower(err.Error())
			if !strings.Contains(lower, "not found") && !strings.Contains(lower, "no such network") && !strings.Contains(lower, "active endpoints") {
				return fmt.Errorf("remove Compose module network %s: %w", network.Name, err)
			}
		}
	}
	return nil
}

func (c Compose) StatusProject(ctx context.Context, project, composeFile, envFile string) (string, error) {
	if c.quadlet {
		containers, err := c.ListComposeContainers(ctx)
		if err != nil {
			return "", err
		}
		var lines []string
		for _, container := range containers {
			if container.Project == project {
				lines = append(lines, fmt.Sprintf("%s\t%s\t%t", container.Service, container.Name, container.Running))
			}
		}
		sort.Strings(lines)
		return strings.Join(lines, "\n"), nil
	}
	return c.outputProject(ctx, project, composeFile, envFile, "ps")
}

func (c Compose) LogsProject(ctx context.Context, project, composeFile, envFile string, services ...string) (string, error) {
	if c.quadlet {
		q, err := quadletRenderProject(composeFile, envFile, project)
		if err != nil {
			return "", err
		}
		return quadletLogs(ctx, c.command, q, services)
	}
	args := []string{"logs", "--no-color", "--tail", "120"}
	args = append(args, services...)
	return c.outputProject(ctx, project, composeFile, envFile, args...)
}

// DiagnosticsProject captures stopped/restarting containers and recent logs
// before a fail-closed lifecycle rollback removes provider resources.
func (c Compose) DiagnosticsProject(ctx context.Context, project, composeFile, envFile string) string {
	if c.quadlet {
		q, renderErr := quadletRenderProject(composeFile, envFile, project)
		status, statusErr := c.StatusProject(ctx, project, composeFile, envFile)
		logs, logsErr := c.LogsProject(ctx, project, composeFile, envFile)
		var b strings.Builder
		if renderErr != nil {
			fmt.Fprintf(&b, "Quadlet render failed: %v\n", renderErr)
		}
		if statusErr != nil {
			fmt.Fprintf(&b, "Quadlet status failed: %v\n", statusErr)
		} else {
			fmt.Fprintf(&b, "Quadlet status:\n%s\n", status)
		}
		if renderErr == nil {
			units := make([]string, 0, len(q.ServiceUnits))
			for _, unit := range q.ServiceUnits {
				units = append(units, unit)
			}
			sort.Strings(units)
			for _, unit := range units {
				unitStatus, _ := quadletSystemctlCombined(ctx, "status", "--no-pager", "--full", unit)
				if strings.TrimSpace(unitStatus) != "" {
					fmt.Fprintf(&b, "Quadlet unit %s:\n%s\n", unit, strings.TrimSpace(unitStatus))
				}
			}
		}
		if logsErr != nil {
			fmt.Fprintf(&b, "Quadlet logs failed: %v\n", logsErr)
		} else {
			fmt.Fprintf(&b, "Quadlet logs:\n%s\n", logs)
		}
		return strings.TrimSpace(b.String())
	}
	status, statusErr := c.outputProject(ctx, project, composeFile, envFile, "ps", "-a")
	logs, logsErr := c.outputProject(ctx, project, composeFile, envFile, "logs", "--no-color", "--tail", "100")

	var b strings.Builder
	if statusErr != nil {
		fmt.Fprintf(&b, "compose ps -a failed: %v\n", statusErr)
	} else {
		fmt.Fprintf(&b, "compose ps -a:\n%s", status)
		if status != "" && !strings.HasSuffix(status, "\n") {
			b.WriteByte('\n')
		}
	}
	if logsErr != nil {
		fmt.Fprintf(&b, "compose logs failed: %v\n", logsErr)
	} else {
		fmt.Fprintf(&b, "compose logs:\n%s", logs)
		if logs != "" && !strings.HasSuffix(logs, "\n") {
			b.WriteByte('\n')
		}
	}
	return strings.TrimSpace(b.String())
}

func (c Compose) ConfigProject(ctx context.Context, project, composeFile, envFile string) error {
	if c.quadlet {
		q, err := quadletRenderProject(composeFile, envFile, project)
		if err != nil {
			return err
		}
		return quadletValidateProject(ctx, q)
	}
	return c.runProject(ctx, project, composeFile, envFile, "config", "--quiet")
}

func (c Compose) ExecProject(ctx context.Context, project, composeFile, envFile, service string, args ...string) (string, error) {
	if c.quadlet {
		q, err := quadletRenderProject(composeFile, envFile, project)
		if err != nil {
			return "", err
		}
		container, ok := q.Containers[service]
		if !ok {
			return "", fmt.Errorf("Quadlet service %q is not part of project %s", service, project)
		}
		return quadletExec(ctx, c.command, container, nil, args...)
	}
	cmdArgs := append([]string{"exec", "-T", service}, args...)
	return c.outputProject(ctx, project, composeFile, envFile, cmdArgs...)
}

// ExecProjectInput executes a command inside a Compose service while supplying
// stdin without placing that input in the host process argument list. It is
// intended for sensitive operator flows such as OpenBao unseal/authentication.
func (c Compose) ExecProjectInput(ctx context.Context, project, composeFile, envFile string, input []byte, service string, args ...string) (string, error) {
	if c.quadlet {
		q, err := quadletRenderProject(composeFile, envFile, project)
		if err != nil {
			return "", err
		}
		container, ok := q.Containers[service]
		if !ok {
			return "", fmt.Errorf("Quadlet service %q is not part of project %s", service, project)
		}
		return quadletExec(ctx, c.command, container, input, args...)
	}
	cmdArgs := append([]string{"exec", "-T", service}, args...)
	return c.outputProjectInput(ctx, project, composeFile, envFile, input, cmdArgs...)
}
