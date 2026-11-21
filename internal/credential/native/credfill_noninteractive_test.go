package native_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/credential/native"
)

// TestGitCredentialFillFailsClosedNamesHost proves the documented-absent case
// fails closed with a host-named "no credential" error (never a timeout/panic
// form) when the helper fails closed.
func TestGitCredentialFillFailsClosedNamesHost(t *testing.T) {
	f := &fakeRunner{runErr: errors.New("exit 1")}
	src := native.GitCredentialSource{Run: f}

	_, err := src.Resolve(context.Background(), "github.example.com")
	if err == nil {
		t.Fatal("no credential for a host with an unresolving helper must fail closed")
	}
	msg := err.Error()
	if !strings.Contains(msg, "github.example.com") {
		t.Fatalf("error must name the failing host, got %q", msg)
	}
	for _, forbidden := range []string{"timeout", "oom", "panic", "hang"} {
		if strings.Contains(strings.ToLower(msg), forbidden) {
			t.Fatalf("error must not read as a %s, got %q", forbidden, msg)
		}
	}
	if !errors.Is(err, native.ErrGitCredentialUnavailable) {
		t.Fatalf("want ErrGitCredentialUnavailable in the chain, got %v", err)
	}
}

// zzCredentialProbeTimeout bounds every credential-fill probe. A hang must
// FAIL the test at this deadline; it must never block the suite.
const zzCredentialProbeTimeout = 15 * time.Second

// TestExecRunnerEnvSentinelsWinInChildVector is the vector-level guarantee
// the whole non-interactivity contract rests on: the production establishment
// (appendNonInteractiveEnv, applied by ExecRunner.Invoke) removes EVERY
// inherited copy of the git/ssh prompt channels — including the helper-
// override channel GIT_CREDENTIAL_HELPER — and pins each authoritative value
// exactly once, so the CHILD's own environment snapshot (read back through
// /proc/self/environ) holds one authoritative value per channel and the
// bootstrap state cannot survive into git under any duplicate-resolution
// rule. HOME/PATH stay intact for the helper.
func TestExecRunnerEnvSentinelsWinInChildVector(t *testing.T) {
	dir := t.TempDir()
	probe := filepath.Join(dir, "env-probe.sh")
	body := "#!/bin/sh\ncat /proc/self/environ | tr '\\0' '\\n'\n"
	if err := os.WriteFile(probe, []byte(body), 0o755); err != nil {
		t.Fatalf("write probe: %v", err)
	}
	t.Setenv("GIT_TERMINAL_PROMPT", "1")
	t.Setenv("GIT_ASKPASS", filepath.Join(dir, "inherited-waiting-prompt.sh"))
	t.Setenv("SSH_ASKPASS", filepath.Join(dir, "inherited-waiting-prompt.sh"))
	t.Setenv("GIT_CREDENTIAL_HELPER", filepath.Join(dir, "inherited-asking-helper.sh"))
	t.Setenv("GIT_SSH_COMMAND", "ssh -o ProxyCommand="+filepath.Join(dir, "inherited-waiting-prompt.sh"))

	var out bytes.Buffer
	cmd := exec.CommandContext(context.Background(), probe)
	cmd.Env = native.EnvFor(os.Environ())
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("probe run: %v", err)
	}
	seen := map[string][]string{}
	for _, line := range strings.Split(out.String(), "\n") {
		if k, v, ok := strings.Cut(line, "="); ok {
			seen[k] = append(seen[k], v)
		}
	}
	for key, want := range map[string]string{
		"GIT_TERMINAL_PROMPT":   "0",
		"GIT_ASKPASS":           "/bin/false",
		"SSH_ASKPASS":           "/bin/false",
		"GIT_CREDENTIAL_HELPER": "/bin/false",
		"GCM_INTERACTIVE":       "never",
	} {
		values := seen[key]
		if len(values) != 1 {
			t.Fatalf("child must see exactly one %s, saw %v", key, values)
		}
		if values[0] != want {
			t.Fatalf("child must see %s=%s, saw %s (inherited prompt channel leaked into the child)", key, want, values[0])
		}
	}
	if _, ok := seen["GIT_SSH_COMMAND"]; ok {
		t.Fatalf("inherited GIT_SSH_COMMAND prompt channel leaked into the child: %v", seen["GIT_SSH_COMMAND"])
	}
}

