package applicationruntimeauth

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
)

func TestVerifierAcceptsOnlyOwningApplicationToken(t *testing.T) {
	root := t.TempDir()
	store := application.Store{Root: filepath.Join(root, "apps")}
	for _, name := range []string{"alpha", "beta"} {
		m := application.New(name, "dev", true, false, true)
		if _, err := store.Create(m); err != nil {
			t.Fatal(err)
		}
		files, err := application.EnsureRuntime(store, m)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := application.EnsureRuntimeIdentity(m, files); err != nil {
			t.Fatal(err)
		}
	}

	alpha, _, err := store.Load("alpha")
	if err != nil {
		t.Fatal(err)
	}
	alphaFiles, err := application.ExistingRuntimeFiles(store, alpha)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(application.RuntimeIdentityTokenPath(alphaFiles))
	if err != nil {
		t.Fatal(err)
	}
	token := strings.TrimSpace(string(raw))
	verifier := New(store)
	if err := verifier.Verify(context.Background(), "alpha", token); err != nil {
		t.Fatalf("own token rejected: %v", err)
	}
	if err := verifier.Verify(context.Background(), "beta", token); err == nil {
		t.Fatal("alpha token authenticated as beta")
	}
	if err := verifier.Verify(context.Background(), "alpha", token+"x"); err == nil {
		t.Fatal("wrong token authenticated")
	}

	if err := application.RevokeRuntimeIdentity(alpha, alphaFiles); err != nil {
		t.Fatal(err)
	}
	if err := verifier.Verify(context.Background(), "alpha", token); err == nil {
		t.Fatal("revoked runtime token authenticated")
	}
	if err := application.RotateRuntimeIdentity(alpha, alphaFiles); err != nil {
		t.Fatal(err)
	}
	rotatedRaw, err := os.ReadFile(application.RuntimeIdentityTokenPath(alphaFiles))
	if err != nil {
		t.Fatal(err)
	}
	rotated := strings.TrimSpace(string(rotatedRaw))
	if rotated == token {
		t.Fatal("rotation did not replace runtime token")
	}
	if err := verifier.Verify(context.Background(), "alpha", token); err == nil {
		t.Fatal("old runtime token authenticated after rotation")
	}
	if err := verifier.Verify(context.Background(), "alpha", rotated); err != nil {
		t.Fatalf("rotated runtime token rejected: %v", err)
	}
}

func TestVerifierRejectsInsecureCredentialPermissions(t *testing.T) {
	root := t.TempDir()
	store := application.Store{Root: filepath.Join(root, "apps")}
	m := application.New("alpha", "dev", true, false, true)
	if _, err := store.Create(m); err != nil {
		t.Fatal(err)
	}
	files, err := application.EnsureRuntime(store, m)
	if err != nil {
		t.Fatal(err)
	}
	path, err := application.EnsureRuntimeIdentity(m, files)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := New(store).Verify(context.Background(), "alpha", strings.TrimSpace(string(raw))); err == nil {
		t.Fatal("insecure runtime credential permissions were accepted")
	}
}
