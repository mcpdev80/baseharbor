package native_test

import (
	"context"
	"errors"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/credential/native"
)

// TestPreInteractiveNoPromptGitHitProvesFoundOnly proves a native git hit
// reports a hit without ever returning secret material: the helper payload is
// a host credential, never delivered to the caller. The result is a found-only
// foretaste about interactivity.
func TestPreInteractiveNoPromptGitHitProvesFoundOnly(t *testing.T) {
	f := &fakeRunner{out: []byte("protocol=https\nhost=github.com\nusername=oath-user\npassword=SECRETTOKEN\n")}
	nativeHit, err := native.PreInteractiveNoPromptWithRunner(context.Background(), "github.com", f)
	if err != nil {
		t.Fatalf("PreInteractiveNoPrompt: %v", err)
	}
	if !nativeHit {
		t.Fatal("expected a native hit when the git helper resolves the host")
	}
}

// TestPreInteractiveNoPromptGitMissFindsNothing proves no hit when the helper
// fails closed. A miss must not be read as authentication success.
func TestPreInteractiveNoPromptGitMissFindsNothing(t *testing.T) {
	f := &fakeRunner{runErr: errors.New("exit 1")}
	nativeHit, err := native.PreInteractiveNoPromptWithRunner(context.Background(), "github.com", f)
	if err != nil {
		t.Fatalf("PreInteractiveNoPrompt: %v", err)
	}
	if nativeHit {
		t.Fatal("no native hit when the git helper fails closed")
	}
}

// keyedRunner returns git-relevant output for the first Invoke (git fill) and
// ssh-relevant output for the second Invoke (ssh-add -L), so the native hit
// branches can be exercised deterministically.
type keyedRunner struct {
	gitOut   []byte
	gitErr   error
	sshOut   []byte
	sshErr   error
	gitCalls int
	sshCalls int
}

func (k *keyedRunner) Invoke(_ context.Context, name string, stdin string, args ...string) ([]byte, error) {
	if name == "git" || (len(args) > 0 && args[0] == "credential") {
		k.gitCalls++
		return k.gitOut, k.gitErr
	}
	k.sshCalls++
	return k.sshOut, k.sshErr
}

// TestPreInteractiveNoPromptSSHHitProvesFoundOnly proves a native ssh hit also
// reports a hit found-only: git fails closed but an ssh agent key is present.
func TestPreInteractiveNoPromptSSHHitProvesFoundOnly(t *testing.T) {
	k := &keyedRunner{
		gitOut: []byte(""),
		gitErr: errors.New("exit 1"),
		sshOut: []byte("ssh-ed25519 AAAAC3NzaC1lZDI1OTE5AAAAexample key\n"),
	}
	nativeHit, err := native.PreInteractiveNoPromptWithRunner(context.Background(), "example.com", k)
	if err != nil {
		t.Fatalf("PreInteractiveNoPrompt: %v", err)
	}
	if !nativeHit {
		t.Fatal("expected a native hit when the ssh agent has a usable key")
	}
	if k.gitCalls == 0 || k.sshCalls == 0 {
		t.Fatalf("expected both git and ssh sources probed: gitCalls=%d sshCalls=%d", k.gitCalls, k.sshCalls)
	}
}

// TestPreInteractiveNoPromptBrokenSSHSourceIsNotFound proves a broken native
// source does not read as authentication success: it stays a plain miss.
func TestPreInteractiveNoPromptBrokenSSHSourceIsNotFound(t *testing.T) {
	k := &keyedRunner{
		gitErr: errors.New("exit 1"),
		sshErr: errors.New("exit 7"),
	}
	nativeHit, err := native.PreInteractiveNoPromptWithRunner(context.Background(), "example.com", k)
	if err != nil {
		t.Fatalf("PreInteractiveNoPrompt: %v", err)
	}
	if nativeHit {
		t.Fatal("a broken native source must not read as authentication success")
	}
}

// TestHostFromURLDerivesHostFromRemote proves host extraction across the git
// remote forms the four production call sites use.
func TestHostFromURLDerivesHostFromRemote(t *testing.T) {
	cases := map[string]string{
		"https://github.com/example/app.git":      "github.com",
		"http://github.com/example/app.git":       "github.com",
		"https://user@github.com/example/app.git": "github.com",
		"ssh://git@github.com/example/app.git":    "github.com",
		"ssh://github.com:2222/example/app.git":   "github.com",
		"git@example.com:example/app.git":         "example.com",
		"https://gitlab.com:8443/group/app.git":   "gitlab.com",
	}
	for source, want := range cases {
		if got := native.HostFromURL(source); got != want {
			t.Fatalf("HostFromURL(%q) = %q, want %q", source, got, want)
		}
	}
}

// TestHostFromURLRejectsUnparseable proves an unparseable source yields no host
// so callers keep their previous behavior.
func TestHostFromURLRejectsUnparseable(t *testing.T) {
	for _, source := range []string{"", "   ", "./local-repo", "/absolute/path", "notaurl"} {
		if got := native.HostFromURL(source); got != "" {
			t.Fatalf("HostFromURL(%q) = %q, want empty", source, got)
		}
	}
}

// TestPreInteractiveNoPromptEmptyHostFindsNothing proves an empty host is a
// plain miss rather than a hit.
func TestPreInteractiveNoPromptEmptyHostFindsNothing(t *testing.T) {
	f := &fakeRunner{out: []byte("username=oath-user\n")}
	nativeHit, err := native.PreInteractiveNoPromptWithRunner(context.Background(), "   ", f)
	if err != nil {
		t.Fatalf("PreInteractiveNoPrompt: %v", err)
	}
	if nativeHit {
		t.Fatal("an empty host must not report a native hit")
	}
}
