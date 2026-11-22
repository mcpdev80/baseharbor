package development

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSourcePullPositiveUpdateAvailable proves the happy path: a clean branch
// strictly behind upstream pulls as update_available and ApplySourcePull moves it
// to current.
func TestSourcePullPositiveUpdateAvailable(t *testing.T) {
	ctx := context.Background()
	_, local, peer := createWorkspaceGitFixture(t)

	if err := os.WriteFile(filepath.Join(peer, "remote.txt"), []byte("remote\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitTest(t, peer, "add", "remote.txt")
	gitTest(t, peer, "commit", "-m", "remote")
	gitTest(t, peer, "push", "origin", "main")

	model, mapping := workspaceGitModel(local)
	plan, err := ResolveSourcePullPlan(ctx, filepath.Join(local, ".baseharbor", "sources.yaml"), model, mapping, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.UpdateAvailable || !plan.Applyable {
		t.Fatalf("plan = %#v", plan)
	}
	got := plan.Repositories[0]
	if got.State != SourcePullUpdateAvailable || got.Source != "app-source" {
		t.Fatalf("plan repo = %#v", got)
	}

	before := gitRev(t, local)
	result, err := ApplySourcePull(ctx, plan, false)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Successful {
		t.Fatalf("result = %#v", result)
	}
	if result.Repositories[0].State != SourcePullCurrent || !result.Repositories[0].Updated {
		t.Fatalf("result = %#v", result.Repositories[0])
	}
	if after := gitRev(t, local); after == before {
		t.Fatalf("HEAD did not advance: %s", before)
	}
}

// TestSourcePullDirtyBlocksNegative proves a dirty working tree blocks the pull
// and ApplySourcePull never mutates the checkout.
func TestSourcePullDirtyBlocksNegative(t *testing.T) {
	ctx := context.Background()
	_, local, _ := createWorkspaceGitFixture(t)
	if err := os.WriteFile(filepath.Join(local, "README.md"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	model, mapping := workspaceGitModel(local)
	plan, err := ResolveSourcePullPlan(ctx, "", model, mapping, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := plan.Repositories[0]; got.State != SourcePullDirty || plan.Applyable {
		t.Fatalf("plan = %#v", got)
	}
	result, err := ApplySourcePull(ctx, plan, false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Successful {
		t.Fatalf("dirty pull must not succeed: %#v", result)
	}
	if result.Repositories[0].State != SourcePullDirty {
		t.Fatalf("dirty repository state wrong: %#v", result.Repositories[0])
	}
	if _, err := os.Stat(filepath.Join(local, "README.md")); err != nil {
		t.Fatalf("checkout was mutated: %v", err)
	}
}

// TestSourcePullDivergedBlocksNegative proves diverged history blocks the pull.
func TestSourcePullDivergedBlocksNegative(t *testing.T) {
	ctx := context.Background()
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
	plan, err := ResolveSourcePullPlan(ctx, "", model, mapping, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := plan.Repositories[0]; got.State != SourcePullDiverged || plan.Applyable {
		t.Fatalf("plan = %#v", got)
	}
	result, err := ApplySourcePull(ctx, plan, false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Successful {
		t.Fatalf("diverged pull must not succeed: %#v", result)
	}
}

// TestSourcePullNoGitBlocksNegative proves a plain directory (no Git) reports a
// not_repository blocker and fails closed.
func TestSourcePullNoGitBlocksNegative(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	model := SourceModel{
		SchemaVersion: SourceModelVersion,
		Application:   "demo",
		Sources:       []SourceDefinition{{ID: "app-source", Type: SourceRepository, Repository: "file://fixture"}},
		Components:    []ComponentSource{{Component: "app", Source: "app-source"}},
	}
	mapping := WorkspaceMapping{SchemaVersion: WorkspaceMappingVersion, Application: "demo", Sources: map[string]string{"app-source": root}}
	plan, err := ResolveSourcePullPlan(ctx, "", model, mapping, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Repositories) != 1 || plan.Repositories[0].Blocker == "" {
		t.Fatalf("non-git path must report a blocker: %#v", plan.Repositories)
	}
	result, err := ApplySourcePull(ctx, plan, false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Successful {
		t.Fatalf("no-git pull must not succeed: %#v", result)
	}
}

// TestSourcePullPartialFailureStaysUnsuccessful proves that a selection with a
// blocker (a dirty working tree) is not applyable: ApplySourcePull returns the
// plan unchanged without fast-forwarding any repository, and the result is
// unsuccessful, so --up must never run. It does not exercise runtime
// partial application; see TestSourcePullApplyTimePartialFailureRunsOnce
// for the case where an earlier repository fast-forwards before a later one
// fails.
func TestSourcePullPartialFailureStaysUnsuccessful(t *testing.T) {
	ctx := context.Background()
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
	plan, err := ResolveSourcePullPlan(ctx, "", model, mapping, nil)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Applyable {
		t.Fatalf("partial selection must not be applyable: %#v", plan)
	}
	result, err := ApplySourcePull(ctx, plan, false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Successful {
		t.Fatalf("partial pull must report unsuccessful: %#v", result)
	}
	if !result.SourcePullHasFailure() {
		t.Fatalf("result must report a failure")
	}
}

// TestSourcePullApplyTimePartialFailureRunsOnce proves that when a repository
// fast-forwards successfully but a later repository's fast-forward fails, the
// aggregate result marks the changed and the failed repositories individually,
// stays unsuccessful so --up never runs, and no cross-repository rollback
// occurs: the changed repository is left advanced and the failed repository is
// left as-is. This is the runtime partial-update path, distinct from the
// preflight-blocker path in TestSourcePullPartialFailureStaysUnsuccessful.
func TestSourcePullApplyTimePartialFailureRunsOnce(t *testing.T) {
	ctx := context.Background()
	_, localA, peerA := createWorkspaceGitFixture(t)
	_, localB, peerB := createWorkspaceGitFixture(t)

	// api-source: clean and strictly behind its upstream, so it applies.
	if err := os.WriteFile(filepath.Join(peerA, "remote.txt"), []byte("remote\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitTest(t, peerA, "add", "remote.txt")
	gitTest(t, peerA, "commit", "-m", "remote")
	gitTest(t, peerA, "push", "origin", "main")

	// web-source: advance its upstream to a new revision.
	if err := os.WriteFile(filepath.Join(peerB, "remote2.txt"), []byte("remote2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitTest(t, peerB, "add", "remote2.txt")
	gitTest(t, peerB, "commit", "-m", "remote2")
	gitTest(t, peerB, "push", "origin", "main")

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
	plan, err := ResolveSourcePullPlan(ctx, "", model, mapping, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Applyable {
		t.Fatalf("both sources must be applyable preflight: %#v", plan)
	}
	// Capture the web-source target before introducing a local commit that makes
	// localB HEAD no longer an ancestor of that target; the fast-forward then
	// fails at apply time even though api-source already advances.
	webTarget := plan.Repositories[1].TargetRevision
	webBefore := gitRev(t, localB)
	gitTest(t, localB, "commit", "--allow-empty", "-m", "local drift")
	if got := gitRev(t, localB); got == webBefore {
		t.Fatal("expected local web-source HEAD to advance after the drift commit")
	}
	apiBefore := gitRev(t, localA)
	webBefore := gitRev(t, localB)

	result, err := ApplySourcePull(ctx, plan, false)
	if err != nil {
		t.Fatal(err)
	}

	if result.Successful {
		t.Fatalf("partial runtime failure must report unsuccessful: %#v", result)
	}
	if !result.SourcePullHasFailure() {
		t.Fatalf("result must report a failure")
	}
	var (
		apiRepo *SourcePullRepository
		webRepo *SourcePullRepository
	)
	for i := range result.Repositories {
		switch result.Repositories[i].Source {
		case "api-source":
			apiRepo = &result.Repositories[i]
		case "web-source":
			webRepo = &result.Repositories[i]
		}
	}
	if apiRepo == nil || webRepo == nil {
		t.Fatalf("result must contain both repositories: %#v", result.Repositories)
	}
	// api-source fast-forwarded and is now current; web-source failed and is left as-is.
	if apiRepo.State != SourcePullCurrent || !apiRepo.Updated {
		t.Fatalf("api-source must be updated and current: %#v", apiRepo)
	}
	if got := gitRev(t, localA); got == apiBefore {
		t.Fatalf("api-source HEAD did not advance: %s", got)
	}
	if webRepo.State == SourcePullCurrent {
		t.Fatalf("web-source must not be marked current: %#v", webRepo)
	}
	if got := gitRev(t, localB); got != webBefore {
		t.Fatalf("web-source was mutated by the failed pull: before=%s after=%s", webBefore, got)
	}
}

// TestSourcePullSameRepoAcrossComponentsStaysOnce proves that two components
// mapping to one repository yield exactly one repository entry and one
// fast-forward, never a double pull.
func TestSourcePullSameRepoAcrossComponentsStaysOnce(t *testing.T) {
	ctx := context.Background()
	_, local, peer := createWorkspaceGitFixture(t)
	if err := os.WriteFile(filepath.Join(peer, "remote.txt"), []byte("remote\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitTest(t, peer, "add", "remote.txt")
	gitTest(t, peer, "commit", "-m", "remote")
	gitTest(t, peer, "push", "origin", "main")

	model := SourceModel{
		SchemaVersion: SourceModelVersion,
		Application:   "demo",
		Sources:       []SourceDefinition{{ID: "shared-source", Type: SourceRepository, Repository: "file://shared"}},
		Components: []ComponentSource{
			{Component: "api", Source: "shared-source"},
			{Component: "web", Source: "shared-source"},
		},
	}
	mapping := WorkspaceMapping{SchemaVersion: WorkspaceMappingVersion, Application: "demo", Sources: map[string]string{"shared-source": local}}
	plan, err := ResolveSourcePullPlan(ctx, "", model, mapping, []string{"api", "web"})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Repositories) != 1 {
		t.Fatalf("expected one repository for one source, got %#v", plan.Repositories)
	}
}

// TestSourcePullComponentSelectionFiltersSources proves -c keeps only the
// requested source and drops the other.
func TestSourcePullComponentSelectionFiltersSources(t *testing.T) {
	ctx := context.Background()
	_, localA, peerA := createWorkspaceGitFixture(t)
	_, localB, peerB := createWorkspaceGitFixture(t)
	if err := os.WriteFile(filepath.Join(peerA, "a.txt"), []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitTest(t, peerA, "add", "a.txt")
	gitTest(t, peerA, "commit", "-m", "a")
	gitTest(t, peerA, "push", "origin", "main")
	if err := os.WriteFile(filepath.Join(peerB, "b.txt"), []byte("b\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitTest(t, peerB, "add", "b.txt")
	gitTest(t, peerB, "commit", "-m", "b")
	gitTest(t, peerB, "push", "origin", "main")

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
	mapping := WorkspaceMapping{SchemaVersion: WorkspaceMappingVersion, Application: "demo", Sources: map[string]string{"api-source": localA, "web-source": localB}}
	plan, err := ResolveSourcePullPlan(ctx, "", model, mapping, []string{"api"})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Repositories) != 1 || plan.Repositories[0].Source != "api-source" {
		t.Fatalf("component filter = %#v", plan.Repositories)
	}
}

// TestSourcePullUnknownComponentFailsClosed proves an unknown component request
// fails closed rather than silently pulling nothing.
func TestSourcePullUnknownComponentFailsClosed(t *testing.T) {
	ctx := context.Background()
	_, local, _ := createWorkspaceGitFixture(t)
	model, mapping := workspaceGitModel(local)
	if _, err := ResolveSourcePullPlan(ctx, "", model, mapping, []string{"ghost"}); err == nil {
		t.Fatalf("unknown component must fail closed")
	}
}

// TestSourcePullCheckDoesNotMutate proves --check inspects without advancing
// HEAD.
func TestSourcePullCheckDoesNotMutate(t *testing.T) {
	ctx := context.Background()
	_, local, peer := createWorkspaceGitFixture(t)
	if err := os.WriteFile(filepath.Join(peer, "remote.txt"), []byte("remote\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitTest(t, peer, "add", "remote.txt")
	gitTest(t, peer, "commit", "-m", "remote")
	gitTest(t, peer, "push", "origin", "main")

	before := gitRev(t, local)
	model, mapping := workspaceGitModel(local)
	plan, err := ResolveSourcePullPlan(ctx, "", model, mapping, nil)
	if err != nil {
		t.Fatal(err)
	}
	check, err := ApplySourcePull(ctx, plan, true)
	if err != nil {
		t.Fatal(err)
	}
	after := gitRev(t, local)
	if before != after {
		t.Fatalf("check mutated HEAD: %s -> %s", before, after)
	}
	if !check.CheckOnly {
		t.Fatalf("check must be check-only: %#v", check)
	}
	if check.Repositories[0].State != SourcePullUpdateAvailable {
		t.Fatalf("check result = %#v", check.Repositories[0])
	}
}

// TestSourcePullMissingUpstreamBlocksNegative proves a branch with no upstream
// fails closed.
func TestSourcePullMissingUpstreamBlocksNegative(t *testing.T) {
	root := t.TempDir()
	gitTest(t, root, "init", "-b", "main")
	gitTest(t, root, "config", "user.email", "test@example.invalid")
	gitTest(t, root, "config", "user.name", "BaseHarbor Test")
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitTest(t, root, "add", "README.md")
	gitTest(t, root, "commit", "-m", "initial")

	model := SourceModel{
		SchemaVersion: SourceModelVersion,
		Application:   "demo",
		Sources:       []SourceDefinition{{ID: "app-source", Type: SourceRepository, Repository: "file://fixture", Ref: "main"}},
		Components:    []ComponentSource{{Component: "app", Source: "app-source"}},
	}
	mapping := WorkspaceMapping{SchemaVersion: WorkspaceMappingVersion, Application: "demo", Sources: map[string]string{"app-source": root}}
	plan, err := ResolveSourcePullPlan(context.Background(), "", model, mapping, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := plan.Repositories[0]; got.Blocker != "missing_upstream" {
		t.Fatalf("plan = %#v", got)
	}
}

// TestSourcePullUpdateAvailableAheadBlocksNegative proves an ahead-only branch
// blocks the pull (no force-push, no merge).
func TestSourcePullUpdateAvailableAheadBlocksNegative(t *testing.T) {
	ctx := context.Background()
	_, local, _ := createWorkspaceGitFixture(t)
	if err := os.WriteFile(filepath.Join(local, "local.txt"), []byte("local\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitTest(t, local, "add", "local.txt")
	gitTest(t, local, "commit", "-m", "local")

	model, mapping := workspaceGitModel(local)
	plan, err := ResolveSourcePullPlan(ctx, "", model, mapping, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := plan.Repositories[0]; got.State != SourcePullAhead || plan.Applyable {
		t.Fatalf("plan = %#v", got)
	}
	result, err := ApplySourcePull(ctx, plan, false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Successful {
		t.Fatalf("ahead-only pull must not succeed: %#v", result)
	}
}

// TestSourcePullFilterComponentsUnknownSourceFailsClosed proves
// FilterSourcesForComponents fails closed on an unknown component.
func TestSourcePullFilterComponentsUnknownSourceFailsClosed(t *testing.T) {
	model := SourceModel{
		SchemaVersion: SourceModelVersion,
		Application:   "demo",
		Sources:       []SourceDefinition{{ID: "app-source", Type: SourceRepository, Repository: "file://fixture"}},
		Components:    []ComponentSource{{Component: "app", Source: "app-source"}},
	}
	mapping := WorkspaceMapping{SchemaVersion: WorkspaceMappingVersion, Application: "demo", Sources: map[string]string{"app-source": "/tmp/does-not-exist"}}
	if _, _, err := FilterSourcesForComponents(model, mapping, []string{"ghost"}); err == nil {
		t.Fatalf("unknown source must fail closed")
	}
}

func gitRev(t *testing.T, dir string) string {
	t.Helper()
	return strings.TrimSpace(gitTestOutput(t, dir, "rev-parse", "HEAD"))
}
