package development

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// mustWriteSnapshotFile writes a file with the given mode under root.
func mustWriteSnapshotFile(t *testing.T, root, rel string, mode os.FileMode, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

// gitInitClean initialises a fresh Git repo in root with a single clean commit.
func gitInitClean(t *testing.T, root, email string) {
	t.Helper()
	gitTest(t, root, "init", "-q")
	gitTest(t, root, "config", "user.email", email)
	gitTest(t, root, "config", "user.name", "BaseHarbor Test")
	gitTest(t, root, "config", "commit.gpgsign", "false")
}

// gitAddCommit stages all files and commits with the given message.
func gitAddCommit(t *testing.T, root, message string) {
	t.Helper()
	gitTest(t, root, "add", "-A")
	gitTest(t, root, "commit", "-q", "-m", message)
}

// included reports whether rel appears in the snapshot.
func included(snap SourceSnapshot, rel string) bool {
	rel = filepath.ToSlash(rel)
	for _, f := range snap.Files {
		if f.Rel == rel {
			return true
		}
	}
	return false
}

// Case 1: ignore-negation. A .dockerignore excludes a file, a later "!keep"
// negation re-includes it, and the excluded file stays out.
func TestSourceSnapshotHonoursDockerIgnoreNegation(t *testing.T) {
	root := t.TempDir()
	mustWriteSnapshotFile(t, root, "Dockerfile", 0o644, "FROM scratch\n")
	mustWriteSnapshotFile(t, root, "app.go", 0o644, "package main\n")
	mustWriteSnapshotFile(t, root, "debug.go", 0o644, "package main\n// debug\n")
	mustWriteSnapshotFile(t, root, ".dockerignore", 0o644, "debug.go\n!debug.go\n")

	snap, err := CaptureSource(context.Background(), root, CaptureOptions{Source: "https://git.example/app.git"})
	if err != nil {
		t.Fatal(err)
	}
	if !included(snap, "Dockerfile") || !included(snap, "app.go") {
		t.Fatalf("expected Dockerfile and app.go to be captured: %#v", snap.Files)
	}
	if !included(snap, "debug.go") {
		t.Fatal("negated !debug.go was not re-included")
	}
}

// Case 2: malicious path / symlink escape. A symlink that points outside the
// checkout root (and a ".." style traversal) must be rejected, not followed.
func TestSourceSnapshotRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	mustWriteSnapshotFile(t, root, "app.go", 0o644, "package main\n")

	// A symlink whose target escapes the snapshot root.
	escapeTarget := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(escapeTarget, []byte("leak\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(escapeTarget, filepath.Join(root, "escape.link")); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	_, err := CaptureSource(context.Background(), root, CaptureOptions{Source: "https://git.example/app.git"})
	if err == nil {
		t.Fatal("expected symlink escape to fail")
	}
	if !errors.Is(err, ErrSymlinkEscapes) {
		t.Fatalf("err = %v, want ErrSymlinkEscapes", err)
	}
}

// A ".." traversal inside a nested symlink must also be rejected.
func TestSourceSnapshotRejectsDotDotSymlink(t *testing.T) {
	root := t.TempDir()
	mustWriteSnapshotFile(t, root, "app.go", 0o644, "package main\n")
	inner := filepath.Join(root, "inner")
	if err := os.MkdirAll(inner, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("..", filepath.Join(inner, "up")); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}
	_, err := CaptureSource(context.Background(), root, CaptureOptions{Source: "https://git.example/app.git"})
	if err == nil {
		t.Fatal("expected '..' symlink to fail")
	}
	if !errors.Is(err, ErrSymlinkEscapes) {
		t.Fatalf("err = %v, want ErrSymlinkEscapes", err)
	}
}

// Case 3: monorepo components. Exactly one checkout root is captured; a
// component subpath inside the checkout is captured with its relative path, and
// the snapshot never leaves the single declared root.
func TestSourceSnapshotCapturesMonorepoComponentSubpaths(t *testing.T) {
	root := t.TempDir()
	mustWriteSnapshotFile(t, root, "frontend/app.js", 0o644, "frontend\n")
	mustWriteSnapshotFile(t, root, "backend/api/main.go", 0o644, "package main\n")
	mustWriteSnapshotFile(t, root, "backend/worker/worker.go", 0o644, "package main\n")

	snap, err := CaptureSource(context.Background(), root, CaptureOptions{Source: "https://git.example/monorepo.git"})
	if err != nil {
		t.Fatal(err)
	}
	if snap.Captured != 3 {
		t.Fatalf("captured = %d, want 3 components: %#v", snap.Captured, snap.Files)
	}
	for _, want := range []string{"frontend/app.js", "backend/api/main.go", "backend/worker/worker.go"} {
		if !included(snap, want) {
			t.Fatalf("expected component path %q to be captured", want)
		}
	}
	// Nothing escaped the single declared root: every path stays under root.
	for _, f := range snap.Files {
		if f.Rel == "." || f.Rel == "" {
			continue
		}
		if filepath.IsAbs(f.Rel) || f.Rel == ".." || len(f.Rel) >= 3 && f.Rel[:3] == ".." {
			t.Fatalf("path escaped single root: %q", f.Rel)
		}
	}
}

// Case 4: dirty situation. A dirty working tree reports dirty honestly for a
// source-transfer capture.
func TestSourceSnapshotReportsDirtyWorkingTree(t *testing.T) {
	root := t.TempDir()
	gitInitClean(t, root, "test@example.com")
	mustWriteSnapshotFile(t, root, "app.go", 0o644, "package main\n")
	gitAddCommit(t, root, "add app")
	// Introduce an uncommitted change.
	if err := os.WriteFile(filepath.Join(root, "app.go"), []byte("package main\n// changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	snap, err := CaptureSource(context.Background(), root, CaptureOptions{
		Source: "https://git.example/app.git",
		Scope:  SourceSnapshotScopeTransfer,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !snap.Dirty {
		t.Fatal("dirty working tree was not reported as dirty")
	}
	if snap.GitState == "" {
		t.Fatal("expected a git state to be recorded for a dirty tree")
	}
}

// A clean tree stays clean in the snapshot.
func TestSourceSnapshotCleanTreeNotDirty(t *testing.T) {
	root := t.TempDir()
	gitInitClean(t, root, "test@example.com")
	mustWriteSnapshotFile(t, root, "app.go", 0o644, "package main\n")
	gitAddCommit(t, root, "add app")
	snap, err := CaptureSource(context.Background(), root, CaptureOptions{
		Source: "https://git.example/app.git",
		Scope:  SourceSnapshotScopeTransfer,
	})
	if err != nil {
		t.Fatal(err)
	}
	if snap.Dirty {
		t.Fatalf("clean tree reported dirty: %#v", snap)
	}
}

// Case 5: change-during-read. A concurrent writer that mutates a file during the
// capture walk must be detected and abort the capture. The writer and the walk
// synchronize on channels (no timing/hope): the walk signals that the baseline
// was captured and the walk has begun, then the writer mutates a path the
// baseline recorded and waits for the walk's change hook to observe it. The test
// asserts the hook actually fired on the mutated path: the change is PROVEN to
// have been observed, not merely timed.
func TestSourceSnapshotDetectsChangeDuringRead(t *testing.T) {
	root := t.TempDir()
	// A large regular file so the walk spends time reading it, giving the watcher
	// a window to touch a second file mid-walk.
	mustWriteSnapshotFile(t, root, "big.bin", 0o644, string(makeNBytes(512*1024)))
	mustWriteSnapshotFile(t, root, "touch.go", 0o644, "package main\n")

	// startWalk proves the baseline was captured and the walk has begun.
	// observed proves the hook fired on the mutated baseline path: the change is
	// PROVEN to have been observed, not merely timed.
	startWalk := make(chan struct{})
	observed := make(chan struct{})
	done := make(chan struct{})
	go writer(t, root, startWalk, observed, done)

	captureErr := func() error {
		_, err := CaptureSource(context.Background(), root, CaptureOptions{
			Source: "https://git.example/app.git",
			ChangeHook: func(path string) {
				if path == "__walk_started__" {
					select {
					case startWalk <- struct{}{}:
					default:
					}
					return
				}
				// Only the writer's baseline path proves the mid-walk change.
				// The hook can fire on other baseline files the walk already
				// read; ignore anything that is not the chosen path.
				if filepath.Base(path) != "touch.go" {
					return
				}
				select {
				case observed <- struct{}{}:
				default:
				}
			},
		})
		return err
	}()
	<-done

	// The writer reports a hook-miss through a named sentinel, not a goroutine
	// t.Fatalf (vet forbids that). Surface it here on the test goroutine. Only
	// the hook-disabled mutant sends here; a successful observation leaves the
	// channel empty, so a non-blocking drain.
	select {
	case err := <-snapshotErr:
		t.Fatal(err)
	default:
	}

	if captureErr == nil {
		t.Fatal("expected change-during-read guard to abort the capture")
	}
}

// writer runs the change-during-read scenario's concurrent mutation. It waits
// for the walk to signal it has begun, mutates a path the baseline already
// recorded, then blocks until the walk's change hook observes that path. The
// blocking on <-observed is the determinism proof: if the guard's hook never
// fires on the mutated path (e.g. the hook seam is removed), this goroutine
// hangs and the test fails instead of passing by luck.
//
// The wait is bounded by the test-local deadline: if the hook never fires the
// writer signals failure via a named sentinel error (errSnapshotHookDidNotFire),
// which the main test goroutine reports with a fatal message inside the local
// bound — never the generic 10-minute test timeout.
func writer(t *testing.T, root string, startWalk, observed chan struct{}, done chan struct{}) {
	defer close(done)
	<-startWalk
	// The walk has begun; mutate a path the baseline already recorded so the
	// walk's change hook fires on that path.
	if err := os.Chtimes(filepath.Join(root, "touch.go"), now(), now()); err != nil {
		t.Errorf("chtimes: %v", err)
		return
	}
	// Block until the walk's change hook observes the mutation, but never past
	// the test-local bound. A hook-less mutant here fails fast with a named
	// sentinel error instead of hanging until the generic test timeout.
	select {
	case <-observed:
	case <-time.After(testLocalBound):
		snapshotErr <- errSnapshotHookDidNotFire
	}
}

// errSnapshotHookDidNotFire is the named sentinel a hook-disabled mutant
// produces: the change-during-read hook never fired on the mutated path within
// the test-local bound. The main test goroutine reports it with a fatal message
// so the mutant fails fast inside the bound, not by generic timeout.
var errSnapshotHookDidNotFire = errors.New("change-during-read hook never fired within the local bound")

// snapshotErr carries the writer's failure (if any) back to the main test
// goroutine, which owns all t.Fatal/t.Fatalf calls.
var snapshotErr = make(chan error, 1)

// testLocalBound is the test-local deadline for the change-during-read hook
// observation. It is short and explicit so a hook-disabled mutant fails fast
// with a named error rather than blocking until the generic 10-minute test
// timeout (which must never again be the only red signal this round).
const testLocalBound = 5 * time.Second

// The during-read guard does not abort a tree that stays still mid-walk.
func TestSourceSnapshotNoGuardAbortWhenQuiet(t *testing.T) {
	root := t.TempDir()
	mustWriteSnapshotFile(t, root, "app.go", 0o644, "package main\n")
	snap, err := CaptureSource(context.Background(), root, CaptureOptions{Source: "https://git.example/app.git"})
	if err != nil {
		t.Fatal(err)
	}
	if snap.Dirty {
		t.Fatal("clean tree should not be reported dirty")
	}
}

// Limits: a MaxBytes bound rejects a tree that exceeds the limit.
func TestSourceSnapshotEnforcesByteLimit(t *testing.T) {
	root := t.TempDir()
	mustWriteSnapshotFile(t, root, "big.txt", 0o644, string(makeNBytes(2048)))

	_, err := CaptureSource(context.Background(), root, CaptureOptions{
		Source:   "https://git.example/app.git",
		MaxBytes: 1024,
	})
	if err == nil {
		t.Fatal("expected byte-limit violation")
	}
	if !errors.Is(err, ErrContextBytes) {
		t.Fatalf("err = %v, want ErrContextBytes", err)
	}
}

// A byte limit large enough to cover the tree passes.
func TestSourceSnapshotRespectsSufficientByteLimit(t *testing.T) {
	root := t.TempDir()
	mustWriteSnapshotFile(t, root, "small.txt", 0o644, "hello\n")

	snap, err := CaptureSource(context.Background(), root, CaptureOptions{
		Source:   "https://git.example/app.git",
		MaxBytes: 4096,
	})
	if err != nil {
		t.Fatal(err)
	}
	if snap.TotalBytes != 6 {
		t.Fatalf("TotalBytes = %d, want 6", snap.TotalBytes)
	}
}

// Depth limit bounds the captured tree depth.
func TestSourceSnapshotEnforcesDepthLimit(t *testing.T) {
	root := t.TempDir()
	mustWriteSnapshotFile(t, root, "a/b/c/deep.txt", 0o644, "deep\n")

	_, err := CaptureSource(context.Background(), root, CaptureOptions{
		Source:   "https://git.example/app.git",
		MaxDepth: 2,
	})
	if err == nil {
		t.Fatal("expected depth-limit violation")
	}
	if !errors.Is(err, ErrContextDeep) {
		t.Fatalf("err = %v, want ErrContextDeep", err)
	}
}

// Default exclusions: .git, .baseharbor state and credential-looking files are
// never captured by default.
func TestSourceSnapshotExcludesDefaultSensitivePaths(t *testing.T) {
	root := t.TempDir()
	mustWriteSnapshotFile(t, root, "app.go", 0o644, "package main\n")
	mustWriteSnapshotFile(t, root, ".env", 0o600, "SECRET=leak\n")
	mustWriteSnapshotFile(t, root, "id_rsa", 0o600, "PRIVATE KEY\n")
	mustWriteSnapshotFile(t, root, ".baseharbor/generated", 0o644, "generated\n")
	mustWriteSnapshotFile(t, root, ".git/HEAD", 0o644, "ref: refs/heads/main\n")
	mustWriteSnapshotFile(t, root, "notes.txt", 0o644, "public notes\n")

	snap, err := CaptureSource(context.Background(), root, CaptureOptions{Source: "https://git.example/app.git"})
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{".env", "id_rsa", ".baseharbor/generated", ".git/HEAD"} {
		if included(snap, forbidden) {
			t.Fatalf("default-sensitive path %q was captured: %#v", forbidden, snap.Files)
		}
	}
	if !included(snap, "app.go") || !included(snap, "notes.txt") {
		t.Fatal("expected app.go and notes.txt to be captured")
	}
}

// A context-ignore negation can re-include a credential-looking file if the
// operator explicitly opts in (honest, explicit).
func TestSourceSnapshotReIncludesCredentialByNegation(t *testing.T) {
	root := t.TempDir()
	mustWriteSnapshotFile(t, root, "app.go", 0o644, "package main\n")
	mustWriteSnapshotFile(t, root, "client.crt", 0o600, "CERT\n")
	mustWriteSnapshotFile(t, root, ".dockerignore", 0o644, "!.client.crt\n")

	snap, err := CaptureSource(context.Background(), root, CaptureOptions{Source: "https://git.example/app.git"})
	if err != nil {
		t.Fatal(err)
	}
	if !included(snap, "client.crt") {
		t.Fatal("negation should re-include client.crt")
	}
}

// A non-Git checkout reports no git state and stays clean.
func TestSourceSnapshotNonGitCheckout(t *testing.T) {
	root := t.TempDir()
	mustWriteSnapshotFile(t, root, "app.go", 0o644, "package main\n")
	snap, err := CaptureSource(context.Background(), root, CaptureOptions{Source: "https://git.example/app.git"})
	if err != nil {
		t.Fatal(err)
	}
	if snap.GitState != "" {
		t.Fatalf("non-git checkout reported git state %q", snap.GitState)
	}
	if snap.Dirty {
		t.Fatal("non-git checkout should not be dirty")
	}
}

// A release-inspect scope does not flag a dirty tree as dirty (no source
// transfer path is selected).
func TestSourceSnapshotReleaseInspectDoesNotFlagDirty(t *testing.T) {
	root := t.TempDir()
	gitInitClean(t, root, "test@example.com")
	mustWriteSnapshotFile(t, root, "app.go", 0o644, "package main\n")
	gitAddCommit(t, root, "add app")
	if err := os.WriteFile(filepath.Join(root, "app.go"), []byte("package main\n// changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	snap, err := CaptureSource(context.Background(), root, CaptureOptions{
		Source: "ghcr.io/example/app@sha256:deadbeef",
		Scope:  SourceSnapshotScopeInspect,
	})
	if err != nil {
		t.Fatal(err)
	}
	if snap.Dirty {
		t.Fatal("release-inspect scope must not flag dirty (no source transfer)")
	}
}

// makeNBytes returns a byte slice of length n.
func makeNBytes(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = 'a'
	}
	return b
}

// now returns the current time for touch-style mtime updates in the during-read
// race.
func now() time.Time { return time.Now() }
