package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"runtime"
	"strconv"
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
	socket := fmt.Sprintf("/run/user/%d/docker.sock", os.Getuid())
	// The normal resolver is authoritative. In particular, system-only hosts
	// may legitimately resolve the active/default engine as rootful; they
	// must never be forced through rootless repair.
	observation, resolveErr := bhruntime.ResolveDockerEngine(targetDockerEngineContext(ctx, target), "docker")
	if resolveErr == nil || observation.Endpoint != "unix://"+socket {
		return nil
	}
	_, socketErr := os.Stat(socket)
	if socketErr != nil && !errors.Is(socketErr, os.ErrNotExist) {
		return fmt.Errorf("inspect selected Rootless Docker socket %s: %w", socket, socketErr)
	}
	// A stopped user service can have no socket. Do not attempt to
	// reinstall it: enabling an existing service is the idempotent repair.
	hasUserUnit := exec.CommandContext(ctx, "systemctl", "--user", "cat", "docker").Run() == nil
	needsInstall := errors.Is(socketErr, os.ErrNotExist) && !hasUserUnit
	// Only the default Docker context qualifies: a named context is user
	// intent even when the Target did not specify one.
	current, err := exec.CommandContext(ctx, "docker", "context", "show").Output()
	if err != nil || strings.TrimSpace(string(current)) != "default" {
		return nil
	}
	if needsInstall {
		if _, err := exec.LookPath("dockerd-rootless-setuptool.sh"); err != nil {
			return fmt.Errorf("Rootless Docker socket %s is missing and its setup tool is not installed; install Docker rootless extras, then retry", socket)
		}
		if !rootlessUbuntuOrDebian() {
			return fmt.Errorf("Rootless Docker socket %s is missing; automatic repair is currently limited to Ubuntu/Debian; no changes made", socket)
		}
		if err := checkRootlessSubIDs(); err != nil {
			return err
		}
	}
	if _, err := exec.LookPath("systemctl"); err != nil {
		return fmt.Errorf("Rootless Docker setup requires a systemd user service: %w", err)
	}
	needUIDMap := false
	if needsInstall {
		for _, binary := range []string{"newuidmap", "newgidmap"} {
			if _, err := exec.LookPath(binary); err != nil {
				needUIDMap = true
			}
		}
	}
	fmt.Fprintln(out, "Rootless Docker is not running at "+socket+".")
	fmt.Fprintln(out, "The selected local Docker Target requires an isolated user daemon; System Docker will not be used as a fallback.")
	fmt.Fprintln(out, "Proposed changes (only after your separate approval):")
	if needUIDMap {
		fmt.Fprintln(out, "  - Install the missing Ubuntu/Debian uidmap package with sudo (newuidmap/newgidmap for user namespaces).")
	}
	if needsInstall {
		fmt.Fprintln(out, "  - Run dockerd-rootless-setuptool.sh install --force for your user; --force permits the existing System Docker to keep running.")
	} else {
		fmt.Fprintln(out, "  - Start or restart the existing Rootless Docker user service; do not reinstall Docker.")
	}
	if needsInstall {
		fmt.Fprintln(out, "  - Enable and start the user-owned Docker service with systemctl --user.")
	}
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
	if needsInstall {
		// Rootless setup changes the CLI context; restore it before verifying
		// the originally selected Target, including when installation fails.
		defer func() {
			_ = runRootlessSetupStep(context.Background(), in, io.Discard, "docker", "context", "use", "default")
		}()
		if err := runRootlessSetupStep(ctx, in, out, "dockerd-rootless-setuptool.sh", "install", "--force"); err != nil {
			return fmt.Errorf("configure Rootless Docker: %w", err)
		}
		if err := runRootlessSetupStep(ctx, in, out, "systemctl", "--user", "enable", "--now", "docker"); err != nil {
			return fmt.Errorf("start Rootless Docker user service: %w", err)
		}
	} else {
		if err := runRootlessSetupStep(ctx, in, out, "systemctl", "--user", "enable", "--now", "docker"); err != nil {
			return fmt.Errorf("start existing Rootless Docker user service: %w", err)
		}
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


// Refuse to allocate UID/GID ranges automatically: guessing a free range
// could overlap an existing account. Docker's rootless setup requires both
// mappings to be configured for the current user beforehand.
func checkRootlessSubIDs() error {
	current, err := user.Current()
	if err != nil || current.Username == "" {
		return fmt.Errorf("cannot determine user for Rootless Docker subordinate ID mappings: %v", err)
	}
	for _, name := range []string{"subuid", "subgid"} {
		contents, err := os.ReadFile("/etc/" + name)
		if err != nil {
			return fmt.Errorf("read Rootless Docker subordinate ID mappings /etc/%s: %w", name, err)
		}
		if !hasRootlessSubIDRange(string(contents), current.Username) {
			return fmt.Errorf("Rootless Docker requires a subordinate ID allocation for %s in /etc/%s (at least 65536 IDs); configure a non-overlapping allocation with your system administrator, then rerun baha up; no system changes made", current.Username, name)
		}
	}
	return nil
}

func hasRootlessSubIDRange(contents, username string) bool {
	for _, line := range strings.Split(contents, "\n") {
		fields := strings.Split(strings.TrimSpace(line), ":")
		if len(fields) != 3 || fields[0] != username {
			continue
		}
		start, startErr := strconv.ParseUint(fields[1], 10, 32)
		count, countErr := strconv.ParseUint(fields[2], 10, 32)
		if startErr == nil && countErr == nil && start > 0 && count >= 65536 && start+count <= 4294967296 {
			return true
		}
	}
	return false
}
