package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// DockerEngineSelection is explicit Target intent, never application environment.
type DockerEngineSelection struct{ Endpoint, Context, Mode string }
type dockerEngineSelectionKey struct{}

func WithDockerEngineSelection(ctx context.Context, selection DockerEngineSelection) context.Context {
	return context.WithValue(ctx, dockerEngineSelectionKey{}, selection)
}

type DockerEngineObservation struct {
	Endpoint        string `json:"endpoint"`
	Context         string `json:"context,omitempty"`
	Mode            string `json:"mode"`
	DaemonID        string `json:"daemon_id,omitempty"`
	SelectionOrigin string `json:"selection_origin"`
	Verified        bool   `json:"verified"`
}

// ResolveDockerEngine resolves once and probes that exact socket. It never
// probes another engine as a fallback after a failed selection.
func ResolveDockerEngine(ctx context.Context, command string) (DockerEngineObservation, error) {
	selection, _ := ctx.Value(dockerEngineSelectionKey{}).(DockerEngineSelection)
	return resolveDockerEngine(ctx, command, selection, dockerEngineOutput, os.Getenv, os.Getuid())
}

func dockerEngineOutput(ctx context.Context, command string, args []string, env []string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Env = env
	return cmd.Output()
}

func resolveDockerEngine(ctx context.Context, command string, selection DockerEngineSelection, run func(context.Context, string, []string, []string) ([]byte, error), getenv func(string) string, uid int) (DockerEngineObservation, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	result := DockerEngineObservation{Mode: "unknown"}
	mode := strings.TrimSpace(selection.Mode)
	if mode == "" {
		mode = "rootless"
	}
	if mode != "rootless" && mode != "rootful" {
		return result, fmt.Errorf("Docker mode must be rootless or explicitly rootful")
	}
	if selection.Endpoint != "" && selection.Context != "" {
		return result, fmt.Errorf("select one Target Docker endpoint or context")
	}
	endpoint, selectedContext := strings.TrimSpace(selection.Endpoint), strings.TrimSpace(selection.Context)
	switch {
	case endpoint != "":
		result.SelectionOrigin = "target-endpoint"
	case selectedContext != "":
		result.SelectionOrigin = "target-context"
	case strings.TrimSpace(getenv("DOCKER_CONTEXT")) != "":
		selectedContext = strings.TrimSpace(getenv("DOCKER_CONTEXT"))
		result.SelectionOrigin = "environment-context"
	case strings.TrimSpace(getenv("DOCKER_HOST")) != "":
		endpoint = strings.TrimSpace(getenv("DOCKER_HOST"))
		result.SelectionOrigin = "environment-endpoint"
	default:
		output, err := run(ctx, command, []string{"context", "show"}, os.Environ())
		if err != nil {
			return result, fmt.Errorf("cannot resolve active Docker context; configure the Target's Docker endpoint or context")
		}
		selectedContext = strings.TrimSpace(string(output))
		result.SelectionOrigin = "active-context"
		if selectedContext == "" {
			return result, fmt.Errorf("active Docker context is empty")
		}
		if selectedContext == "default" && mode == "rootless" {
			if uid <= 0 {
				return result, fmt.Errorf("rootless Docker requires a non-root user and an explicit rootless context")
			}
			endpoint = "unix://" + filepath.Join("/run/user", strconv.Itoa(uid), "docker.sock")
			selectedContext = ""
			result.SelectionOrigin = "rootless-default"
		}
	}
	if selectedContext != "" {
		if strings.HasPrefix(selectedContext, "-") || strings.ContainsAny(selectedContext, "\x00\r\n") {
			return result, fmt.Errorf("invalid Docker context name")
		}
		output, err := run(ctx, command, []string{"context", "inspect", selectedContext, "--format", "{{json .Endpoints.docker.Host}}"}, os.Environ())
		if err != nil || json.Unmarshal(output, &endpoint) != nil {
			return result, fmt.Errorf("Docker context %q endpoint could not be resolved; no engine fallback", selectedContext)
		}
	}
	result.Endpoint, result.Context = endpoint, selectedContext
	if !strings.HasPrefix(endpoint, "unix:///") || strings.ContainsAny(endpoint, "\x00\r\n") || strings.Contains(endpoint, "?") || strings.Contains(endpoint, "#") {
		return result, fmt.Errorf("local Docker Target requires an absolute unix socket endpoint; use authenticated Node access for remote engines")
	}
	output, err := run(ctx, command, []string{"--host", endpoint, "info", "--format", "{{json .}}"}, dockerBoundEnvironment(os.Environ(), endpoint))
	if err != nil {
		return result, fmt.Errorf("selected Docker endpoint %s is unavailable; start its rootless daemon or correct the Target endpoint/context; no fallback to System Docker", endpoint)
	}
	var info struct {
		ID              string
		SecurityOptions []string
	}
	if json.Unmarshal(output, &info) != nil || strings.TrimSpace(info.ID) == "" {
		return result, fmt.Errorf("Docker endpoint %s returned no verifiable daemon identity", endpoint)
	}
	result.Mode = "rootful"
	for _, option := range info.SecurityOptions {
		if option == "rootless" || option == "name=rootless" {
			result.Mode = "rootless"
		}
	}
	result.DaemonID = info.ID
	if result.Mode != mode {
		return result, fmt.Errorf("Docker endpoint %s is %s, but Target requires %s; no engine fallback or automatic migration; explicitly configure docker-mode: rootful only to maintain an existing rootful installation", endpoint, result.Mode, mode)
	}
	result.Verified = true
	return result, nil
}

