package development

import (
	"context"
	"errors"
	"fmt"
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

// Case 5a: change-during-read. A concurrent writer that mutates a file during
// the capture walk must be detected and abort the capture. The writer and the
// walk synchronize on a two-sided barrier (no timing/hope): the walk signals
// that the walk has begun, then the writer mutates a path the baseline
// recorded and *acks* the mutation, then the walk's MutationAcked seam fires on
// that exact path before the walk may proceed past it. The test asserts the
// mutation was actually observed by the guard, not merely timed: the change is
// PROVEN, and a guard that dropped the acknowledgement seam fails fast with a
// distinct named sentinel.
func TestSourceSnapshotDetectsChangeDuringRead(t *testing.T) {
	root := t.TempDir()
	// A large regular file so the walk spends time reading it, giving the writer
	// a window to mutate a second baseline path mid-walk.
	mustWriteSnapshotFile(t, root, "big.bin", 0o644, string(makeNBytes(512*1024)))
	mustWriteSnapshotFile(t, root, "touch.go", 0o644, "package main\n")
	mutate := filepath.Join(root, "touch.go")

	started := make(chan struct{})
	// ack fires from the guard's MutationAcked seam the instant Layer 1 observes
	// the mutation of touch.go, before the walk may proceed past that event.
	ack := make(chan struct{})
	done := make(chan struct{})
	go writer2(t, root, mutate, started, ack, done)

	captureErr := func() error {
		_, err := CaptureSource(context.Background(), root, CaptureOptions{
			Source: "https://git.example/app.git",
			ChangeHook: func(path string) {
				if path == "__walk_started__" {
					select {
					case started <- struct{}{}:
					default:
					}
				}
			},
			MutationAcked: func(path string) {
				if path != "touch.go" {
					return
				}
				select {
				case ack <- struct{}{}:
				default:
				}
			},
		})
		return err
	}()
	<-done

	// A guard that dropped the acknowledgement seam (mutant) never fires
	// MutationAcked on touch.go; the writer signals that miss through a named
	// sentinel inside the local bound, never the generic 10-minute timeout.
	select {
	case miss := <-ackMiss:
		t.Fatal(miss)
	default:
	}

	if captureErr == nil {
		t.Fatal("expected change-during-read guard to abort the capture")
	}
}

// writer2 runs the change-during-read scenario's concurrent mutation under the
// round-6 two-sided barrier. It waits for the walk to signal the walk has
// begun, mutates the target path (a path the baseline already recorded), then
// blocks until the guard's MutationAcked seam observes that exact mutation. The
// blocking on <-ack is the determinism proof: a guard that never acknowledges
// the observed mutation hangs here and fails instead of passing by luck.
//
// The wait is bounded by the test-local deadline: a seam-less mutant signals
// failure via a named sentinel error (errSnapshotMutationUnobserved) inside the
// bound — never the generic 10-minute test timeout.
func writer2(t *testing.T, root string, mutate string, started, ack chan struct{}, done chan struct{}) {
	defer close(done)
	<-started
	// The walk has begun; mutate the path the baseline recorded. Layer 1 (the
	// in-walk per-file barrier) observes this mutation on its re-stat of
	// touch.go and fires MutationAcked; the writer blocks there, so a guard that
	// dropped the acknowledgement seam fails instead of passing by luck.
	if err := os.Chtimes(mutate, now(), now()); err != nil {
		t.Errorf("chtimes: %v", err)
		return
	}
	select {
	case <-ack:
	case <-time.After(testLocalBound):
		ackMiss <- errSnapshotMutationUnobserved
	}
}

// errSnapshotMutationUnobserved is the named sentinel a MutationAcked-less
// mutant produces: the guard never acknowledged the writer's mutation on the
// mutated path within the test-local bound. The main test goroutine reports it
// with a fatal message so the mutant fails fast inside the bound, not by
// generic timeout.
var errSnapshotMutationUnobserved = errors.New("change-during-read MutationAcked never fired on the mutated path within the local bound")

// ackMiss carries the writer's acknowledgement-miss back to the main test
// goroutine, which owns all t.Fatal calls.
var ackMiss = make(chan error, 1)

// The final baseline-path verification (Layer 2, round-4) catches a writer
// that mutates an already-visited file while a later file is still being
// captured: the in-walk barrier only checks each path when the walk reaches
// it, so a mutation of an already-read path that lands after the walk passed
// it would otherwise slip past the barrier. This regression pins that gap.
//
// Discrimination (round 6): the writer uses the same two-sided barrier as the
// change-during-read test — it acks its mutation, then blocks on the guard's
// MutationAcked seam observing that exact path. Because aaa.bin is read first
// (lexically before the filler/ directory) and the filler files keep the walk
// busy afterward, aaa.bin is already visited when the writer acks; the in-walk
// barrier (Layer 1) already passed aaa.bin, so only Layer 2 (Layer 2 re-checks
// every baseline path after the walk) can abort. Removing Layer 2 leaves the
// capture succeeding and this test red.
//
// The control (no mutation) is a separate case below: with no writer mutation
// the capture must stay green even with the probe cell attached, which proves
// the red here is caused by an actual late mutation and not by the probe or the
// acknowledgement seam alone.
func TestSourceSnapshotFinalBaselineVerificationCatchesLateMutation(t *testing.T) {
	root := t.TempDir()
	// aaa.bin is read first (lexically before the filler/ directory), the 40
	// filler files keep the walk busy afterward, so a mutation of aaa.bin that
	// lands after the walk reported it read can only be caught by the post-walk
	// final sweep (Layer 2) — the in-walk barrier already passed aaa.bin.
	mustWriteSnapshotFile(t, root, "aaa.bin", 0o644, string(makeNBytes(256*1024)))
	for i := 0; i < 40; i++ {
		mustWriteSnapshotFile(t, root, fmt.Sprintf("filler/%02d.dat", i), 0o644, "filler")
	}
	mutate := filepath.Join(root, "aaa.bin")

	// ack fires from the guard's MutationAcked seam the instant Layer 2 observes
	// the late mutation of aaa.bin (Layer 1 already passed it during the walk).
	ack := make(chan struct{})
	// aaaRead fires from the walk's consumed-path ChangeHook when the walk reads
	// aaa.bin past its per-file barrier; the writer triggers its late mutation
	// from this signal so the mutation lands on an already-visited path that
	// Layer 1 can no longer see — only Layer 2 can catch it.
	aaaRead := make(chan struct{}, 1)
	done := make(chan struct{})
	go finalBaselineWriter(t, mutate, aaaRead, ack, done)

	_, err := CaptureSource(context.Background(), root, CaptureOptions{
		Source: "https://git.example/app.git",
		ChangeHook: func(path string) {
			if path == "__path_read_aaa.bin" {
				select {
				case aaaRead <- struct{}{}:
				default:
				}
			}
		},
		MutationAcked: func(path string) {
			if path != "aaa.bin" {
				return
			}
			select {
			case ack <- struct{}{}:
			default:
			}
		},
	})
	<-done

	// A Layer-2-less mutant never acks aaa.bin; the writer signals that miss
	// with a named sentinel inside the local bound, never the generic timeout.
	select {
	case miss := <-ackMiss:
		t.Fatal(miss)
	default:
	}

	if err == nil {
		t.Fatal("source_snapshot_test.go: expected final baseline-path verification to abort the capture on a late mutation of an already-visited file")
	}
}

// finalBaselineWriter runs the late-mutation scenario's concurrent writer under
// the round-6 two-sided barrier. It waits for the walk to signal it has read
// aaa.bin past its per-file barrier (aaaRead), then mutates aaa.bin — now an
// already-visited path the in-walk barrier (Layer 1) can no longer see — and
// blocks until the guard's MutationAcked seam observes that exact mutation
// (Layer 2). A guard that dropped Layer 2 hangs here and fails instead of
// passing by luck.
//
// The wait is bounded by the test-local deadline: a Layer-2-less mutant signals
// failure via a named sentinel error (errSnapshotMutationUnobserved) inside the
// bound — never the generic 10-minute test timeout.
func finalBaselineWriter(t *testing.T, mutate string, aaaRead, ack chan struct{}, done chan struct{}) {
	defer close(done)
	<-aaaRead
	// The walk has read aaa.bin; mutate the already-visited path so only Layer 2
	// (the post-walk sweep) can observe the divergence.
	if err := os.Chtimes(mutate, now(), now()); err != nil {
		t.Errorf("chtimes: %v", err)
		return
	}
	select {
	case <-ack:
	case <-time.After(testLocalBound):
		ackMiss <- errSnapshotMutationUnobserved
	}
}

// testLocalBound is the test-local deadline for the MutationAcked observation.
// It is short and explicit so a Layer-2-less mutant fails fast with a named
// error rather than blocking until the generic 10-minute test timeout (which
// must never again be the only red signal this round).
const testLocalBound = 5 * time.Second

// Control: the exact final-baseline test path with the probe cell attached but
// NO writer mutation. It MUST stay green, which proves that the red in
// TestSourceSnapshotFinalBaselineVerificationCatchesLateMutation is caused by an
// actual late mutation and not by the probe cell, the MutationAcked seam, or
// the acknowledgement barrier alone. Without this control the red is
// non-discriminating: it could fail even when nothing was written.
func TestSourceSnapshotFinalBaselineNoMutationStaysGreen(t *testing.T) {
	root := t.TempDir()
	mustWriteSnapshotFile(t, root, "aaa.bin", 0o644, string(makeNBytes(256*1024)))
	for i := 0; i < 40; i++ {
		mustWriteSnapshotFile(t, root, fmt.Sprintf("filler/%02d.dat", i), 0o644, "filler")
	}

	// The probe cell observes MutationAcked but performs no mutation of its own;
	// it also reports the write locally so a guard that acks on a non-mutated
	// path still works. With no mutation the guard never acks and the capture
	// succeeds, so the probe must not introduce an abort.
	probe := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-probe:
				return
			default:
			}
		}
	}()

	_, err := CaptureSource(context.Background(), root, CaptureOptions{
		Source: "https://git.example/app.git",
		MutationAcked: func(path string) {
			select {
			case probe <- struct{}{}:
			default:
			}
		},
	})
	close(probe)
	<-done

	if err != nil {
		t.Fatalf("CONTROL FAILED: no-mutation case produced an error (test not discriminating): %v", err)
	}
}

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
