package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestInspectGitApplicationUpdateDetectsFastForwardAndDirtyTree(t *testing.T) {
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
	if state.Branch != "main" || state.Upstream != "origin/main" || state.Remote != "origin" {
		t.Fatalf("unexpected Git tracking state: %#v", state)
	}
	if state.Relation != "update-available" || state.Dirty || state.Current == state.Target {
		t.Fatalf("expected clean fast-forward update: %#v", state)
	}

	if err := os.WriteFile(filepath.Join(work, "local.txt"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	state, err = inspectGitApplicationUpdate(context.Background(), work, false)
	if err != nil {
		t.Fatal(err)
	}
	if !state.Dirty || state.Relation != "update-available" {
		t.Fatalf("expected dirty tree with update available: %#v", state)
	}
}

func TestInspectGitApplicationUpdateRejectsDetachedHead(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not available")
	}
	repo := filepath.Join(t.TempDir(), "repo")
	mustGitUpdateTest(t, "", "init", "-b", "main", repo)
	configureGitUpdateTestIdentity(t, repo)
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGitUpdateTest(t, repo, "add", "README.md")
	mustGitUpdateTest(t, repo, "commit", "-m", "initial")
	mustGitUpdateTest(t, repo, "checkout", "--detach")

	_, err := inspectGitApplicationUpdate(context.Background(), repo, false)
	if err == nil || !strings.Contains(err.Error(), "detached HEAD") {
		t.Fatalf("expected detached HEAD rejection, got %v", err)
	}
}

func TestApplicationUpdateCommandIsDiscoverable(t *testing.T) {
	root := rootCommand()
	var appFound, updateFound bool
	for _, child := range root.Children {
		if child.Name != "app" {
			continue
		}
		appFound = true
		for _, appChild := range child.Children {
			if appChild.Name == "update" {
				updateFound = true
				if appChild.Usage != "baha app update [--check]" {
					t.Fatalf("unexpected update usage: %s", appChild.Usage)
				}
			}
		}
	}
	if !appFound || !updateFound {
		t.Fatalf("application update command not discoverable: app=%v update=%v", appFound, updateFound)
	}
}

func configureGitUpdateTestIdentity(t *testing.T, dir string) {
	t.Helper()
	mustGitUpdateTest(t, dir, "config", "user.name", "BaseHarbor Test")
	mustGitUpdateTest(t, dir, "config", "user.email", "baseharbor@example.invalid")
}

func mustGitUpdateTest(t *testing.T, dir string, args ...string) string {
	t.Helper()
	path, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(path, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
	return strings.TrimSpace(string(output))
}
