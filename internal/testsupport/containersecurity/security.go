package containersecurity

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

type Requirements struct {
	ReadOnlyRootfs bool
	DropAllCaps    bool
	NoNewPrivs     bool
}

type inspectRecord struct {
	Config struct {
		User string
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
	idOut, err := exec.CommandContext(ctx, "docker", "ps", "-q",
		"--filter", "label=com.docker.compose.project="+project,
		"--filter", "label=com.docker.compose.service="+service,
	).Output()
	if err != nil {
		return fmt.Errorf("list %s/%s container: %w", project, service, err)
	}
	id := strings.TrimSpace(string(idOut))
	if id == "" {
		return fmt.Errorf("running container for %s/%s not found", project, service)
	}
	if strings.Contains(id, "\n") {
		return fmt.Errorf("multiple running containers found for %s/%s", project, service)
	}

	raw, err := exec.CommandContext(ctx, "docker", "inspect", id).Output()
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
	if isRootUser(r.Config.User) {
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
	if req.DropAllCaps && !containsFold(r.HostConfig.CapDrop, "ALL") {
		return fmt.Errorf("%s/%s does not drop ALL capabilities: %v", project, service, r.HostConfig.CapDrop)
	}
	if req.NoNewPrivs && !containsSecurityOpt(r.HostConfig.SecurityOpt, "no-new-privileges") {
		return fmt.Errorf("%s/%s does not enable no-new-privileges: %v", project, service, r.HostConfig.SecurityOpt)
	}
	return nil
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
