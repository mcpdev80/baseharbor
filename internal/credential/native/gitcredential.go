// Package native provides bounded adapters that reuse existing host-native
// authentication sources instead of prompting for new credentials. It never
// implements an authentication provider and never wires a cloud credential
// chain; it only detects and forwards material already present on the host
// (git credential helper, SSH agent) or returns an explicit documented error
// when a source is absent. Session-only material is not persisted; protected
// temp files are cleaned on cancel or error.
package native

import (
	"context"
	"fmt"
	"strings"
)

// GitCredentialSource reuses the configured git credential helper to resolve a
// credential for a host. It speaks the `git credential fill` line protocol: the
// request goes on stdin, resolved key=value lines come back on stdout, and the
// secret payload is never returned to the caller.
type GitCredentialSource struct {
	// Helper is the resolved helper command prefix. When empty the source
	// defers to the host configuration (git credential <action>).
	Helper string
	// Run executes a native command. Injectable for tests; when nil the real
	// os/exec runner is used.
	Run CommandRunner
}

// ErrGitCredentialUnavailable documents the documented-absent case: no helper is
// configured and no credential can be resolved from a native source.
var ErrGitCredentialUnavailable = fmt.Errorf("git: no native credential helper available for host")

// Resolve queries the git credential helper for an https://host entry. A
// successful helper reply yields resolved=true; an absent helper, a non-zero
// exit, or an empty reply fails closed with ErrGitCredentialUnavailable. The
// secret payload is never surfaced: callers only learn whether a source
// resolved, never its value.
func (s GitCredentialSource) Resolve(ctx context.Context, host string) (resolved bool, err error) {
	host = strings.TrimSpace(host)
	if host == "" {
		return false, fmt.Errorf("git: empty host")
	}

	// The git credential helper protocol carries the request on stdin, not on
	// the command line. Send a protocol=https/host line and read the resolved
	// credential back on stdout; a missing helper or non-zero exit fails closed.
	in := "protocol=https\nhost=" + host + "\n"
	name, args := s.helperCommand()
	out, runErr := s.runner().Invoke(ctx, name, in, args...)

	if runErr != nil {
		return false, gitUnavailable(runErr)
	}
	if len(resolvedField(out, "username")) == 0 && len(resolvedField(out, "password")) == 0 {
		return false, ErrGitCredentialUnavailable
	}
	return true, nil
}

// helperCommand returns the helper name and its argument prefix. The `fill`
// action is passed to the helper only when a custom helper prefix is configured
// (helper names may not accept the action token); the default `git` binary
// requires the action on the command line.
func (s GitCredentialSource) helperCommand() (string, []string) {
	if helper := strings.TrimSpace(s.Helper); helper != "" {
		return helper, []string{"fill"}
	}
	return "git", []string{"credential", "fill"}
}

// Cleanup is a no-op for the git source: it persists nothing. Retained for the
// Source interface so callers clean uniformly.
func (s GitCredentialSource) Cleanup() error { return nil }

func gitUnavailable(err error) error {
	if err == nil {
		return ErrGitCredentialUnavailable
	}
	return fmt.Errorf("%w: %v", ErrGitCredentialUnavailable, err)
}

// resolvedField extracts the value of a key=value line from the helper reply,
// returning nil when the key is absent. The caller decides whether the value is
// significant; the raw reply is never surfaced.
func resolvedField(out []byte, key string) []byte {
	for _, line := range strings.Split(string(out), "\n") {
		if i := strings.IndexByte(line, '='); i >= 0 {
			if strings.TrimSpace(line[:i]) == key {
				return []byte(strings.TrimSpace(line[i+1:]))
			}
		}
	}
	return nil
}
