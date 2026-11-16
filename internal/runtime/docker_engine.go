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

// rootlessSocketPath is the Rootless Docker socket of a uid. Tests replace
// the rootlessSocketExists variable to simulate hosts with and without that
// daemon.
func rootlessSocketPath(uid int) string {
	return filepath.Join("/run/user", strconv.Itoa(uid), "docker.sock")
}

// systemSocketPath is the System (rootful) Docker daemon socket.
const systemSocketPath = "/var/run/docker.sock"

// rootlessSocketExists reports whether a Docker daemon socket is present.
// Tests replace this variable to simulate hosts with and without a Rootless
// daemon. It is a presence check, never an engine probe: production code
// still resolves and verifies exactly one endpoint through the CLI.
var rootlessSocketExists = func(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode()&os.ModeSocket != 0
}

// rootlessCapable reports whether this host can run Rootless Docker at all:
// it needs unprivileged user namespaces. A host that denies them has no
// Rootless daemon anywhere and its active Docker is System Docker (GitHub-
// hosted runners, per gh-ci-core run 41255597778). Tests replace this
// variable to simulate such system-only hosts.
//
// We check three signals, in order of specificity:
//  1. AppArmor restricts unprivileged user namespaces (kernel.apparmor_*
//     restrict_unprivileged_userns or /proc/sys/kernel/unprivileged_userns_clone);
//     when blocked → definitely NOT capable.
//  2. max_user_namespaces exists and is non-zero → user namespaces are
//     allowed by the kernel, but AppArmor may still block them; the
//     AppArmor check above is the gatekeeper.
//  3. If neither knob exists the old heuristic assumed capability; we now
//     conservatively require at least one namespace knob to be present.
var rootlessCapable = func() bool {
	// 1. AppArmor gate: kernel.apparmor_restrict_unprivileged_userns
	//    (sysctl knob) — 1 means blocked, 0 means allowed. This is the
	//    decisive signal on hardened / GitHub-hosted runners.
	data, err := os.ReadFile("/proc/sys/kernel/apparmor_restrict_unprivileged_userns")
	if err == nil {
		if strings.TrimSpace(string(data)) != "0" {
			return false // AppArmor blocks unprivileged user namespaces
		}
		// AppArmor knob exists and allows → continue to namespace check
	}
	// Fallback: older kernels expose unprivileged_userns_clone directly.
	data, err = os.ReadFile("/proc/sys/kernel/unprivileged_userns_clone")
	if err == nil && strings.TrimSpace(string(data)) == "0" {
		return false // kernel-level block
	}
	// 2. Namespace knob must exist for user namespaces to be usable.
	if _, err := os.Stat("/proc/sys/user/max_user_namespaces"); err != nil {
		return false // no namespace limit knob → cannot run user namespaces
	}
	return true
}

// hostDefaultEndpoint resolves the fully unconfigured host default the way
// the Docker CLI does. Rootless Docker is a per-user installation needing
// unprivileged user namespaces; its socket wins wherever Rootless can exist
// on this host. A root session and a host that denies those namespaces
// entirely (system-only hosts such as GitHub runners) have no Rootless
// option and their active daemon is System Docker. This never probes an
// engine; the returned endpoint is verified once by the caller.
func hostDefaultEndpoint(uid int) string {
	if uid <= 0 {
		return "unix://" + systemSocketPath
	}
	if !rootlessSocketExists(rootlessSocketPath(uid)) && !rootlessCapable() {
		return "unix://" + systemSocketPath
	}
	return "unix://" + rootlessSocketPath(uid)
}

// resolveDockerEngine determines the active Docker endpoint from Target,
// environment, and the CLI's own context, in that precedence, and probes
// that one endpoint exactly. When nothing is configured — no Target
// endpoint or context, no DOCKER_HOST or DOCKER_CONTEXT, no explicit
// docker-mode — the CLI's own context selection decides: the "default"
// context carries no socket of its own, so it stands for the daemon the
// host actually runs. The documented host default, Rootless Docker, needs
// unprivileged user namespaces and its per-user socket; where that socket
// is present, or the host cannot prove otherwise, it stays the default.
// Where the socket is absent and the host denies unprivileged user
// namespaces, Rootless Docker is impossible there and the active daemon is
// System Docker (GitHub-hosted runners, per gh-ci-core run 41255597778).
// An explicit docker-mode, Target endpoint or context, or DOCKER_*
// variable never takes this branch: it stays on its configured path,
// probes exactly one engine and fails closed when unreachable.
func resolveDockerEngine(ctx context.Context, command string, selection DockerEngineSelection, run func(context.Context, string, []string, []string) ([]byte, error), getenv func(string) string, uid int) (DockerEngineObservation, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	result := DockerEngineObservation{Mode: "unknown"}
	mode := strings.TrimSpace(selection.Mode)
	modeDeclared := mode != ""
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
	// Precedence: explicit Target endpoint or context, DOCKER_CONTEXT,
	// DOCKER_HOST, then the active Docker context of the CLI. A declared
	// docker-mode is an intent the selected engine must satisfy. Without
	// one BaseHarbor follows the engine the active context actually runs;
	// exactly one endpoint is probed below and an unreachable or
	// unidentifiable daemon fails closed. No path ever probes a second
	// engine as a fallback.
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
		if !modeDeclared {
			// The active context decides the engine, not a hardcoded
			// Rootless path. Rootless Docker is a per-user installation
			// needing unprivileged user namespaces; it wins exactly where
			// it can run on this host. The "default" context carries no
			// socket of its own, so it resolves from the host: a host with
			// no rootless socket whose kernel denies those namespaces
			// cannot run Rootless Docker anywhere and its active daemon,
			// the one `docker run hello-world` uses, is System Docker
			// (GitHub-hosted runners, per gh-ci-core run 41255597778). A
			// rootless-capable host without a running rootless daemon
			// stays fail-closed on its rootless socket, as documented.
			// A named context resolves through `docker context inspect`
			// below and reports its actual mode. Either way the chosen
			// endpoint is probed exactly once and its real mode verified,
			// so nothing is silently substituted and no path ever probes
			// a second engine as a fallback. A declared docker-mode never
			// takes this branch; it stays a fail-closed Target intent.
			if selectedContext == "default" {
				endpoint = hostDefaultEndpoint(uid)
				selectedContext = ""
				result.SelectionOrigin = "active-default"
			}
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
	// The engine's real mode is what the single verified probe answered.
	// A declared docker-mode is a Target intent that must match reality;
	// a mismatch fails closed with no fallback and no automatic
	// migration. The one intended exception is the fully unconfigured
	// host default ("active-default"): its contract is to use the host's
	// active daemon, and hostDefaultEndpoint only leaves the documented
	// Rootless path where the host proves Rootless cannot run at all
	// (system-only hosts); elsewhere it stays on the rootless socket and
	// fails closed. A rootful answer from any other selection never
	// passes quietly for an undeclared rootless expectation.
	if result.Mode != mode && !(mode == "rootless" && !modeDeclared && result.SelectionOrigin == "active-default") {
		return result, fmt.Errorf("Docker endpoint %s is %s, but Target requires %s; no engine fallback or automatic migration; explicitly configure docker-mode: rootful only to maintain an existing rootful installation", endpoint, result.Mode, mode)
	}
	result.Verified = true
	return result, nil
}

func dockerBoundEnvironment(env []string, endpoint string) []string {
	result := make([]string, 0, len(env)+1)
	for _, entry := range env {
		// DOCKER_CONTEXT must go: docker honours it over DOCKER_HOST, which
		// would silently point a later probe at another daemon.
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
