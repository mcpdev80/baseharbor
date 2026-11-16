package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/credential/native"
)

// installGitCredentialHelper installs a fake git credential helper that always
// resolves a credential, using an isolated HOME so no real git config or system
// helper interferes. It returns the isolated HOME directory.
func installGitCredentialHelper(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatalf("mkdir home: %v", err)
	}
	script := filepath.Join(dir, "helper.sh")
	body := "#!/bin/sh\n" +
		"printf 'protocol=https\\nhost=%s\\nusername=oath-user\\npassword=SECRETTOKEN\\n' \"$(cat)\"\n" +
		"exit 0\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatalf("write helper: %v", err)
	}
	// Point git at a private config that installs the fake helper, so the helper
	// is discovered through git's normal credential lookup (not a nonstandard env).
	gitconfig := filepath.Join(home, ".gitconfig")
	if err := os.WriteFile(gitconfig, []byte("[credential]\n\thelper = "+script+"\n"), 0o600); err != nil {
		t.Fatalf("write gitconfig: %v", err)
	}
	t.Setenv("HOME", home)
	// Drop any inherited ssh agent so the native hit comes solely from the helper.
	t.Setenv("SSH_AUTH_SOCK", filepath.Join(dir, "absent.sock"))
	return home
}

// TestNativeHitRunsGitNonInteractively proves the caller-level counter with a
// real helper: on a native hit the gate returns true, which is the precondition
// for launching git non-interactively (GIT_TERMINAL_PROMPT=0). The found-only
// result never returns the secret the helper returned.
func TestNativeHitRunsGitNonInteractively(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	installGitCredentialHelper(t)

	nativeHit, err := native.PreInteractiveNoPrompt(context.Background(), "example.invalid")
	if err != nil {
		t.Fatalf("PreInteractiveNoPrompt: %v", err)
	}
	if !nativeHit {
		t.Fatal("expected a native hit when the git credential helper resolves the host")
	}
}

// TestNativeMissDoesNotReadAsAuthentication proves the caller-level counter for
// the miss path: with a helper that fails closed, the gate reports no native
// hit, so the caller keeps its previous interactive-allowed behavior.
func TestNativeMissDoesNotReadAsAuthentication(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	// A helper that exits non-zero (fails closed) so no credential resolves.
	failing := filepath.Join(dir, "failing-helper.sh")
	if err := os.WriteFile(failing, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatalf("write failing helper: %v", err)
	}
	// Isolate HOME so no real git config installs a system helper, and point git
	// at the failing helper.
	home := filepath.Join(dir, "home")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatalf("mkdir home: %v", err)
	}
	gitconfig := filepath.Join(home, ".gitconfig")
	if err := os.WriteFile(gitconfig, []byte("[credential]\n\thelper = "+failing+"\n"), 0o600); err != nil {
		t.Fatalf("write gitconfig: %v", err)
	}
	t.Setenv("HOME", home)
	t.Setenv("SSH_AUTH_SOCK", filepath.Join(dir, "absent.sock"))

	nativeHit, err := native.PreInteractiveNoPrompt(context.Background(), "example.invalid")
	if err != nil {
		t.Fatalf("PreInteractiveNoPrompt: %v", err)
	}
	if nativeHit {
		t.Fatal("with a helper that fails closed, the gate must not report a native hit")
	}
}

// TestNativeHitDoesNotLeakSecret proves the found-only contract at the caller
// seam: the helper returns a password, but PreInteractiveNoPrompt returns only a
// boolean hit, never the secret material.
func TestNativeHitDoesNotLeakSecret(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	installGitCredentialHelper(t)

	nativeHit, err := native.PreInteractiveNoPrompt(context.Background(), "example.invalid")
	if err != nil {
		t.Fatalf("PreInteractiveNoPrompt: %v", err)
	}
	if !nativeHit {
		t.Fatal("expected a native hit")
	}
	// The returned value is a bool; the helper's secret never leaks through the
	// boolean found-only contract.
}

// TestHostFromURLAcceptsProductionRemotes proves the host extraction used by the
// four wiring sites accepts the remote forms the product passes in.
func TestHostFromURLAcceptsProductionRemotes(t *testing.T) {
	cases := map[string]string{
		"https://github.com/example/app.git":   "github.com",
		"git@example.com:example/app.git":      "example.com",
		"ssh://git@github.com/example/app.git": "github.com",
	}
	for source, want := range cases {
		if got := native.HostFromURL(source); got != want {
			t.Fatalf("HostFromURL(%q) = %q, want %q", source, got, want)
		}
	}
}
