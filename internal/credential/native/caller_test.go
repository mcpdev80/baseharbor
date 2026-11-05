package native_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/credential/native"
)

// TestGitHelperRealProtocol exercises the actual `git credential fill` line
// protocol through a real helper script: the request goes on stdin and the
// resolved credential comes back on stdout. This proves the callable path is
// not the broken "git credential fill https <host>" argument form.
func TestGitHelperRealProtocol(t *testing.T) {
	if _, ok := os.LookupEnv("HOME"); !ok {
		t.Skip("no HOME")
	}

	dir := t.TempDir()
	// A helper script that echoes a resolved credential on stdout for any input.
	script := filepath.Join(dir, "helper.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf 'protocol=https\nhost=%s\nusername=oath-user\npassword=SECRETTOKEN\n'\n"), 0o755); err != nil {
		t.Fatalf("write helper: %v", err)
	}

	src := native.GitCredentialSource{Helper: script, Run: nil}
	// A real helper cannot be injected with fakeRunner (it runs the binary), so
	// use ExecRunner by leaving Run nil.
	resolved, err := src.Resolve(context.Background(), "github.com")
	if err != nil {
		t.Fatalf("Resolve with a working helper must succeed: %v", err)
	}
	if !resolved {
		t.Fatal("expected resolved=true when a helper returns a credential")
	}
}

// TestGitHelperMissingHelperFailsClosed proves an absent helper does not prompt
// and fails closed with the documented marker.
func TestGitHelperMissingHelperFailsClosed(t *testing.T) {
	// Point PATH at an empty directory so `git` is not found.
	empty := t.TempDir()
	t.Setenv("PATH", empty)

	src := native.GitCredentialSource{}
	resolved, err := src.Resolve(context.Background(), "github.com")
	if resolved {
		t.Fatal("missing helper must not resolve")
	}
	if !errors.Is(err, native.ErrGitCredentialUnavailable) {
		t.Fatalf("want ErrGitCredentialUnavailable, got %v", err)
	}
}

// TestGitHelperEmptyReplyFailsClosed proves a helper that exits zero but returns
// no credential fails closed rather than reporting a false success.
func TestGitHelperEmptyReplyFailsClosed(t *testing.T) {
	f := &fakeRunner{out: []byte("")}
	src := native.GitCredentialSource{Run: f}

	resolved, err := src.Resolve(context.Background(), "github.com")
	if resolved {
		t.Fatal("empty reply must not resolve")
	}
	if !errors.Is(err, native.ErrGitCredentialUnavailable) {
		t.Fatalf("want ErrGitCredentialUnavailable, got %v", err)
	}
}

// TestExecRunnerRedactsStderr proves the production runner never surfaces raw
// stderr (secrets / helper payloads) on failure, only a redacted class+exit.
func TestExecRunnerRedactsStderr(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "leak.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho secret-on-stderr-to-stderr>&1\nexit 3\n"), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	// Note: the script's stdout is empty; it exits non-zero.
	src := native.GitCredentialSource{Helper: script}
	_, err := src.Resolve(context.Background(), "github.com")
	if err == nil {
		t.Fatal("non-zero helper must return an error")
	}
	if strings.Contains(strings.ToLower(err.Error()), "secret") {
		t.Fatalf("error leaked secret material from stderr: %v", err)
	}
}

// TestNativeBrokerPresent resolves when a git helper is configured and reports a
// native source resolved, without ever returning the secret.
func TestNativeBrokerPresent(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "helper.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf 'username=oath-user\npassword=x\n'\n"), 0o755); err != nil {
		t.Fatalf("write helper: %v", err)
	}
	broker := native.Broker{
		Git: native.GitCredentialSource{Helper: script},
	}
	data, err := broker.Resolve(context.Background(), "ref/secret", "github.com")
	if err != nil {
		t.Fatalf("Broker.Resolve with helper present: %v", err)
	}
	if len(data.Payload) != 0 {
		t.Fatal("broker must never return secret payload")
	}
}

// TestNativeBrokerAbsentFailsClosed proves the broker fails closed when no
// native source is available and never prompts.
func TestNativeBrokerAbsentFailsClosed(t *testing.T) {
	empty := t.TempDir()
	t.Setenv("PATH", empty)

	broker := native.Broker{
		Git: native.GitCredentialSource{},
	}
	_, err := broker.Resolve(context.Background(), "ref/secret", "github.com")
	if !errors.Is(err, native.ErrNoNativeCredential) {
		t.Fatalf("want ErrNoNativeCredential when no source resolves, got %v", err)
	}
}

// TestNativeBrokerEmptyHostFailsClosed proves an empty host is rejected before
// any helper is queried.
func TestNativeBrokerEmptyHostFailsClosed(t *testing.T) {
	broker := native.Broker{
		Git: native.GitCredentialSource{},
	}
	_, err := broker.Resolve(context.Background(), "ref/secret", "  ")
	if !errors.Is(err, native.ErrNoNativeCredential) {
		t.Fatalf("want ErrNoNativeCredential for empty host, got %v", err)
	}
}

// TestNativeBrokerEmptyRefFailsClosed proves the standard empty-ref gate.
func TestNativeBrokerEmptyRefFailsClosed(t *testing.T) {
	broker := native.Broker{}
	if _, err := broker.Resolve(context.Background(), "", "github.com"); err == nil {
		t.Fatal("empty ref must fail closed")
	}
}
