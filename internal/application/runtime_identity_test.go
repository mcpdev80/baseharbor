package application

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureRuntimeIdentityIsStableAndOwnerOnly(t *testing.T) {
	root := t.TempDir()
	files := RuntimeFiles{
		Dir:      filepath.Join(root, "runtime"),
		Bindings: filepath.Join(root, "runtime", "bindings"),
	}
	m := New("demo", "dev", true, false, true)
	first, err := EnsureRuntimeIdentity(m, files)
	if err != nil {
		t.Fatal(err)
	}
	firstValue, err := os.ReadFile(first)
	if err != nil {
		t.Fatal(err)
	}
	if len(strings.TrimSpace(string(firstValue))) < 40 {
		t.Fatal("runtime identity token is unexpectedly short")
	}
	info, err := os.Stat(first)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("token mode = %o, want 600", info.Mode().Perm())
	}
	second, err := EnsureRuntimeIdentity(m, files)
	if err != nil {
		t.Fatal(err)
	}
	secondValue, err := os.ReadFile(second)
	if err != nil {
		t.Fatal(err)
	}
	if first != second || string(firstValue) != string(secondValue) {
		t.Fatal("runtime identity changed during convergence")
	}
}

func TestEnsureRuntimeIdentityDisabledWithoutManagedSecrets(t *testing.T) {
	files := RuntimeFiles{Bindings: filepath.Join(t.TempDir(), "bindings")}
	m := New("demo", "dev", true, false, false)
	path, err := EnsureRuntimeIdentity(m, files)
	if err != nil {
		t.Fatal(err)
	}
	if path != "" {
		t.Fatalf("unexpected runtime identity path %q", path)
	}
}
