package native

import (
	"context"
	"fmt"
	"os"
	"strings"
)

// SSHAgentSource reuses an existing SSH agent (or SSH_AUTH_SOCK) for a host's
// key-based authentication. It never creates keys, starts a new agent, or
// prompts for a passphrase; it only reports whether a usable agent is present
// and forwards the request through the native agent.
type SSHAgentSource struct {
	// Env reads the process environment. Injectable for tests. When nil the
	// real os.Getenv is used.
	Env func(string) string
	// Run executes a native command. Injectable for tests; when nil the real
	// os/exec runner is used.
	Run CommandRunner
}

// ErrSSHAgentUnavailable documents the documented-absent case: no SSH agent is
// available (no SSH_AUTH_SOCK) so key-based authentication cannot be reused
// natively.
var ErrSSHAgentUnavailable = fmt.Errorf("ssh: no usable ssh-agent (SSH_AUTH_SOCK unset)")

// Available reports whether an SSH agent is reachable. Absence is a documented,
// fail-closed condition, not a silent success.
func (s SSHAgentSource) Available(ctx context.Context) bool {
	agent := s.envVar("SSH_AUTH_SOCK")
	if strings.TrimSpace(agent) == "" {
		return false
	}
	// A socket file must exist; a missing path means the agent is gone.
	if _, err := os.Stat(agent); err != nil {
		return false
	}
	return true
}

// HasKey reports whether the agent carries a key usable for host auth. It runs
// `ssh-add -L` and reports success only when at least one public key is listed.
// The key material itself is never returned; callers learn only presence or
// absence.
func (s SSHAgentSource) HasKey(ctx context.Context, _ string) (bool, error) {
	if !s.Available(ctx) {
		return false, ErrSSHAgentUnavailable
	}
	out, runErr := s.runner().Invoke(ctx, "ssh-add", "-L")
	if runErr != nil {
		return false, sshUnavailable(runErr)
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.TrimSpace(line) != "" {
			return true, nil
		}
	}
	return false, nil
}

// Cleanup is a no-op for the SSH source: the agent persists outside this call
// and session-only material is never written here.
func (s SSHAgentSource) Cleanup() error { return nil }

func (s SSHAgentSource) envVar(name string) string {
	if s.Env != nil {
		return s.Env(name)
	}
	return os.Getenv(name)
}
