package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
)

var ErrRuntimeNotFound = errors.New("container runtime orchestration not found")
var ErrResourceOwnership = errors.New("runtime resource ownership does not match the application project")

// Compose provides the small lifecycle surface BaseHarbor needs from a
// container runtime. Application code should not shell out to Docker/Podman
// directly.
type Compose struct {
	command string
	prefix  []string
}

func NewCLIBackend(command string, prefix ...string) Compose {
	return Compose{command: command, prefix: append([]string(nil), prefix...)}
}

func (c Compose) CommandPath() string {
	return c.command
}

func (c Compose) DirectOutput(ctx context.Context, args ...string) (string, error) {
	return c.directOutput(ctx, args...)
}

func (c Compose) DirectStream(ctx context.Context, args ...string) (io.ReadCloser, error) {
	return c.directStream(ctx, args...)
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
	_, err := c.outputProjectInputProgress(ctx, project, composeFile, envFile, nil, onProgress, "up", "-d")
	return err
}

func (c Compose) DownProject(ctx context.Context, project, composeFile, envFile string) error {
	if consolidatedProject(project) {
		return c.removeComposeModule(ctx, project, composeFile, envFile, false)
	}
	return c.runProject(ctx, project, composeFile, envFile, "down")
}

func (c Compose) StopProject(ctx context.Context, project, composeFile, envFile string) error {
	return c.runProject(ctx, project, composeFile, envFile, "stop")
}

func (c Compose) DownProjectRemoveOrphans(ctx context.Context, project, composeFile, envFile string) error {
	if consolidatedProject(project) {
		return c.DownProject(ctx, project, composeFile, envFile)
	}
	return c.runProject(ctx, project, composeFile, envFile, "down", "--remove-orphans")
}

func (c Compose) DestroyProject(ctx context.Context, project, composeFile, envFile string) error {
	if consolidatedProject(project) {
		return c.removeComposeModule(ctx, project, composeFile, envFile, true)
	}
	return c.runProject(ctx, project, composeFile, envFile, "down", "--volumes")
}

func (c Compose) DestroyProjectRemoveOrphans(ctx context.Context, project, composeFile, envFile string) error {
	if consolidatedProject(project) {
		resources, err := c.ListOwnedProjectResources(ctx, project)
		if err != nil {
			return fmt.Errorf("inventory owned project resources before full destroy: %w", err)
		}
		if err := c.DestroyOwnedProjectResources(ctx, project, resources); err != nil {
			return fmt.Errorf("destroy owned project resources: %w", err)
		}
		remaining, err := c.ListOwnedProjectResources(ctx, project)
		if err != nil {
			return fmt.Errorf("verify owned project resources after full destroy: %w", err)
		}
		if len(remaining) != 0 {
			var names []string
			for _, resource := range remaining {
				names = append(names, resource.Kind+" "+resource.Name)
			}
			sort.Strings(names)
			return fmt.Errorf("owned project resources remain after full destroy: %s", strings.Join(names, ", "))
		}
		return nil
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
	return c.outputProject(ctx, project, composeFile, envFile, "ps")
}

func (c Compose) LogsProject(ctx context.Context, project, composeFile, envFile string, services ...string) (string, error) {
	args := []string{"logs", "--no-color", "--tail", "120"}
	args = append(args, services...)
	return c.outputProject(ctx, project, composeFile, envFile, args...)
}

// DiagnosticsProject captures stopped/restarting containers and recent logs
// before a fail-closed lifecycle rollback removes provider resources.
func (c Compose) DiagnosticsProject(ctx context.Context, project, composeFile, envFile string) string {
	status, statusErr := c.outputProject(ctx, project, composeFile, envFile, "ps", "-a")
	logs, logsErr := c.outputProject(ctx, project, composeFile, envFile, "logs", "--no-color", "--tail", "1000")
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