// TestRound1VectorKeepsPromptChannelOpen is the mutation-M1 witness on git's
// OWN prompt/askpass path: it execve's `git credential fill` with the exact
// vector shape round 1 put on the wire — the inherited hostile copies survive
// (waiting GIT_ASKPASS script + GIT_TERMINAL_PROMPT=1) and the three round-1
// sentinels are APPENDED behind them (round-1 order: TERMINAL, ASKPASS,
// SSH_ASKPASS). syscall.ForkExec passes the vector to execve WITHOUT
// de-duplication, which is what the round-1 establishment actually handed
// the operating system; Go's own exec layer de-duplicates duplicates toward
// the last copy and would hide the defect (pinned by the calibration log
// this witness prints). Git must REACH its prompt path on that vector: the
// waiting script executes (fabricating a credential — the card's fail-open)
// or the probe hangs to the deadline (the card's hang). Either outcome is
// RED for round 1 and GREEN for the fix, which strips every inherited copy
// first so no duplicate can revive a prompt channel.
func TestRound1VectorKeepsPromptChannelOpen(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	if runtime.GOOS != "linux" {
		t.Skip("raw execve vector witness is implemented for Linux")
	}
	gitbin, err := exec.LookPath("git")
	if err != nil {
		t.Skipf("git binary: %v", err)
	}
	dir := t.TempDir()
	promptLog := filepath.Join(dir, "askpass.log")
	prompt := zzRecordingPrompt(t, filepath.Join(dir, "prompt.sh"), promptLog)
	gitconfig := filepath.Join(dir, ".gitconfig")
	if err := os.WriteFile(gitconfig, []byte("[credential]\n\thelper = /bin/false\n"), 0o600); err != nil {
		t.Fatalf("write gitconfig: %v", err)
	}
	reqf := filepath.Join(dir, "req")
	if err := os.WriteFile(reqf, []byte("protocol=https\nhost=example.invalid\n\n"), 0o644); err != nil {
		t.Fatalf("write request: %v", err)
	}
	outf, err := os.Create(filepath.Join(dir, "out"))
	if err != nil {
		t.Fatalf("open out: %v", err)
	}
	defer outf.Close()
	// The vector through the round-1 establishment builder itself: a clean
	// inherited base carrying the hostile prompt channels, round-1 order.
	base := []string{
		"PATH=" + zzPath(),
		"HOME=" + dir,
		"TERM=dumb",
		"XDG_CONFIG_HOME=" + filepath.Join(dir, "xdg"),
		"GIT_CONFIG_GLOBAL=" + gitconfig,
		"GIT_CONFIG_NOSYSTEM=1",
		"SSH_AUTH_SOCK=" + filepath.Join(dir, "absent.sock"),
		"GIT_ASKPASS=" + prompt,
		"SSH_ASKPASS=" + prompt,
		"GIT_TERMINAL_PROMPT=1",
	}
	vector := native.CredentialFillEnvSickForTest(base)
	rf, err := os.Open(reqf)
	if err != nil {
		t.Fatalf("open request: %v", err)
	}
	defer rf.Close()
	pid, err := syscall.ForkExec(gitbin, []string{"git", "credential", "fill"}, &syscall.ProcAttr{
		Dir:   dir,
		Env:   vector,
		Files: []uintptr{rf.Fd(), outf.Fd(), outf.Fd()},
	})
	if err != nil {
		t.Fatalf("fork+exec git: %v", err)
	}
	waited := make(chan syscall.WaitStatus, 1)
	go func() {
		var ws syscall.WaitStatus
		_, _ = syscall.Wait4(pid, &ws, 0, nil)
		waited <- ws
	}()
	var hung bool
	select {
	case <-waited:
	case <-time.After(zzCredentialProbeTimeout):
		hung = true
		_ = syscall.Kill(pid, syscall.SIGKILL)
		<-waited
	}
	out, _ := os.ReadFile(filepath.Join(dir, "out"))
	data, _ := os.ReadFile(promptLog)
	// Guard: the probe must not hang forever in CI — kill any survivor.
	if !hung && len(data) == 0 {
		t.Fatalf("mutation M1: the round-1 vector did NOT reach git's prompt path via raw execve on this "+
			"platform (askpass script silent, no hang); out=%q — the red-first claim must then rely on the "+
			"develop-baseline witness in the card evidence", strings.TrimSpace(string(out)))
	}
	t.Logf("M1 red as required: round-1 vector reaches git's prompt path (hung=%v); git asked:\n%s",
		hung, strings.TrimSpace(string(data)))
}

