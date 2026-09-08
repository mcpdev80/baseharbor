package application

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestExistingRuntimeFilesRequiresAppliedState(t *testing.T) {
	store := Store{Root: filepath.Join(t.TempDir(), "apps")}
	m := New("demo", "dev", true, false, false)
	if _, err := store.Create(m); err != nil {
		t.Fatal(err)
	}
	if _, err := ExistingRuntimeFiles(store, m); !errors.Is(err, ErrRuntimeNotApplied) {
		t.Fatalf("expected ErrRuntimeNotApplied, got %v", err)
	}
}

func TestCheckRuntimePermissionsRejectsBroadAccess(t *testing.T) {
	store := Store{Root: filepath.Join(t.TempDir(), "apps")}
	m := New("demo", "dev", true, false, false)
	if _, err := store.Create(m); err != nil {
		t.Fatal(err)
	}
	files, err := EnsurePostgresRuntime(store, m)
	if err != nil {
		t.Fatal(err)
	}
	if err := CheckRuntimePermissions(files); err != nil {
		t.Fatalf("secure runtime rejected: %v", err)
	}
	if err := os.Chmod(files.Env, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := CheckRuntimePermissions(files); err == nil {
		t.Fatal("expected broad runtime permissions to fail")
	}
}

func TestCheckRuntimePermissionsRejectsBroadOpenBaoCredentials(t *testing.T) {
	store := Store{Root: filepath.Join(t.TempDir(), "apps")}
	m := New("demo", "dev", true, false, true)
	files, err := EnsureRuntime(store, m)
	if err != nil {
		t.Fatal(err)
	}
	openBaoCredentials := filepath.Join(files.Dir, "openbao.env")
	if err := os.WriteFile(openBaoCredentials, []byte("OPENBAO_ROLE_ID=role\nOPENBAO_SECRET_ID=secret\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := CheckRuntimePermissions(files); err == nil {
		t.Fatal("expected broad OpenBao application credential permissions to fail")
	}
}
