package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestFastForwardGitApplicationUpdateUsesExactFetchedTarget(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not available")
	}

	remote := filepath.Join(t.TempDir(), "remote.git")
	mustGitUpdateTest(t, "", "init", "--bare", remote)

	seed := filepath.Join(t.TempDir(), "seed")
	mustGitUpdateTest(t, "", "init", "-b", "main", seed)
	configureGitUpdateTestIdentity(t, seed)
	if err := os.WriteFile(filepath.Join(seed, "README.md"), []byte("one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGitUpdateTest(t, seed, "add", "README.md")
	mustGitUpdateTest(t, seed, "commit", "-m", "initial")
	mustGitUpdateTest(t, seed, "remote", "add", "origin", remote)
	mustGitUpdateTest(t, seed, "push", "-u", "origin", "main")

	work := filepath.Join(t.TempDir(), "work")
	mustGitUpdateTest(t, "", "clone", "--branch", "main", remote, work)
	configureGitUpdateTestIdentity(t, work)

	if err := os.WriteFile(filepath.Join(seed, "README.md"), []byte("two\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGitUpdateTest(t, seed, "add", "README.md")
	mustGitUpdateTest(t, seed, "commit", "-m", "upstream")
	mustGitUpdateTest(t, seed, "push", "origin", "main")

	state, err := inspectGitApplicationUpdate(context.Background(), work, true)
	if err != nil {
		t.Fatal(err)
	}
	if state.Relation != "update-available" || state.Dirty {
		t.Fatalf("expected clean fast-forward state: %#v", state)
	}
	if err := fastForwardGitApplicationUpdate(context.Background(), state); err != nil {
		t.Fatal(err)
	}
	if got := mustGitUpdateTest(t, work, "rev-parse", "HEAD"); got != state.Target {
		t.Fatalf("unexpected HEAD after fast-forward: got %s want %s", got, state.Target)
	}
	if got, err := os.ReadFile(filepath.Join(work, "README.md")); err != nil || string(got) != "two\n" {
		t.Fatalf("updated source not materialized: %q err=%v", got, err)
	}
}

func TestFastForwardGitApplicationUpdateRejectsTreeChangedAfterPreflight(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not available")
	}

	remote := filepath.Join(t.TempDir(), "remote.git")
	mustGitUpdateTest(t, "", "init", "--bare", remote)
	seed := filepath.Join(t.TempDir(), "seed")
	mustGitUpdateTest(t, "", "init", "-b", "main", seed)
	configureGitUpdateTestIdentity(t, seed)
	if err := os.WriteFile(filepath.Join(seed, "README.md"), []byte("one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGitUpdateTest(t, seed, "add", "README.md")
	mustGitUpdateTest(t, seed, "commit", "-m", "initial")
	mustGitUpdateTest(t, seed, "remote", "add", "origin", remote)
	mustGitUpdateTest(t, seed, "push", "-u", "origin", "main")

	work := filepath.Join(t.TempDir(), "work")
	mustGitUpdateTest(t, "", "clone", "--branch", "main", remote, work)
	configureGitUpdateTestIdentity(t, work)
	if err := os.WriteFile(filepath.Join(seed, "README.md"), []byte("two\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGitUpdateTest(t, seed, "add", "README.md")
	mustGitUpdateTest(t, seed, "commit", "-m", "upstream")
	mustGitUpdateTest(t, seed, "push", "origin", "main")

	state, err := inspectGitApplicationUpdate(context.Background(), work, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, "local.txt"), []byte("changed after preflight\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := fastForwardGitApplicationUpdate(context.Background(), state); err == nil || !strings.Contains(err.Error(), "changed after preflight") {
		t.Fatalf("expected changed-tree rejection, got %v", err)
	}
	if got := mustGitUpdateTest(t, work, "rev-parse", "HEAD"); got != state.Current {
		t.Fatalf("HEAD changed despite rejected update: got %s want %s", got, state.Current)
	}
}
