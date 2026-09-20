package application

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	WorkloadSecurityModeEnv  = "BASEHARBOR_WORKLOAD_SECURITY_MODE"
	WorkloadSecurityAllowEnv = "BASEHARBOR_WORKLOAD_SECURITY_ALLOW"
)

type WorkloadSecurityDecision string

const (
	WorkloadSecurityAllow WorkloadSecurityDecision = "allow"
	WorkloadSecurityWarn  WorkloadSecurityDecision = "warn"
	WorkloadSecurityDeny  WorkloadSecurityDecision = "deny"
)

type WorkloadSecurityFinding struct {
	Code     string                   `json:"code"`
	Decision WorkloadSecurityDecision `json:"decision"`
	Service  string                   `json:"service"`
	Field    string                   `json:"field"`
	Value    string                   `json:"value,omitempty"`
	Message  string                   `json:"message"`
}

type WorkloadSecurityReport struct {
	Mode     string                    `json:"mode"`
	Findings []WorkloadSecurityFinding `json:"findings"`
}

func (r WorkloadSecurityReport) Denied() bool {
	for _, finding := range r.Findings {
		if finding.Decision == WorkloadSecurityDeny {
			return true
		}
	}
	return false
}

func (r WorkloadSecurityReport) Error() error {
	if !r.Denied() {
		return nil
	}
	data, _ := json.Marshal(r)
	return fmt.Errorf("repository workload violates BaseHarbor isolation policy: %s", data)
}

type renderedSecurityCompose struct {
	Services map[string]struct {
		Privileged  bool                 `json:"privileged"`
		NetworkMode string               `json:"network_mode"`
		PID         string               `json:"pid"`
		IPC         string               `json:"ipc"`
		CapAdd      []string             `json:"cap_add"`
		Devices     []any                `json:"devices"`
		Volumes     []renderedComposeMount `json:"volumes"`
	} `json:"services"`
}

type renderedComposeMount struct {
	Type   string `json:"type"`
	Source string `json:"source"`
	Target string `json:"target"`
}

func AnalyzeRenderedComposeSecurity(m Manifest, rendered []byte) (WorkloadSecurityReport, error) {
	mode, err := workloadSecurityMode(m)
	if err != nil {
		return WorkloadSecurityReport{}, err
	}
	allowed := map[string]bool{}
	if raw := strings.TrimSpace(os.Getenv(WorkloadSecurityAllowEnv)); raw != "" {
		if mode != "development" {
			return WorkloadSecurityReport{}, fmt.Errorf("%s is permitted only in development mode", WorkloadSecurityAllowEnv)
		}
		for _, value := range strings.Split(raw, ",") {
			code := strings.TrimSpace(strings.ToLower(value))
			if code != "" {
				allowed[code] = true
			}
		}
	}

	var config renderedSecurityCompose
	if err := json.Unmarshal(rendered, &config); err != nil {
		return WorkloadSecurityReport{}, fmt.Errorf("decode rendered Compose security model: %w", err)
	}
	report := WorkloadSecurityReport{Mode: mode}
	services := make([]string, 0, len(config.Services))
	for service := range config.Services {
		services = append(services, service)
	}
	sort.Strings(services)
	for _, service := range services {
		definition := config.Services[service]
		add := func(code, field, value, message string, deviceOnly bool) {
			decision := WorkloadSecurityDeny
			if mode == "development" && deviceOnly {
				decision = WorkloadSecurityWarn
			}
			if mode == "development" && allowed[code] {
				decision = WorkloadSecurityAllow
			}
			report.Findings = append(report.Findings, WorkloadSecurityFinding{
				Code: code, Decision: decision, Service: service, Field: field, Value: value, Message: message,
			})
		}
		if definition.Privileged {
			add("privileged", "privileged", "true", "privileged containers bypass the documented workload isolation boundary", false)
		}
		if strings.EqualFold(strings.TrimSpace(definition.NetworkMode), "host") {
			add("host-network", "network_mode", definition.NetworkMode, "host networking bypasses BaseHarbor network isolation", false)
		}
		if strings.EqualFold(strings.TrimSpace(definition.PID), "host") {
			add("host-pid", "pid", definition.PID, "host PID namespace sharing bypasses process isolation", false)
		}
		if strings.EqualFold(strings.TrimSpace(definition.IPC), "host") {
			add("host-ipc", "ipc", definition.IPC, "host IPC namespace sharing bypasses workload isolation", false)
		}
		for _, capability := range definition.CapAdd {
			capability = strings.ToUpper(strings.TrimSpace(capability))
			if dangerousLinuxCapability(capability) {
				add("dangerous-capability", "cap_add", capability, "Linux capability can materially weaken the workload isolation boundary", false)
			}
		}
		if len(definition.Devices) > 0 {
			add("host-device", "devices", fmt.Sprintf("%d device mapping(s)", len(definition.Devices)), "host device access expands workload privileges and requires explicit development acknowledgement", true)
		}
		for _, mount := range definition.Volumes {
			if strings.ToLower(strings.TrimSpace(mount.Type)) != "bind" {
				continue
			}
			source := filepath.Clean(strings.TrimSpace(mount.Source))
			if isRuntimeSocket(source) {
				add("runtime-socket", "volumes", source, "Docker/Podman runtime socket access is a host-control-plane escape path", false)
				continue
			}
			if criticalHostMount(source) {
				add("critical-host-mount", "volumes", source, "critical host filesystem bind mount bypasses BaseHarbor filesystem isolation", false)
			}
		}
	}
	return report, nil
}

func workloadSecurityMode(m Manifest) (string, error) {
	if raw := strings.ToLower(strings.TrimSpace(os.Getenv(WorkloadSecurityModeEnv))); raw != "" {
		switch raw {
		case "development", "managed":
			return raw, nil
		default:
			return "", fmt.Errorf("%s must be development or managed", WorkloadSecurityModeEnv)
		}
	}
	switch strings.ToLower(strings.TrimSpace(m.Environment)) {
	case "dev", "development":
		return "development", nil
	default:
		return "managed", nil
	}
}

func dangerousLinuxCapability(value string) bool {
	switch value {
	case "ALL", "SYS_ADMIN", "SYS_MODULE", "SYS_PTRACE", "NET_ADMIN", "SYS_RAWIO", "DAC_READ_SEARCH", "DAC_OVERRIDE", "MKNOD", "SETUID", "SETGID":
		return true
	default:
		return false
	}
}

func isRuntimeSocket(source string) bool {
	source = filepath.Clean(source)
	switch source {
	case "/var/run/docker.sock", "/run/docker.sock", "/run/podman/podman.sock":
		return true
	}
	return strings.HasSuffix(source, "/podman/podman.sock")
}

func criticalHostMount(source string) bool {
	source = filepath.Clean(source)
	if source == "/" {
		return true
	}
	for _, root := range []string{"/etc", "/run", "/var/run", "/dev"} {
		if source == root || strings.HasPrefix(source, root+string(os.PathSeparator)) {
			return true
		}
	}
	return false
}
