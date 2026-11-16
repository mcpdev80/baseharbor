package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/deployment"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// This repair is strictly opt-in and limited to the local, unconfigured Docker
// bootstrap. Explicit Targets, remote runtimes and Podman are never rewritten.
func rootlessBootstrapEligible(target deployment.ResolvedTarget, uid int, getenv func(string) string) bool {
	if runtime.GOOS != "linux" || uid <= 0 || strings.TrimSpace(target.RuntimeProvider) != "docker" ||
		(target.AccessProvider != "" && target.AccessProvider != "local") ||
		target.DockerMode == "rootful" || target.DockerEndpoint != "" || target.DockerContext != "" ||
		getenv("DOCKER_HOST") != "" || getenv("DOCKER_CONTEXT") != "" {
		return false
	}
	return true
}

func runRootlessSetupStep(ctx context.Context, in io.Reader, out io.Writer, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = in
	cmd.Stdout = out
	cmd.Stderr = out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return nil
}

func rootlessUbuntuOrDebian() bool {
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		if line == "ID=ubuntu" || line == "ID=debian" || line == "ID=\"ubuntu\"" || line == "ID=\"debian\"" {
			return true
		}
	}
	return false
}

// maybeRepairRootlessDocker is intentionally invoked only by the human-guided
// Core bootstrap, never by the generic runtime resolver or an application
// apply. A plain --yes is not approval for host package/service changes.
func maybeRepairRootlessDocker(ctx context.Context, in io.Reader, out io.Writer, target deployment.ResolvedTarget) error {
	if !rootlessBootstrapEligible(target, os.Getuid(), os.Getenv) {
		return nil
	}
	socket := filepath.Join(os.Getenv("XDG_RUNTIME_DIR"), "docker.sock")
	if os.Getenv("XDG_RUNTIME_DIR") == "" {
		socket = fmt.Sprintf("/run/user/%d/docker.sock", os.Getuid())
	}
	// The normal resolver is authoritative. In particular, system-only hosts
	// may legitimately resolve the active/default engine as rootful; they
	// must never be forced through rootless repair.
	observation, resolveErr := bhruntime.ResolveDockerEngine(targetDockerEngineContext(ctx, target), "docker")
	if resolveErr == nil || observation.Endpoint != "unix://"+socket {
		return nil
	}
	if info, err := os.Stat(socket); err == nil && info.Mode()&os.ModeSocket != 0 {
		// Existing sockets are handled by the normal verified resolver. A
		// daemon that fails its probe must not trigger a blind reinstall.
		return nil
	}
	// Only the default Docker context qualifies: a named context is user
	// intent even when the Target did not specify one.
	current, err := exec.CommandContext(ctx, "docker", "context", "show").Output()
	if err != nil || strings.TrimSpace(string(current)) != "default" {
		return nil
	}
	if _, err := exec.LookPath("dockerd-rootless-setuptool.sh"); err != nil {
		return fmt.Errorf("Rootless Docker socket %s is missing and its setup tool is not installed; install Docker rootless extras, then retry", socket)
	}
	if !rootlessUbuntuOrDebian() {
		return fmt.Errorf("Rootless Docker socket %s is missing; automatic repair is currently limited to Ubuntu/Debian; no changes made", socket)
	}
	needUIDMap := false
	for _, binary := range []string{"newuidmap", "newgidmap"} {
		if _, err := exec.LookPath(binary); err != nil {
			needUIDMap = true
		}
	}
	fmt.Fprintln(out, "Rootless Docker is not running at "+socket+".")
	fmt.Fprintln(out, "The selected local Docker Target requires an isolated user daemon; System Docker will not be used as a fallback.")
	fmt.Fprintln(out, "Proposed changes (only after your separate approval):")
	if needUIDMap {
		fmt.Fprintln(out, "  - Install the missing Ubuntu/Debian uidmap package with sudo (newuidmap/newgidmap for user namespaces).")
	}
	fmt.Fprintln(out, "  - Run dockerd-rootless-setuptool.sh install --force for your user; --force allows the existing System Docker to keep running.")
	fmt.Fprintln(out, "  - Enable and start the user-owned Docker service with systemctl --user.")
	fmt.Fprintln(out, "  - Restore the original Docker CLI context and verify the rootless engine. Existing System Docker stays unchanged.")
	if err := requireRootlessSetupConsent(ctx, in, out, !noInput(ctx) && readerIsTerminal(in)); err != nil {
		return err
	}
	if needUIDMap {
		if err := runRootlessSetupStep(ctx, in, out, "sudo", "apt-get", "install", "-y", "uidmap"); err != nil {
			return fmt.Errorf("install required uidmap package: %w", err)
		}
		for _, binary := range []string{"newuidmap", "newgidmap"} {
			if _, err := exec.LookPath(binary); err != nil {
				return fmt.Errorf("%s remains unavailable after approved installation: %w", binary, err)
			}
		}
	}
	// setup tool changes the active Docker CLI context as a side effect.
	// Restore the recorded context even if the installation fails.
	defer func() {
		_ = runRootlessSetupStep(context.Background(), in, io.Discard, "docker", "context", "use", "default")
	}()
	if err := runRootlessSetupStep(ctx, in, out, "dockerd-rootless-setuptool.sh", "install", "--force"); err != nil {
		return fmt.Errorf("configure Rootless Docker: %w", err)
	}
	if err := runRootlessSetupStep(ctx, in, out, "systemctl", "--user", "enable", "--now", "docker"); err != nil {
		return fmt.Errorf("start Rootless Docker user service: %w", err)
	}
	// The setup tool switches contexts; verify the original/default Target
	// after restoring it, never mistake the new context for Target intent.
	if err := runRootlessSetupStep(ctx, in, io.Discard, "docker", "context", "use", "default"); err != nil {
		return fmt.Errorf("restore Docker default context: %w", err)
	}
	engine, err := bhruntime.ResolveDockerEngine(ctx, "docker")
	if err != nil {
		return fmt.Errorf("Rootless Docker setup finished but engine verification failed: %w", err)
	}
	if !engine.Verified || engine.Mode != "rootless" || engine.Endpoint != "unix://"+socket {
		return fmt.Errorf("Rootless Docker verification returned unexpected mode/endpoint: mode=%s endpoint=%s", engine.Mode, engine.Endpoint)
	}
	fmt.Fprintln(out, "[OK] Rootless Docker is ready; continuing Core setup.")
	return nil
}

// Approval is deliberately independent of --yes: neither unattended calls
// nor generic CLI confirmations grant permission to modify the host.
func requireRootlessSetupConsent(ctx context.Context, in io.Reader, out io.Writer, interactive bool) error {
	if !interactive {
		return errors.New("Rootless Docker setup requires an interactive terminal and explicit approval; no system changes made")
	}
	approved, err := promptYesNo(orasPromptReader(in), out, "Apply these Rootless Docker setup changes?", false)
	if err != nil {
		return fmt.Errorf("Rootless Docker setup not approved: %w", err)
	}
	if !approved {
		return errors.New("Rootless Docker setup declined; no system changes made")
	}
	return nil
}
