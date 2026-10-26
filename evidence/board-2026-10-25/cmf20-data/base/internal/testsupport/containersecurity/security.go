package containersecurity

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type Requirements struct {
	ReadOnlyRootfs bool
	DropAllCaps    bool
	NoNewPrivs     bool
}

type inspectRecord struct {
	EffectiveCaps []string
	Config        struct {
		User   string
		Labels map[string]string
	}
	HostConfig struct {
		Privileged     bool
		ReadonlyRootfs bool
		CapAdd         []string
		CapDrop        []string
		SecurityOpt    []string
	}
	State struct {
		Running bool
	}
}

func VerifyComposeService(ctx context.Context, project, service string, req Requirements) error {
	id, err := composeServiceContainerID(ctx, project, service)
	if err != nil {
		return err
	}

	raw, err := runtimeCommand(ctx, containerRuntime(), "container", "inspect", id).Output()
	if err != nil {
		return fmt.Errorf("inspect %s/%s: %w", project, service, err)
	}
	var records []inspectRecord
	if err := json.Unmarshal(raw, &records); err != nil {
		return fmt.Errorf("decode inspect for %s/%s: %w", project, service, err)
	}
	if len(records) != 1 {
		return fmt.Errorf("inspect for %s/%s returned %d records", project, service, len(records))
	}
	r := records[0]
	if !r.State.Running {
		return fmt.Errorf("%s/%s is not running", project, service)
	}
	if isRootUser(r.Config.User) && !rootlessPodman() {
		return fmt.Errorf("%s/%s runs as root (Config.User=%q)", project, service, r.Config.User)
	}
	if r.HostConfig.Privileged {
		return fmt.Errorf("%s/%s is privileged", project, service)
	}
	if len(r.HostConfig.CapAdd) != 0 {
		return fmt.Errorf("%s/%s adds Linux capabilities: %v", project, service, r.HostConfig.CapAdd)
	}
	if req.ReadOnlyRootfs && !r.HostConfig.ReadonlyRootfs {
		return fmt.Errorf("%s/%s root filesystem is writable", project, service)
	}
	if req.DropAllCaps {
		if containerRuntime() == "podman" {
			if len(r.EffectiveCaps) != 0 {
				return fmt.Errorf("%s/%s retains effective capabilities under Podman: %v", project, service, r.EffectiveCaps)
			}
		} else if !containsFold(r.HostConfig.CapDrop, "ALL") {
			return fmt.Errorf("%s/%s does not drop ALL capabilities: %v", project, service, r.HostConfig.CapDrop)
		}
	}
	if req.NoNewPrivs && !containsSecurityOpt(r.HostConfig.SecurityOpt, "no-new-privileges") {
		return fmt.Errorf("%s/%s does not enable no-new-privileges: %v", project, service, r.HostConfig.SecurityOpt)
	}
	return nil
}

func composeServiceContainerID(ctx context.Context, project, service string) (string, error) {
	runtime := containerRuntime()
	if runtime != "podman" {
		idOut, err := runtimeCommand(ctx, runtime, "ps", "-q",
			"--filter", "label=com.docker.compose.project="+project,
			"--filter", "label=com.docker.compose.service="+service,
		).Output()
		if err != nil {
			return "", fmt.Errorf("list %s/%s container: %w", project, service, err)
		}
		ids := strings.Fields(string(idOut))
		if len(ids) == 0 {
			return "", fmt.Errorf("running container for %s/%s not found", project, service)
		}
		if len(ids) != 1 {
			return "", fmt.Errorf("multiple running containers found for %s/%s", project, service)
		}
		return ids[0], nil
	}

	idOut, err := runtimeCommand(ctx, runtime, "container", "ls", "-aq").Output()
	if err != nil {
		return "", fmt.Errorf("list running Podman containers: %w", err)
	}
	var matches []string
	var observed []string
	for _, id := range strings.Fields(string(idOut)) {
		raw, inspectErr := runtimeCommand(ctx, runtime, "container", "inspect", id).Output()
		if inspectErr != nil {
			return "", fmt.Errorf("inspect running Podman container %s: %w", id, inspectErr)
		}
		var records []inspectRecord
		if err := json.Unmarshal(raw, &records); err != nil {
			return "", fmt.Errorf("decode running Podman container %s: %w", id, err)
		}
		if len(records) != 1 {
			return "", fmt.Errorf("inspect running Podman container %s returned %d records", id, len(records))
		}
		labels := records[0].Config.Labels
		projectLabel := labels["com.docker.compose.project"]
		if projectLabel == "" {
			projectLabel = labels["io.podman.compose.project"]
		}
		serviceLabel := labels["com.docker.compose.service"]
		if serviceLabel == "" {
			serviceLabel = labels["io.podman.compose.service"]
		}
		if len(observed) < 64 {
			observed = append(observed, projectLabel+"/"+serviceLabel)
		}
		if records[0].State.Running && projectLabel == project && serviceLabel == service {
			matches = append(matches, id)
		}
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("running container for %s/%s not found; inspected Podman ownership labels: %v", project, service, observed)
	}
	if len(matches) != 1 {
		return "", fmt.Errorf("multiple running containers found for %s/%s", project, service)
	}
	return matches[0], nil
}

func isRootUser(user string) bool {
	user = strings.TrimSpace(user)
	if user == "" {
		return true
	}
	name, _, _ := strings.Cut(user, ":")
	return name == "0" || strings.EqualFold(name, "root")
}

func containsFold(values []string, want string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), want) {
			return true
		}
	}
	return false
}

func containsSecurityOpt(values []string, want string) bool {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == want || strings.HasPrefix(value, want+":") {
			return true
		}
	}
	return false
}

func rootlessPodman() bool {
	if containerRuntime() != "podman" {
		return false
	}
	out, err := runtimeCommand(context.Background(), "podman", "info", "--format", "{{.Host.Security.Rootless}}").Output()
	if err != nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(string(out)), "true")
}

func containerRuntime() string {
	if runtime := strings.TrimSpace(os.Getenv("BASEHARBOR_TEST_RUNTIME")); runtime == "docker" || runtime == "podman" {
		return runtime
	}
	if err := runtimeCommand(context.Background(), "docker", "info").Run(); err == nil {
		return "docker"
	}
	if err := runtimeCommand(context.Background(), "podman", "info").Run(); err == nil {
		return "podman"
	}
	return "docker"
}

// Match the production Runtime adapter: BaseHarbor's isolated XDG paths must
// not select a different rootless Podman store. Keep XDG_RUNTIME_DIR, which
// identifies the actual user session and runtime socket.
func runtimeCommand(ctx context.Context, runtime string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, runtime, args...)
	if runtime == "podman" {
		cmd.Env = []string{}
		for _, entry := range os.Environ() {
			if strings.HasPrefix(entry, "XDG_CONFIG_HOME=") || strings.HasPrefix(entry, "XDG_DATA_HOME=") {
				continue
			}
			cmd.Env = append(cmd.Env, entry)
		}
	}
	return cmd
}
