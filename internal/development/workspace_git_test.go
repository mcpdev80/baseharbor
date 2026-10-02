package development

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkspaceGitFastForwardPositive(t *testing.T) {
	ctx := context.Background()
	remote, local, peer := createWorkspaceGitFixture(t)

	if err := os.WriteFile(filepath.Join(peer, "remote.txt"), []byte("remote\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitTest(t, peer, "add", "remote.txt")
	gitTest(t, peer, "commit", "-m", "remote")
	gitTest(t, peer, "push", "origin", "main")

	model, mapping := workspaceGitModel(local)
	status, err := InspectWorkspaceGit(ctx, model, mapping, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(status.Repositories) != 1 || status.Repositories[0].State != WorkspaceGitUpdateAvailable || !status.Repositories[0].UpdateSafe {
		t.Fatalf("status = %#v, remote=%s", status, remote)
	}
	gotStatus := status.Repositories[0]
	if gotStatus.Relation != "behind" || gotStatus.Behind < 1 || gotStatus.Ahead != 0 || gotStatus.Remote != "origin" {
		t.Fatalf("relation/provenance = %#v", gotStatus)
	}

	result, err := UpdateWorkspaceGit(ctx, model, mapping, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := result.Repositories[0]; got.State != WorkspaceGitCurrent || got.UpdateSafe {
		t.Fatalf("result = %#v", got)
	}
	if _, err := os.Stat(filepath.Join(local, "remote.txt")); err != nil {
		t.Fatalf("fast-forward did not update checkout: %v", err)
	}
}

func TestWorkspaceGitDirtyNegative(t *testing.T) {
	_, local, _ := createWorkspaceGitFixture(t)
	if err := os.WriteFile(filepath.Join(local, "README.md"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	model, mapping := workspaceGitModel(local)
	status, err := InspectWorkspaceGit(context.Background(), model, mapping, false)
	if err != nil {
		t.Fatal(err)
	}
	got := status.Repositories[0]
	if got.State != WorkspaceGitDirty || got.UpdateSafe {
		t.Fatalf("status = %#v", got)
	}
}

func TestWorkspaceGitDivergedNegative(t *testing.T) {
	_, local, peer := createWorkspaceGitFixture(t)
	if err := os.WriteFile(filepath.Join(local, "local.txt"), []byte("local\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitTest(t, local, "add", "local.txt")
	gitTest(t, local, "commit", "-m", "local")

	if err := os.WriteFile(filepath.Join(peer, "remote.txt"), []byte("remote\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitTest(t, peer, "add", "remote.txt")
	gitTest(t, peer, "commit", "-m", "remote")
	gitTest(t, peer, "push", "origin", "main")

	model, mapping := workspaceGitModel(local)
	status, err := InspectWorkspaceGit(context.Background(), model, mapping, true)
	if err != nil {
		t.Fatal(err)
	}
	got := status.Repositories[0]
	if got.State != WorkspaceGitDiverged || got.UpdateSafe {
		t.Fatalf("status = %#v", got)
	}
}

func TestWorkspaceGitAheadOnlyNegative(t *testing.T) {
	_, local, _ := createWorkspaceGitFixture(t)
	if err := os.WriteFile(filepath.Join(local, "local.txt"), []byte("local\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitTest(t, local, "add", "local.txt")
	gitTest(t, local, "commit", "-m", "local")

	model, mapping := workspaceGitModel(local)
	status, err := InspectWorkspaceGit(context.Background(), model, mapping, false)
	if err != nil {
		t.Fatal(err)
	}
	got := status.Repositories[0]
	if got.State != WorkspaceGitAhead || got.UpdateSafe || got.Blocker != "ahead_only" {
		t.Fatalf("status = %#v", got)
	}
}

func TestClassifyGitFetchBlockerAuthentication(t *testing.T) {
	for _, message := range []string{
		"Permission denied (publickey).",
		"fatal: Authentication failed for 'https://example.invalid/repo.git/'",
		"fatal: could not read Username for 'https://example.invalid': terminal prompts disabled",
	} {
		if got := classifyGitFetchBlocker(errors.New(message)); got != "authentication_failed" {
			t.Fatalf("message %q classified as %q", message, got)
		}
	}
	if got := classifyGitFetchBlocker(errors.New("network is unreachable")); got != "fetch_failed" {
		t.Fatalf("network failure classified as %q", got)
	}
}

func TestWorkspaceGitFetchFailureIsActionableNegative(t *testing.T) {
	_, local, _ := createWorkspaceGitFixture(t)
	gitTest(t, local, "remote", "set-url", "origin", filepath.Join(t.TempDir(), "missing.git"))

	model, mapping := workspaceGitModel(local)
	status, err := InspectWorkspaceGit(context.Background(), model, mapping, true)
	if err != nil {
		t.Fatal(err)
	}
	got := status.Repositories[0]
	if got.State != WorkspaceGitUnavailable || got.Blocker != "fetch_failed" || got.NextAction == "" {
		t.Fatalf("status = %#v", got)
	}
}

func TestWorkspaceGitDetachedNegative(t *testing.T) {
	_, local, _ := createWorkspaceGitFixture(t)
	head := strings.TrimSpace(gitTestOutput(t, local, "rev-parse", "HEAD"))
	gitTest(t, local, "checkout", "--detach", head)

	model, mapping := workspaceGitModel(local)
	status, err := InspectWorkspaceGit(context.Background(), model, mapping, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := status.Repositories[0]; got.State != WorkspaceGitDetached || got.UpdateSafe {
		t.Fatalf("status = %#v", got)
	}
}

func TestWorkspaceGitMissingUpstreamNegative(t *testing.T) {
	root := t.TempDir()
	gitTest(t, root, "init", "-b", "main")
	gitTest(t, root, "config", "user.email", "test@example.invalid")
	gitTest(t, root, "config", "user.name", "BaseHarbor Test")
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitTest(t, root, "add", "README.md")
	gitTest(t, root, "commit", "-m", "initial")

	model, mapping := workspaceGitModel(root)
	status, err := InspectWorkspaceGit(context.Background(), model, mapping, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := status.Repositories[0]; got.State != WorkspaceGitNoUpstream || got.UpdateSafe {
		t.Fatalf("status = %#v", got)
	}
}

func TestWorkspaceGitUpdateReportsPartialProgress(t *testing.T) {
	_, localA, peerA := createWorkspaceGitFixture(t)
	_, localB, _ := createWorkspaceGitFixture(t)

	if err := os.WriteFile(filepath.Join(peerA, "remote.txt"), []byte("remote\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitTest(t, peerA, "add", "remote.txt")
	gitTest(t, peerA, "commit", "-m", "remote")
	gitTest(t, peerA, "push", "origin", "main")

	if err := os.WriteFile(filepath.Join(localB, "README.md"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	model := SourceModel{
		SchemaVersion: SourceModelVersion,
		Application:   "demo",
		Sources: []SourceDefinition{
			{ID: "api-source", Type: SourceRepository, Repository: "file://api", Ref: "main"},
			{ID: "web-source", Type: SourceRepository, Repository: "file://web", Ref: "main"},
		},
		Components: []ComponentSource{
			{Component: "api", Source: "api-source"},
			{Component: "web", Source: "web-source"},
		},
	}
	mapping := WorkspaceMapping{
		SchemaVersion: WorkspaceMappingVersion,
		Application:   "demo",
		Sources: map[string]string{
			"api-source": localA,
			"web-source": localB,
		},
	}
	result, err := UpdateWorkspaceGit(context.Background(), model, mapping, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Repositories) != 2 {
		t.Fatalf("repositories = %#v", result.Repositories)
	}
	bySource := map[string]WorkspaceGitRepositoryStatus{}
	for _, item := range result.Repositories {
		bySource[item.Source] = item
	}
	if got := bySource["api-source"]; got.State != WorkspaceGitCurrent || !got.Updated {
		t.Fatalf("api result = %#v", got)
	}
	if got := bySource["web-source"]; got.State != WorkspaceGitDirty || got.Updated || got.Blocker != "dirty_worktree" {
		t.Fatalf("web result = %#v", got)
	}
	if _, err := os.Stat(filepath.Join(localA, "remote.txt")); err != nil {
		t.Fatalf("safe repository was not updated: %v", err)
	}
}

func TestWorkspaceGitReportsDeclaredProvenancePositive(t *testing.T) {
	_, local, _ := createWorkspaceGitFixture(t)
	model, mapping := workspaceGitModel(local)
	status, err := InspectWorkspaceGit(context.Background(), model, mapping, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(status.Repositories) != 1 {
		t.Fatalf("repositories = %#v", status.Repositories)
	}
	got := status.Repositories[0]
	if got.DeclaredRef != "main" {
		t.Fatalf("declared ref = %q", got.DeclaredRef)
	}
	if got.DeclaredRevision == "" || got.CurrentRevision == "" {
		t.Fatalf("missing provenance: %#v", got)
	}
	if got.DeclaredRevision != got.CurrentRevision {
		t.Fatalf("declared/current revision mismatch: %#v", got)
	}
}

func TestWorkspaceGitCheckDoesNotMutate(t *testing.T) {
	_, local, peer := createWorkspaceGitFixture(t)
	if err := os.WriteFile(filepath.Join(peer, "remote.txt"), []byte("remote\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitTest(t, peer, "add", "remote.txt")
	gitTest(t, peer, "commit", "-m", "remote")
	gitTest(t, peer, "push", "origin", "main")

	before := strings.TrimSpace(gitTestOutput(t, local, "rev-parse", "HEAD"))
	model, mapping := workspaceGitModel(local)
	result, err := UpdateWorkspaceGit(context.Background(), model, mapping, true)
	if err != nil {
		t.Fatal(err)
	}
	after := strings.TrimSpace(gitTestOutput(t, local, "rev-parse", "HEAD"))
	if before != after {
		t.Fatalf("check mutated HEAD: %s -> %s", before, after)
	}
	if result.Repositories[0].State != WorkspaceGitUpdateAvailable {
		t.Fatalf("result = %#v", result.Repositories[0])
	}
}

func workspaceGitModel(path string) (SourceModel, WorkspaceMapping) {
	return SourceModel{
			SchemaVersion: SourceModelVersion,
			Application:   "demo",
			Sources:       []SourceDefinition{{ID: "app-source", Type: SourceRepository, Repository: "file://fixture", Ref: "main"}},
			Components:    []ComponentSource{{Component: "app", Source: "app-source"}},
		}, WorkspaceMapping{
			SchemaVersion: WorkspaceMappingVersion,
			Application:   "demo",
			Sources:       map[string]string{"app-source": path},
		}
}

func createWorkspaceGitFixture(t *testing.T) (remote, local, peer string) {
	t.Helper()
	root := t.TempDir()
	remote = filepath.Join(root, "remote.git")
	gitTest(t, root, "init", "--bare", remote)

	seed := filepath.Join(root, "seed")
	gitTest(t, root, "clone", remote, seed)
	gitTest(t, seed, "config", "user.email", "test@example.invalid")
	gitTest(t, seed, "config", "user.name", "BaseHarbor Test")
	gitTest(t, seed, "checkout", "-b", "main")
	if err := os.WriteFile(filepath.Join(seed, "README.md"), []byte("initial\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitTest(t, seed, "add", "README.md")
	gitTest(t, seed, "commit", "-m", "initial")
	gitTest(t, seed, "push", "-u", "origin", "main")
	gitTest(t, remote, "symbolic-ref", "HEAD", "refs/heads/main")

	local = filepath.Join(root, "local")
	peer = filepath.Join(root, "peer")
	gitTest(t, root, "clone", remote, local)
	gitTest(t, root, "clone", remote, peer)
	for _, repo := range []string{local, peer} {
		gitTest(t, repo, "config", "user.email", "test@example.invalid")
		gitTest(t, repo, "config", "user.name", "BaseHarbor Test")
	}
	return remote, local, peer
}

func gitTest(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func gitTestOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}