// TestGitCredentialFillIsNonInteractiveProvesTheFix is the red-first witness
// for card 0428-217 on git's OWN prompt/askpass path (the reviewer recipe:
// explicit GIT_ASKPASS script + GIT_TERMINAL_PROMPT + a context deadline that
// turns a hang into a FAIL), driven through the production seam
// (GitCredentialSource.Resolve → ExecRunner.Invoke → git credential fill).
//
// The child inherits the bootstrap-host state the card names: prompting
// enabled in the parent environment plus a waiting GIT_ASKPASS password
// prompt (the script records each invocation, answers the first prompt once
// so git advances, then blocks — nobody is at the keyboard). A helper-
// override channel (GIT_CREDENTIAL_HELPER) points at a helper that consumes
// the fill request and then asks for the password through the environment's
// prompt machinery — the credential lookup that hangs the first
// authenticated git operation.
//
// With the FIX: appendNonInteractiveEnv strips every inherited copy of every
// prompt channel (including the helper override) and pins the authoritative
// sentinels exactly once, git cannot prompt anywhere, the script never
// executes, and the probe fails closed with the host-named no-credential
// error inside the deadline.
//
// Its RED counterpart without the fix is proven twice: by
// TestRound1VectorKeepsPromptChannelOpen above (the round-1 vector reaches
// git's prompt path) and by the develop-baseline witness recorded in the
// card evidence (the same tests hang to the deadline at develop 40cb28c75,
// where Invoke set no environment at all).
func TestGitCredentialFillIsNonInteractiveProvesTheFix(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	promptLog := filepath.Join(dir, "askpass.log")
	prompt := zzRecordingPrompt(t, filepath.Join(dir, "prompt.sh"), promptLog)
	helper := zzAskingHelper(t, filepath.Join(dir, "asking-helper.sh"))
	gitconfig := filepath.Join(dir, ".gitconfig")
	if err := os.WriteFile(gitconfig, []byte("[credential]\n\thelper = /bin/false\n"), 0o600); err != nil {
		t.Fatalf("write gitconfig: %v", err)
	}
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	t.Setenv("GIT_CONFIG_GLOBAL", gitconfig)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("SSH_AUTH_SOCK", filepath.Join(dir, "absent.sock"))
	t.Setenv("GIT_CREDENTIAL_HELPER", helper)
	t.Setenv("GIT_ASKPASS", prompt)
	t.Setenv("SSH_ASKPASS", prompt)
	t.Setenv("GIT_TERMINAL_PROMPT", "1")

	src := native.GitCredentialSource{}
	resolved, err := zzResolveWithDeadline(t, src, "example.invalid")
	data, _ := os.ReadFile(promptLog)
	if resolved {
		t.Fatalf("probe resolved credential material through the inherited prompt channel — the production "+
			"establishment left git's own prompt path open (card 0428-217); git asked:\n%s", string(data))
	}
	if errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("git credential fill hung on git's own prompt path (card 0428-217); git asked:\n%s err=%v", string(data), err)
	}
	if len(data) != 0 {
		t.Fatalf("git reached an inherited prompt channel, which the production establishment must close (card 0428-217);\ncalls:\n%s", string(data))
	}
	msg := fmt.Sprint(err)
	if !strings.Contains(msg, "example.invalid") {
		t.Fatalf("probe must fail closed with the host named, got %q", msg)
	}
	if !errors.Is(err, native.ErrGitCredentialUnavailable) {
		t.Fatalf("want ErrGitCredentialUnavailable in the chain, got %v", err)
	}
}