func dockerBoundEnvironment(env []string, endpoint string) []string {
	result := make([]string, 0, len(env)+1)
	for _, entry := range env {
		if !strings.HasPrefix(entry, "DOCKER_HOST=") && !strings.HasPrefix(entry, "DOCKER_CONTEXT=") {
			result = append(result, entry)
		}
	}
	return append(result, "DOCKER_HOST="+endpoint)
}

func NewDockerCLIBackend(command string, engine DockerEngineObservation) Compose {
	return Compose{command: command, prefix: []string{"--host", engine.Endpoint, "compose"}, engineArgs: []string{"--host", engine.Endpoint}, dockerEngine: &engine}
}

func (c Compose) DockerEngine() *DockerEngineObservation {
	if c.dockerEngine == nil {
		return nil
	}
	value := *c.dockerEngine
	return &value
}

func DockerEngineForProvider(provider any) *DockerEngineObservation {
	if bound, ok := provider.(interface {
		DockerEngine() *DockerEngineObservation
	}); ok {
		return bound.DockerEngine()
	}
	return nil
}

func (c Compose) commandEnvironment(overrides map[string]string) ([]string, error) {
	if c.dockerEngine == nil {
		if overrides == nil {
			return runtimeCommandEnv(c.command), nil
		}
		return mergeProcessEnvironment(overrides)
	}
	for key := range overrides {
		if strings.HasPrefix(key, "DOCKER_") {
			return nil, fmt.Errorf("application environment cannot override the selected Docker engine: %s", key)
		}
	}
	base := runtimeCommandEnv(c.command)
	result := make([]string, 0, len(base)+len(overrides))
	for _, entry := range base {
		key, _, _ := strings.Cut(entry, "=")
		if _, ok := overrides[key]; !ok {
			result = append(result, entry)
		}
	}
	for key, value := range overrides {
		if key == "" || strings.ContainsAny(key, "=\x00") || strings.ContainsRune(value, 0) {
			return nil, fmt.Errorf("invalid runtime environment variable")
		}
		result = append(result, key+"="+value)
	}
	return dockerBoundEnvironment(result, c.dockerEngine.Endpoint), nil
}
