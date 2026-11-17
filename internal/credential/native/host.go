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

// HostFromURL derives the authentication host from a git remote reference. It
// accepts the forms git uses in practice: https://host[/path],
// ssh://[user@]host[/path], git@host:path, and host:path. It returns "" when no
// host can be parsed so callers treat an unparseable source as "no native
// source" and keep their previous behavior.
func HostFromURL(source string) string {
	source = strings.TrimSpace(source)
	if source == "" {
		return ""
	}
	switch {
	case strings.HasPrefix(source, "git@"):
		rest := strings.TrimPrefix(source, "git@")
		if idx := strings.IndexByte(rest, ':'); idx >= 0 {
			rest = rest[:idx]
		}
		return hostOnly(rest)
	case strings.HasPrefix(strings.ToLower(source), "ssh://"):
		return hostOnly(strings.TrimPrefix(source, "ssh://"))
	case strings.HasPrefix(source, "https://"):
		return hostOnly(strings.TrimPrefix(source, "https://"))
	case strings.HasPrefix(strings.ToLower(source), "http://"):
		return hostOnly(strings.TrimPrefix(source, "http://"))
	default:
		return ""
	}
}

// hostOnly strips userinfo, a trailing :port, and any /path suffix from a host
// reference, returning the bare host.
func hostOnly(reference string) string {
	reference = strings.TrimSpace(reference)
	if reference == "" {
		return ""
	}
	if at := strings.IndexByte(reference, '@'); at >= 0 {
		reference = reference[at+1:]
	}
	// Strip any trailing :port.
	if idx := strings.LastIndexByte(reference, ':'); idx > 0 {
		reference = reference[:idx]
	}
	// Strip any trailing /path (https/ssh remotes).
	if idx := strings.IndexByte(reference, '/'); idx >= 0 {
		reference = reference[:idx]
	}
	return reference
}

// PreInteractiveNoPrompt answers one question about a git operation: can a
// native source (git credential helper or ssh agent) service host without any
// interactive prompt? It returns nativeHit=true only when a native source is
// present; a miss, an absent source, or a broken source all report nativeHit=false.
//
// The result is a foretaste about interactivity only — it never reports that
// authentication succeeded. Callers that answer yes must start the git command
// non-interactively (for example by exporting GIT_TERMINAL_PROMPT=0) so no
// prompt is possible; callers that answer no keep their previous behavior
// unchanged. The resolved secret material is never returned: this mirrors the
// documented broker contract and never leaks material through the result.
func PreInteractiveNoPrompt(ctx context.Context, host string) (nativeHit bool, err error) {
	return PreInteractiveNoPromptWithRunner(ctx, host, DefaultRunner)
}

// PreInteractiveNoPromptWithRunner is PreInteractiveNoPrompt with an injectable
// CommandRunner for tests. See PreInteractiveNoPrompt for the contract.
func PreInteractiveNoPromptWithRunner(ctx context.Context, host string, runner CommandRunner) (nativeHit bool, err error) {
	host = strings.TrimSpace(host)
	if host == "" {
		return false, nil
	}

	git := GitCredentialSource{Run: runner}
	if resolved, gerr := git.Resolve(ctx, host); gerr == nil && resolved {
		return true, nil
	}

	ssh := SSHAgentSource{Run: runner}
	if has, serr := ssh.HasKey(ctx, host); serr == nil && has {
		return true, nil
	}

	return false, nil
}
