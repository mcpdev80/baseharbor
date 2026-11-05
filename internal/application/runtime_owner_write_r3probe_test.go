package application

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// TestVerifyRuntimeFileOwnerMissingRuntimeEnv proves the mutation the review
// demands: a runtime directory that exists but whose runtime.env is absent
// (a provider-change abort that left the dir but never materialized runtime.env)
// is reported by VerifyRuntimeFileOwner as NOT owned, not as an error and not as
// owned. This is the exact state the shared stopRepositoryWorkload early-returns
// (false, nil) for; the destroy-only regression below proves destroy still retires.
func TestVerifyRuntimeFileOwnerMissingRuntimeEnv(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	envPath := filepath.Join(dir, "runtime.env")
	// runtime.env is genuinely absent.
	if _, err := os.Stat(envPath); !os.IsNotExist(err) {
		t.Fatalf("precondition: runtime.env must be absent, got err=%v", err)
	}
	owned, err := VerifyRuntimeFileOwner(envPath)
	if err != nil {
		t.Fatalf("missing runtime.env must not error, got %v", err)
	}
	if owned {
		t.Fatalf("missing runtime.env must report NOT owned")
	}
	t.Log("VerifyRuntimeFileOwner(missing runtime.env) = (false, nil); runtime dir present, no owned materialization")
}

// TestVerifyRuntimeFileOwnerPresentOwned proves the healthy cell: a runtime.env
// that is an exclusively owned regular file is reported as owned. This is the
// "genuinely running owned workload" the mutation fixture contrasts against.
func TestVerifyRuntimeFileOwnerPresentOwned(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, "runtime.env")
	if err := os.WriteFile(envPath, []byte("KEY=value\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	info, statErr := os.Lstat(envPath)
	if statErr != nil {
		t.Fatal(statErr)
	}
	if !info.Mode().IsRegular() {
		t.Fatalf("runtime.env must be a regular file")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != uint32(os.Geteuid()) || stat.Nlink != 1 {
		t.Fatalf("runtime.env must be exclusively owned regular file")
	}
	owned, err := VerifyRuntimeFileOwner(envPath)
	if err != nil {
		t.Fatalf("owned runtime.env must not error, got %v", err)
	}
	if !owned {
		t.Fatalf("owned runtime.env must report owned")
	}
	t.Log("VerifyRuntimeFileOwner(owned runtime.env) = (true, nil); the healthy running-owned-workload cell")
}

// TestVerifyRuntimeFileOwnerPresentUnowned proves a present-but-unowned
// runtime.env (not an exclusively owned regular file: a directory or symlink)
// fails CLOSED with an error, never returns "owned". This is the fail-open
// guard: a corrupt/foreign runtime.env must never be silently treated as owned
// materialization, and must never be silently absorbed as "not owned either" —
// it is an inventory error the shared caller surfaces.
func TestVerifyRuntimeFileOwnerPresentUnowned(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, "runtime.env")
	// A directory named runtime.env is not a regular file.
	if err := os.Mkdir(envPath, 0o700); err != nil {
		t.Fatal(err)
	}
	owned, err := VerifyRuntimeFileOwner(envPath)
	if err == nil {
		t.Fatalf("non-regular runtime.env must fail closed with an error, got owned=%t", owned)
	}
	if owned {
		t.Fatalf("non-regular runtime.env must not report owned, got owned=true")
	}
	t.Logf("VerifyRuntimeFileOwner(non-regular runtime.env) = (false, %v); fail-closed, caller must surface it", err)
}
