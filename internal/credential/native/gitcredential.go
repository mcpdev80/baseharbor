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
// credential for a host. It follows the same line protocol git uses for the
// `credential helper` flow and never returns the secret value to the caller.
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

// Resolve queries the git credential helper for host. A successful helper reply
// yields resolved=true; an absent helper, a non-zero exit, or an empty reply
// fails closed with ErrGitCredentialUnavailable. The secret payload is never
// surfaced: callers only learn whether a source resolved, never its value.
func (s GitCredentialSource) Resolve(ctx context.Context, host string) (resolved bool, err error) {
	if strings.TrimSpace(host) == "" {
		return false, fmt.Errorf("git: empty host")
	}

	args := []string{"credential", "fill", "https", host}
	name := "git"
	if strings.TrimSpace(s.Helper) != "" {
		name = s.Helper
	}

	if _, runErr := s.runner().Invoke(ctx, name, args...); runErr != nil {
		return false, gitUnavailable(runErr)
	}
	return true, nil
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
