package native

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
)

// CommandRunner executes an external native command with a bounded stdin. It
// isolates every os/exec call behind one seam so tests can substitute
// deterministic output without touching the network, the agent, or any host
// state.
type CommandRunner interface {
	Invoke(ctx context.Context, name string, args ...string) ([]byte, error)
}

// ExecRunner is the production CommandRunner backed by os/exec.
type ExecRunner struct{}

// Invoke runs name+args and returns its stdout. Stderr is attached to the error
// on failure for diagnosis without leaking secrets (helper payloads are never
// echoed back).
func (ExecRunner) Invoke(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return out, fmt.Errorf("%s: %v (stderr: %s)", name, err, string(exitErr.Stderr))
		}
		return out, err
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
