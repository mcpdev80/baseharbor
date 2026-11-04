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

// Available reports whether an SSH agent is configured via SSH_AUTH_SOCK.
//
// It is a fund-only gate: it answers ONLY whether the variable is set, never
// whether a live socket file exists at that path. Whether a usable key is
// present is decided fund-only by the injected runner in HasKey, never by the
// host environment. Requiring a stat-able socket here would make the SSH branch
// depend on whether a socket FILE happens to exist on the runner — exactly the
// environment-dependent behaviour this gate exists to prevent (a found key must
// be reported regardless of the agent's filesystem state). Absence is a
// documented, fail-closed condition: no variable means no agent.
func (s SSHAgentSource) Available(ctx context.Context) bool {
	agent := s.envVar("SSH_AUTH_SOCK")
	return strings.TrimSpace(agent) != ""
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
