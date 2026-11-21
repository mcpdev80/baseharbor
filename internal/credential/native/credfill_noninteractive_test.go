package native_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/credential/native"
)

// TestGitCredentialFillFailsClosedNamesHost proves the documented-absent case
// fails closed with a host-named "no credential" error (never a timeout/panic
// form) when the helper fails closed, and that the sentinel is reachable.
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

// TestGitCredentialFillIsNonInteractiveProvesTheFix is the red-first witness: a
// git credential HELPER that blocks on the controlling terminal simulates the
// production bootstrap-hang. On the sick code the probe hangs; with the fix the
// helper cannot open a TTY (git's stdin is a non-interactive Reader) and the
// probe fails closed with a host-named no-credential error.
func TestGitCredentialFillIsNonInteractiveProvesTheFix(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatalf("mkdir home: %v", err)
	}
	// A helper that consumes the protocol request then tries a password prompt
	// on the tty. Without a non-interactive child this reproduces the hang.
	helper := filepath.Join(dir, "block-helper.sh")
	body := "#!/bin/sh\n" +
		"cat >/dev/null\n" +
		"read -r pw < /dev/tty 2>/dev/null || true\n" +
		"printf '\\n'\n"
	if err := os.WriteFile(helper, []byte(body), 0o755); err != nil {
		t.Fatalf("write helper: %v", err)
	}
	gitconfig := filepath.Join(home, ".gitconfig")
	if err := os.WriteFile(gitconfig, []byte("[credential]\n\thelper = "+helper+"\n"), 0o600); err != nil {
		t.Fatalf("write gitconfig: %v", err)
	}
	t.Setenv("HOME", home)
	t.Setenv("SSH_AUTH_SOCK", filepath.Join(dir, "absent.sock"))

	// The production GitCredentialSource uses the real ExecRunner, which forces
	// the child non-interactive. No helper resolves (it returns nothing), so the
	// probe must fail closed with a host-named error.
	src := native.GitCredentialSource{}
	_, err := src.Resolve(context.Background(), "github.example.com")
	if err == nil {
		t.Fatal("probe must fail closed when no helper resolves")
	}
	if !strings.Contains(err.Error(), "github.example.com") {
		t.Fatalf("probe error must name the host, got %q", err.Error())
	}
}