// TestGitCredentialFillIsNonInteractiveUnderControllingTerminal is the PTY
// witness the review recipe asks for: the production seam starts git with a
// REAL controlling terminal attached (the developer-shell state during
// target bootstrap), and GIT_TERMINAL_PROMPT is UNSET in the parent so only
// production code can disable prompting in the child. The credential helper
// consumes the fill request and then reads the password from the controlling
// terminal — the hang shape made deterministic: nobody types that password.
//
//   - without the fix: the child inherits no suppression, the helper's
//     terminal read waits forever, and the context deadline FAILS the test
//     (the develop-baseline hang; at 40cb28c75 this witness failed at the
//     20s deadline);
//   - with the fix: the production vector pins GIT_TERMINAL_PROMPT=0 (and
//     neutralizes the helper-override channel), the terminal read fails
//     closed, nothing resolves, and the probe returns the host-named
//     no-credential error well inside the deadline.
func TestGitCredentialFillIsNonInteractiveUnderControllingTerminal(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	if runtime.GOOS != "linux" {
		t.Skip("controlling-terminal witness is implemented for Linux")
	}
	dir := t.TempDir()
	helper := filepath.Join(dir, "tty-prompt-helper.sh")
	hbody := "#!/bin/sh\n" +
		"cat >/dev/null\n" +
		"stty echo >/dev/null 2>&1 || exit 1\n" +
		"printf 'Password for https://example.invalid: ' >&2\n" +
		"read -r p || exit 0\n" +
		"printf 'password=%s\\n' \"$p\"\n"
	if err := os.WriteFile(helper, []byte(hbody), 0o755); err != nil {
		t.Fatalf("write helper: %v", err)
	}
	gitconfig := filepath.Join(dir, ".gitconfig")
	if err := os.WriteFile(gitconfig, []byte("[credential]\n\thelper = "+helper+"\n"), 0o600); err != nil {
		t.Fatalf("write gitconfig: %v", err)
	}
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	t.Setenv("GIT_CONFIG_GLOBAL", gitconfig)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("SSH_AUTH_SOCK", filepath.Join(dir, "absent.sock"))
	os.Unsetenv("GIT_TERMINAL_PROMPT")
	os.Unsetenv("GIT_ASKPASS")
	os.Unsetenv("SSH_ASKPASS")
	os.Unsetenv("GIT_CREDENTIAL_HELPER")

	ptyUnavailable.Store(false)
	native.SetTTYCommandHookForTest(attachControllingPTY(t))
	t.Cleanup(func() { native.SetTTYCommandHookForTest(nil) })

	src := native.GitCredentialSource{}
	resolved, err := zzResolveWithDeadline(t, src, "example.invalid")
	if ptyUnavailable.Load() {
		t.Skip("cannot allocate a PTY in this environment")
	}
	if !ttyPromptBranchWasLive {
		t.Fatalf("the PTY hook never ran: the witness did not exercise a controlling terminal")
	}
	if errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("git credential fill hung on the terminal password prompt under a controlling "+
			"terminal (card 0428-217): the production vector did not disable git's prompt path: %v", err)
	}
	if resolved {
		t.Fatalf("probe resolved on the terminal prompt path (card 0428-217): resolved=true err=%v", err)
	}
	msg := fmt.Sprint(err)
	if !strings.Contains(msg, "example.invalid") {
		t.Fatalf("probe must fail closed with the host named, got %q", msg)
	}
}

// zzRecordingPrompt writes the askpass witness script: it records every
// invocation (so the test can prove git reached its own prompt path) and
// answers immediately — deterministic, never blocking.
func zzRecordingPrompt(t *testing.T, path, log string) string {
	t.Helper()
	body := "#!/bin/sh\n" +
		"echo \"CALL $1 AP=${GIT_ASKPASS:-unset}\" >> " + log + "\n" +
		"printf 'x\\n'\n"
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("write prompt script: %v", err)
	}
	return path
}

// zzAskingHelper writes the bootstrap-shape helper: consumes the fill request,
// then asks for the password through git's documented prompt channel
// (GIT_ASKPASS when offered, else the controlling terminal).
func zzAskingHelper(t *testing.T, path string) string {
	t.Helper()
	body := "#!/bin/sh\n" +
		"cat >/dev/null\n" +
		"if [ -n \"${GIT_ASKPASS:-}\" ] && [ -x \"${GIT_ASKPASS}\" ]; then\n" +
		"  printf 'password=%s\\n' \"$(${GIT_ASKPASS} \"Password for 'https://example.invalid': \")\"\n" +
		"  exit 0\n" +
		"fi\n" +
		"stty echo >/dev/null 2>&1 || exit 1\n" +
		"printf 'Password for https://example.invalid: ' >&2\n" +
		"read -r p || exit 0\n" +
		"printf 'password=%s\\n' \"$p\"\n"
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("write helper: %v", err)
	}
	return path
}

// zzPath returns the test process PATH (kept intact for git itself).
func zzPath() string {
	if p := os.Getenv("PATH"); p != "" {
		return p
	}
	return "/usr/bin:/bin"
}

// zzResolveWithDeadline runs src.Resolve under a context deadline so a hang
// fails the test at the bound instead of blocking the suite forever.
func zzResolveWithDeadline(t *testing.T, src native.GitCredentialSource, host string) (bool, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), zzCredentialProbeTimeout)
	t.Cleanup(cancel)
	type outcome struct {
		resolved bool
		err      error
	}
	done := make(chan outcome, 1)
	go func() {
		r, err := src.Resolve(ctx, host)
		done <- outcome{r, err}
	}()
	select {
	case o := <-done:
		return o.resolved, o.err
	case <-time.After(zzCredentialProbeTimeout + 5*time.Second):
		t.Fatalf("git credential fill blocked past the probe deadline (uninterruptible child)")
		return false, nil
	}
}
