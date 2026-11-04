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

// fakeRunner records the last invocation and returns queued output.
type fakeRunner struct {
	lastName string
	lastArgs []string
	out      []byte
	runErr   error
}

func (f *fakeRunner) Invoke(_ context.Context, name string, args ...string) ([]byte, error) {
	f.lastName = name
	f.lastArgs = args
	return f.out, f.runErr
}

func TestGitCredentialPresentUsesGitHelper(t *testing.T) {
	f := &fakeRunner{out: []byte("protocol=https\nhost=github.com\nusername=oath-user\n")}
	src := native.GitCredentialSource{Run: f}

	resolved, err := src.Resolve(context.Background(), "github.com")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !resolved {
		t.Fatal("expected resolved=true when helper exits zero")
	}
	if f.lastName != "git" {
		t.Fatalf("expected git helper, got %q", f.lastName)
	}
	wantArgs := []string{"credential", "fill", "https", "github.com"}
	if strings.Join(f.lastArgs, ",") != strings.Join(wantArgs, ",") {
		t.Fatalf("unexpected args: got %v want %v", f.lastArgs, wantArgs)
	}
}

func TestGitCredentialExplicitHelperIsForwarded(t *testing.T) {
	f := &fakeRunner{}
	src := native.GitCredentialSource{Helper: "!gh auth git-credential", Run: f}

	if _, err := src.Resolve(context.Background(), "github.com"); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if f.lastName != "!gh auth git-credential" {
		t.Fatalf("explicit helper not forwarded, got %q", f.lastName)
	}
}

func TestGitCredentialAbsentFailsClosed(t *testing.T) {
	f := &fakeRunner{runErr: errors.New("exit 1")}
	src := native.GitCredentialSource{Run: f}

	resolved, err := src.Resolve(context.Background(), "github.com")
	if resolved {
		t.Fatal("resolved must be false when helper fails")
	}
	if !errors.Is(err, native.ErrGitCredentialUnavailable) {
		t.Fatalf("want ErrGitCredentialUnavailable, got %v", err)
	}
}

func TestGitCredentialEmptyHostRejected(t *testing.T) {
	src := native.GitCredentialSource{}
	if _, err := src.Resolve(context.Background(), ""); err == nil {
		t.Fatal("empty host must fail closed")
	}
}

func TestSSHAgentUnavailableDocumentsAbsentCase(t *testing.T) {
	src := native.SSHAgentSource{Env: func(string) string { return "" }}

	if src.Available(context.Background()) {
		t.Fatal("no agent should be available")
	}
	if has, err := src.HasKey(context.Background(), "example.com"); err == nil || has {
		t.Fatalf("absent agent must report not-present: has=%v err=%v", has, err)
	}
}

// sshAgentTempSocket points SSH_AUTH_SOCK at a real socket file in the temp dir
// so the ssh-agent tests are deterministic and host-independent. On a clean
// hosted runner there is no /run/user/1000/openssh_agent; relying on the runner
// environment instead of isolating is exactly the failure that bit these tests
// before. A listening unix socket file is enough for os.Stat to succeed.
func sshAgentTempSocket(t *testing.T) string {
	t.Helper()
	sockPath := filepath.Join(t.TempDir(), "agent.sock")
	f, err := os.Create(sockPath)
	if err != nil {
		t.Fatalf("create temp agent socket: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })
	t.Setenv("SSH_AUTH_SOCK", sockPath)
	return sockPath
}

func TestSSHAgentAvailableNoKey(t *testing.T) {
	sshAgentTempSocket(t)
	f := &fakeRunner{out: []byte("\n\n")}
	src := native.SSHAgentSource{Run: f}

	if !src.Available(context.Background()) {
		t.Fatal("agent socket present should be available")
	}
	has, err := src.HasKey(context.Background(), "example.com")
	if err != nil {
		t.Fatalf("HasKey with agent but no keys: %v", err)
	}
	if has {
		t.Fatal("agent with no keys must report not-present")
	}
}

func TestSSHAgentAvailableWithKey(t *testing.T) {
	sshAgentTempSocket(t)
	f := &fakeRunner{out: []byte("ssh-ed25519 AAAAC3NzaC1lZDI1OTE5AAAAexample key\n")}
	src := native.SSHAgentSource{Run: f}

	has, err := src.HasKey(context.Background(), "example.com")
	if err != nil {
		t.Fatalf("HasKey: %v", err)
	}
	if !has {
		t.Fatal("agent with a key must report present")
	}
}

func TestKubeSourceAbsentDocumented(t *testing.T) {
	src := native.KubeSource{}
	if src.Available(context.Background()) {
		t.Fatal("kube absent in this environment")
	}
	if err := src.Cleanup(); err != nil {
		t.Fatalf("kube Cleanup: %v", err)
	}
}

func TestRegistrySourceDocumentedMarker(t *testing.T) {
	src := native.RegistrySource{}
	if err := src.Cleanup(); err != nil {
		t.Fatalf("registry Cleanup: %v", err)
	}
	if !src.Available(context.Background()) {
		t.Fatal("registry marker present by default")
	}
}

func TestNoSecretOutputLeak(t *testing.T) {
	f := &fakeRunner{runErr: errors.New("exit 1")}
	src := native.GitCredentialSource{Run: f}

	_, err := src.Resolve(context.Background(), "github.com")
	if !errors.Is(err, native.ErrGitCredentialUnavailable) {
		t.Fatalf("expected documented marker, got %v", err)
	}
	if strings.Contains(err.Error(), "SECRETTOKEN") {
		t.Fatalf("error leaked secret value: %v", err)
	}
}

func TestCleanupIsSafeForAllSources(t *testing.T) {
	var g native.GitCredentialSource
	if err := g.Cleanup(); err != nil {
		t.Fatalf("git Cleanup: %v", err)
	}
	var s native.SSHAgentSource
	if err := s.Cleanup(); err != nil {
		t.Fatalf("ssh Cleanup: %v", err)
	}
	var k native.KubeSource
	if err := k.Cleanup(); err != nil {
		t.Fatalf("kube Cleanup: %v", err)
	}
	var r native.RegistrySource
	if err := r.Cleanup(); err != nil {
		t.Fatalf("registry Cleanup: %v", err)
	}
}
