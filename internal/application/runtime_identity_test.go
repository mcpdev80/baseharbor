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

func TestRuntimeIdentityRotateAndRevoke(t *testing.T) {
	root := t.TempDir()
	files := RuntimeFiles{
		Dir:      filepath.Join(root, "runtime"),
		Bindings: filepath.Join(root, "runtime", "bindings"),
	}
	m := New("demo", "dev", true, false, true)
	path, err := EnsureRuntimeIdentity(m, files)
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := RevokeRuntimeIdentity(m, files); err != nil {
		t.Fatal(err)
	}
	if !RuntimeIdentityRevoked(files) {
		t.Fatal("runtime identity revocation marker was not created")
	}
	if err := RotateRuntimeIdentity(m, files); err != nil {
		t.Fatal(err)
	}
	if RuntimeIdentityRevoked(files) {
		t.Fatal("rotation did not clear runtime identity revocation")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) == string(after) {
		t.Fatal("rotation did not change runtime identity token")
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
