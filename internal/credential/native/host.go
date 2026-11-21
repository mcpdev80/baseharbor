package native

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
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
//
// The child is forced fully non-interactive: every inherited git/ssh prompt
// channel is stripped from the child environment and the non-interactive
// sentinels are pinned, so a missing credential helper, an empty reply, or a
// helper that would otherwise wait on the controlling terminal fails closed
// instantly instead of hanging on a password prompt. This keeps the native
// probe fail-closed on a host with no native credential source.
func (ExecRunner) Invoke(ctx context.Context, name string, stdin string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = strings.NewReader(stdin)
	if fn := ttyCommandHook.Load(); fn != nil {
		// Test-only attachment of a controlling terminal; production never
		// sets the hook and the vector below is unchanged either way.
		(*fn)(cmd)
	}
	cmd.Env = appendNonInteractiveEnv(cmd.Env)
	out, err := cmd.Output()
	if err != nil {
		return out, exitStatusError(name, err)
	}
	return out, nil
}

// appendNonInteractiveEnv establishes the non-interactive child environment.
//
// Appending the sentinels alone is NOT enough. The child vector starts from
// the inherited process environment (os.Environ when the caller set none),
// and a duplicated key in an execve vector resolves by a platform rule no
// caller may depend on: glibc getenv returns the FIRST copy while the shells
// and Go's exec de-duplicate toward the LAST, and git itself consults both
// channels in different stages of its credential lookup. Round 1 appended
// GIT_TERMINAL_PROMPT=0 / GIT_ASKPASS=/bin/false / SSH_ASKPASS=/bin/false
// over the inherited vector and left the helper-override channel open at
// all; on the card's bootstrap host a waiting prompt script (or a helper
// that reaches one) stayed reachable and `git credential fill` hung the
// first authenticated git operation. Every inherited instance of every
// prompt channel is therefore removed first and the authoritative
// non-interactive values are appended exactly once, so the vector is
// unambiguous on every platform. The rest of the environment stays intact
// (HOME/PATH the helper needs), and the process environment itself is never
// mutated, so concurrent callers are unaffected.
func appendNonInteractiveEnv(base []string) []string {
	env := base
	if env == nil {
		env = os.Environ()
	}
	env = removeEnvKeys(env, promptEnvKeys()...)
	env = append(env,
		"GIT_TERMINAL_PROMPT=0",
		"SSH_ASKPASS=/bin/false",
		"GCM_INTERACTIVE=never",
		"GIT_CREDENTIAL_HELPER=/bin/false",
		"GIT_ASKPASS=/bin/false",
	)
	return env
}

// EnvFor exposes the production establishment for the witnesses that build a
// command vector around the real ExecRunner path (e.g. after attaching a
// controlling terminal). Production code never calls it.
func EnvFor(base []string) []string {
	return appendNonInteractiveEnv(base)
}

// CredentialFillEnvSickForTest is the round-1 establishment kept for the
// mutation-M1 witness: the sentinels APPENDED over the inherited vector,
// never stripping inherited copies, in round-1 order. It also leaves the
// helper-override channel GIT_CREDENTIAL_HELPER completely unhandled — the
// gap this round closes. Production code never calls it.
func CredentialFillEnvSickForTest(base []string) []string {
	env := base
	if env == nil {
		env = os.Environ()
	}
	return append(env,
		"GIT_TERMINAL_PROMPT=0",
		"GIT_ASKPASS=/bin/false",
		"SSH_ASKPASS=/bin/false",
	)
}

// promptEnvKeys returns the inherited environment keys that can open a
// git/ssh credential prompt channel. GIT_CREDENTIAL_HELPER is part of the
// set: git consults it as the helper-override channel BEFORE the configured
// helper stage, so an inherited value pointing at an asking helper keeps the
// prompt path reachable even when every prompt variable is pinned.
func promptEnvKeys() []string {
	return []string{
		"GIT_TERMINAL_PROMPT",
		"GIT_ASKPASS",
		"SSH_ASKPASS",
		"GCM_INTERACTIVE",
		"GCM_CREDENTIAL_STORE",
		"GIT_SSH",
		"GIT_SSH_COMMAND",
		"GIT_CREDENTIAL_HELPER",
	}
}

// removeEnvKeys drops every "key=..." entry whose key appears in keys, keeping
// the remaining entries in their original order.
func removeEnvKeys(env []string, keys ...string) []string {
	drop := make(map[string]bool, len(keys))
	for _, k := range keys {
		drop[k] = true
	}
	kept := make([]string, 0, len(env))
	for _, entry := range env {
		if i := strings.IndexByte(entry, '='); i > 0 && drop[entry[:i]] {
			continue
		}
		kept = append(kept, entry)
	}
	return kept
}

// ttyCommandHook lets a test attach a real controlling terminal to the
// command the production runner builds (stdin/stdout/stderr are wired by the
// hook before Start). Production never sets it; when unset the runner runs
// exactly as on any host.
var ttyCommandHook atomic.Pointer[func(*exec.Cmd)]

// SetTTYCommandHookForTest installs (fn != nil) or clears (fn == nil) the
// terminal hook used by the controlling-terminal witness. It exists so the
// red-first test drives the PRODUCTION ExecRunner and its environment
// establishment instead of a test-local copy of them.
func SetTTYCommandHookForTest(fn func(*exec.Cmd)) {
	if fn == nil {
		ttyCommandHook.Store(nil)
		return
	}
	ttyCommandHook.Store(&fn)
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
