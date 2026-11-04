package native

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// CommandRunner executes an external native command with a bounded stdin. It
// isolates every os/exec call behind one seam so tests can substitute
// deterministic output without touching the network, the agent, or any host
// state.
type CommandRunner interface {
	Invoke(ctx context.Context, name string, stdin string, args ...string) ([]byte, error)
}

// ExecRunner is the production CommandRunner backed by os/exec.
type ExecRunner struct{}

// Invoke runs name+args with the given stdin and returns its stdout. On failure
// the error carries only a redacted class and exit status; the raw stderr is
// never surfaced so helper payloads and secrets cannot leak through it.
func (ExecRunner) Invoke(ctx context.Context, name string, stdin string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = strings.NewReader(stdin)
	out, err := cmd.Output()
	if err != nil {
		return out, exitStatusError(name, err)
	}
	return out, nil
}

// DefaultRunner is the production runner backed by os/exec.
var DefaultRunner CommandRunner = ExecRunner{}

// runner returns the effective runner for a source, defaulting to os/exec when
// the source did not inject one for tests.
func (s GitCredentialSource) runner() CommandRunner {
	if s.Run != nil {
		return s.Run
	}
	return DefaultRunner
}

// runner returns the effective runner for a source, defaulting to os/exec when
// the source did not inject one for tests.
func (s SSHAgentSource) runner() CommandRunner {
	if s.Run != nil {
		return s.Run
	}
	return DefaultRunner
}

// sshUnavailable wraps a runner error as an ssh-unavailable documented case.
func sshUnavailable(err error) error {
	if err == nil {
		return ErrSSHAgentUnavailable
	}
	return fmt.Errorf("%w: %v", ErrSSHAgentUnavailable, err)
}

// exitStatusError reports an external-command failure with only a redacted class
// and exit status. The raw stderr is dropped so helper payloads, secrets, or
// other host material never reach the returned error.
func exitStatusError(name string, err error) error {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
		return fmt.Errorf("%s: command failed with a non-zero exit status", name)
	}
	return fmt.Errorf("%s: command failed", name)
}
