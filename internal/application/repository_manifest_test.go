package application

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindRepositoryManifestWalksParents(t *testing.T) {
	root := t.TempDir()
	manifest := filepath.Join(root, RepositoryManifestName)
	if err := os.WriteFile(manifest, []byte(New("demo", "dev", true, false, false).YAML()), 0o644); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "frontend", "src")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := FindRepositoryManifest(nested)
	if err != nil {
		t.Fatal(err)
	}
	if got != manifest {
		t.Fatalf("manifest = %q want %q", got, manifest)
	}
}

func TestStoreSyncPreservesRuntimeState(t *testing.T) {
	store := Store{Root: filepath.Join(t.TempDir(), ".baseharbor", "apps")}
	m := New("demo", "dev", true, false, false)
	if _, err := store.Sync(m); err != nil {
		t.Fatal(err)
	}
	ignorePath := filepath.Join(filepath.Dir(store.Root), ".gitignore")
	ignore, err := os.ReadFile(ignorePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(ignore) != stateGitIgnore {
		t.Fatalf("unexpected state gitignore %q", string(ignore))
	}
	runtimeDir := filepath.Join(store.Root, "demo", "runtime")
	if err := os.MkdirAll(runtimeDir, 0o700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(runtimeDir, "preserved")
	if err := os.WriteFile(marker, []byte("yes"), 0o600); err != nil {
		t.Fatal(err)
	}
	m = WithRedisInstances(m, "cache")
	if _, err := store.Sync(m); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("runtime state was not preserved: %v", err)
	}
	loaded, path, err := store.Load("demo")
	if err != nil {
		t.Fatal(err)
	}
	if len(RedisInstanceNames(loaded)) != 1 {
		t.Fatalf("synchronized manifest did not update: %#v", loaded)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("internal synchronized manifest permissions = %o", info.Mode().Perm())
	}
}
