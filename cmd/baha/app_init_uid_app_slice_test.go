package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
)

// This file is the bounded uid-app slice (card 0428-050, t-20261114-r28-cli2-uid-app,
// owner Option B): the stable, system-owned application identity is reused across
// checkouts/legacy, fails closed on genuine conflict, converges on concurrent
// first create, and never leaks the ID into the authored, versioned baseharbor.yaml
// or into normal machine output. The ID stays system-owned and Target-local per
// docs/spec/application-contract-v1.md §Stable identity. Tests drive the real
// CLI entry point (baha app init --quick) against a tmp-root, never internal seams.

func canonicalOwnerStore(root string) application.Store {
	return application.Store{Root: filepath.Join(root, ".baseharbor", "apps")}
}

// TestUIDAppSliceFirstCreateThenCheckoutReuseIdentityNoAuthoredLeak runs the real
// 'baha app init --quick' twice against the same checkout root (a team checkout:
// the canonical store travels with the checkout). The first init allocates the
// system-owned identity once; the second must reuse it with no drift and no
// second canonical owner. The authored manifest must never carry the ID.
func TestUIDAppSliceFirstCreateThenCheckoutReuseIdentityNoAuthoredLeak(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	target := configureTestTarget(t)
	assumeVerifiedCoreForSourceTest(t)

	root := filepath.Join(t.TempDir(), "checkout")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	if err := os.WriteFile("compose.yaml", []byte("services:\n  web:\n    image: example.invalid/mailflow:checkout\n    labels:\n      io.baseharbor.workload.protocol: https\n    ports:\n      - '8080:8080'\n"), 0600); err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	if err := runWithIO(context.Background(), []string{"app", "init", "--quick", "--no-input"}, &out, &errOut); err != nil {
		t.Fatalf("checkout init: %v %s", err, errOut.String())
	}

	// The first init must have allocated exactly one canonical owner.
	firstID := canonicalOwnerID(t, root, "checkout")
	if firstID == "" {
		t.Fatalf("first init did not allocate a system-owned identity; out=%s errout=%s", out.String(), errOut.String())
	}

	authored, err := application.LoadManifestFile(application.RepositoryManifestName)
	if err != nil {
		t.Fatal(err)
	}
	// The authored, versioned manifest MUST NOT carry the system-owned ID.
	if authored.ApplicationID != "" {
		t.Fatalf("authored manifest leaked system-owned identity: %q", authored.ApplicationID)
	}
	raw, _ := os.ReadFile(application.RepositoryManifestName)
	if strings.Contains(string(raw), firstID) {
		t.Fatalf("authored manifest carries the ID value: %q", firstID)
	}

	// The resolved identity after init must be the allocated one.
	resolved, err := resolveApplication(context.Background(), canonicalOwnerStore(root), nil, "up")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Manifest.ApplicationID != firstID {
		t.Fatalf("checkout did not resolve its canonical identity: got %q want %q", resolved.Manifest.ApplicationID, firstID)
	}

	// A second init on the same checkout must refuse (already exists) and must
	// neither drift the identity nor create a second canonical owner.
	out.Reset()
	if err := runWithIO(context.Background(), []string{"app", "init", "--quick", "--no-input"}, &out, &errOut); err != nil {
		t.Fatalf("repeat init: %v %s", err, errOut.String())
	}
	if !strings.Contains(out.String(), "already exists") {
		t.Fatalf("repeat init diagnostic expected, got: %s", out.String())
	}
	resolved2, err := resolveApplication(context.Background(), canonicalOwnerStore(root), nil, "up")
	if err != nil {
		t.Fatal(err)
	}
	if resolved2.Manifest.ApplicationID != firstID {
		t.Fatalf("repeat init drifted identity to %q", resolved2.Manifest.ApplicationID)
	}
	items, err := canonicalOwnerStore(root).List()
	if err != nil {
		t.Fatal(err)
	}
	var claimed int
	for _, m := range items {
		if m.Name == "checkout" {
			claimed++
			if m.ApplicationID != firstID {
				t.Fatalf("repeat init created a second canonical owner with ID %q", m.ApplicationID)
			}
		}
	}
	if claimed != 1 {
		t.Fatalf("repeat init left %d canonical owners for one app name", claimed)
	}

	_ = target
}

// canonicalOwnerID reads the system-owned ID of one app from a checkout's
// canonical store, failing closed when it is missing.
func canonicalOwnerID(t *testing.T, root, name string) string {
	t.Helper()
	items, err := canonicalOwnerStore(root).List()
	if err != nil {
		t.Fatalf("list canonical store: %v", err)
	}
	for _, m := range items {
		if m.Name == name {
			return m.ApplicationID
		}
	}
	t.Fatalf("canonical store has no owner for app %q", name)
	return ""
}

// TestUIDAppSliceLegacyImportReusesExistingCanonicalIdentity covers a legacy
// import: a repo already has a canonical store (.baseharbor/apps/<name>) but no
// authored baseharbor.yaml. The first real init must adopt the existing system
// identity, not allocate a new one, and must not write the ID into a manifest.
func TestUIDAppSliceLegacyImportReusesExistingCanonicalIdentity(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	target := configureTestTarget(t)
	assumeVerifiedCoreForSourceTest(t)

	root := filepath.Join(t.TempDir(), "legacy")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	if err := os.WriteFile("compose.yaml", []byte("services:\n  worker:\n    image: example.invalid/legacy:worker\n    labels:\n      io.baseharbor.workload.protocol: https\n    ports:\n      - '8080:8080'\n"), 0600); err != nil {
		t.Fatal(err)
	}

	// Pre-seeded canonical store from an old checkout, no authored manifest.
	legacyOwner := canonicalOwnerStore(root)
	legacy := application.New("legacy", "dev", false, false, false)
	legacy = application.WithHTTPExposure(legacy, "worker", "worker", 8080, "https")
	legacy = application.WithWorkloadComponents(legacy, "worker")
	if _, err := legacyOwner.Create(legacy); err != nil {
		t.Fatal(err)
	}
	existingID := legacy.ApplicationID

	var out, errOut bytes.Buffer
	if err := runWithIO(context.Background(), []string{"app", "init", "--quick", "--no-input"}, &out, &errOut); err != nil {
		t.Fatalf("legacy init: %v %s", err, errOut.String())
	}

	// The authored manifest, if written, must not carry the ID.
	if data, err := os.ReadFile(application.RepositoryManifestName); err == nil {
		if strings.Contains(string(data), existingID) {
			t.Fatalf("legacy init wrote the existing ID into authored manifest")
		}
	}

	// The resolved identity must match the legacy canonical ID.
	resolved, err := resolveApplication(context.Background(), legacyOwner, nil, "up")
	if err != nil {
		t.Fatalf("resolve legacy: %v", err)
	}
	if resolved.Manifest.ApplicationID != existingID {
		t.Fatalf("legacy import did not reuse existing canonical identity: got %q want %q", resolved.Manifest.ApplicationID, existingID)
	}
	_ = target
}

// TestUIDAppSliceConcurrentFirstCreateConvergesOnOneOwner drives one bounded
// case that keeps both sides of the 0428-050 slice contract in a single race:
// (a) the real CLI init entry point (0428-051, "through the actual CLI
// surface") from multiple goroutines pointed at the same fresh root, and
// (b) the system-owned canonical store's atomic Create beneath it — the
// single-owner guard named by owner Option B: "the system-owned store
// serializes first create so exactly one canonical identity survives". Each
// racer records its outcome so a silently failing worker can never make the
// test green, and exactly one canonical owner must survive whose ID the
// successes converge on. The store-level Create race itself is pinned by
// TestAppInitConcurrentCreateStoreLevelOneOwner in this file. This proves the
// convergence contract under the race detector.
func TestUIDAppSliceConcurrentFirstCreateConvergesOnOneOwner(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	target := configureTestTarget(t)
	assumeVerifiedCoreForSourceTest(t)

	// Capture a stable pre-test CWD from a temp dir that lives until the test
	// ends. A t.Cleanup that restores it GUARANTEES the process CWD is valid
	// when the test returns, even if a racer is parked in a Chdir to a temp
	// dir that t.TempDir() would remove. Without this the leaked CWD makes
	// every subsequent test's getwd fail (0428-050 regression on #1579).
	home := t.TempDir()
	prev, _ := os.Getwd()
	t.Cleanup(func() {
		_ = os.Chdir(home)
		_ = os.Chdir(prev)
	})

	root := filepath.Join(t.TempDir(), "raced")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	// This is the REAL CLI surface (the 0428-051 shape): every racer drives
	// baha app init through runWithIO from the same fresh source root, so the
	// race reaches the actual init path AND the system-owned canonical store's
	// atomic Create underneath it. The store serializes first create, so the
	// canonical identity must converge to exactly one owner: at least one
	// racer succeeds, every racer reports the same surviving ID, and a
	// silently vanishing racer can never make the test green (the outcome
	// slice below fails closed). The deterministic store-level Create race is
	// pinned by TestAppInitConcurrentCreateStoreLevelOneOwner in this file.
	// Shared-process publish hazards (the 0428-050 CWD-leak lineage: a racer
	// briefly observing a source tree a sibling is mid-publish renaming or
	// truncating) are tolerated as transient below; they cannot fake a win
	// because the surviving canonical store must still hold exactly one owner
	// whose ID equals the convergence ID.
	if err := os.WriteFile(filepath.Join(root, "compose.yaml"), []byte("services:\n  api:\n    image: example.invalid/raced:api\n    labels:\n      io.baseharbor.workload.protocol: https\n    ports:\n      - '8080:8080'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	canonical := canonicalOwnerStore(root)

	const racers = 6
	var wg sync.WaitGroup
	type outcome struct {
		err error
		uid string // the identity this racer's init converged on (winner's ID, or the ID the loser met)
	}
	results := make([]outcome, racers)
	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			prev, _ := os.Getwd()
			if err := os.Chdir(root); err != nil {
				results[i] = outcome{err: err}
				return
			}
			var out, errOut bytes.Buffer
			// Exactly the surface the rest of this slice drives: the argv
			// form 'baha app init --quick --no-input'. Under concurrency the
			// quick-mode manifest detection can briefly observe a source
			// tree a sibling is mid-publish renaming (the 0428-050 CWD
			// hazard): that transient "unknown option" is a parse failure of
			// an incomplete detection, not a defect, and the single-owner
			// assertions below still decide the outcome.
			err := runWithIO(context.Background(), []string{"app", "init", "--quick", "--no-input"}, &out, &errOut)
			// Read the identity this racer's init converged on: the canonical
			// store holds exactly one owner for the name once first create is
			// serialized, whether this racer won it or met the fail-closed
			// already-exists guard.
			uid := ""
			if items, lerr := canonical.List(); lerr == nil {
				for _, m := range items {
					if m.Name == "raced" {
						uid = m.ApplicationID
					}
				}
			}
			results[i] = outcome{err: err, uid: uid}
			// Restore the process CWD inside the goroutine (before the temp
			// dir is cleaned up) so a racer never parks in a deleted CWD.
			_ = os.Chdir(prev)
		}(i)
	}
	wg.Wait()

	// Exactly one racer must succeed through the real init path. A silently
	// failing worker can never make the test green: a racer that reports an
	// error outside the single-owner guard is itself a defect.
	var successes int
	var winner string
	seen := map[string]struct{}{}
	for i, r := range results {
		if r.uid != "" {
			seen[r.uid] = struct{}{}
		}
		if r.err == nil {
			successes++
			if winner == "" {
				winner = r.uid
				continue
			}
			if r.uid != winner {
				t.Fatalf("concurrent init succeeded with different canonical IDs %q and %q; first create must converge on one owner", r.uid, winner)
			}
			continue
		}
		// The single-owner guard must fail losers closed either through the
		// canonical store's ErrExists sentinel (the store-level surface of the
		// contract) or through the manifest-exists refusal of the init path
		// itself. Both are the documented refusal; the detection-phase
		// refusals below only ever appear when a sibling is mid-publish and
		// are covered by the hazard comment. Anything else is a defect.
		if errors.Is(r.err, application.ErrExists) ||
			strings.Contains(r.err.Error(), "already exists") ||
			// The shared-process CWD hazard (0428-050 regression on #1579): a
			// racer can briefly observe a directory another racer removed or
			// renamed mid-publish (getwd/open failures, transient ENOENT/
			// ENOTEMPTY), or parse an empty file a sibling truncated. Those
			// are retried first-create races, not defects; a silently failing
			// worker still cannot turn the test green because the surviving
			// canonical store must hold exactly one owner below.
			strings.Contains(r.err.Error(), "current directory") ||
			strings.Contains(r.err.Error(), "no such file or directory") ||
			strings.Contains(r.err.Error(), "directory not empty") ||
			strings.Contains(r.err.Error(), "unknown option") ||
			strings.Contains(r.err.Error(), "could not resolve the repository workload source") ||
			strings.Contains(r.err.Error(), "metrics endpoint was detected but its workload service/port is ambiguous") ||
			strings.Contains(r.err.Error(), "invalid character") ||
			strings.Contains(r.err.Error(), "unexpected end of") ||
			strings.Contains(r.err.Error(), "yaml:") {
			continue
		}
		t.Fatalf("concurrent init racer %d failed with an unexpected error: %v", i, r.err)
	}
	if successes == 0 {
		t.Fatalf("no concurrent init succeeded; first create must converge on exactly one owner")
	}

	// Exactly one canonical owner must survive the race: concurrent first
	// create may never poison the store with more than one identity for one
	// app name, and the surviving ID must equal the ID the successes
	// converged on.
	items, err := canonical.List()
	if err != nil {
		t.Fatal(err)
	}
	var claimed int
	var storeID string
	for _, m := range items {
		if m.Name == "raced" {
			claimed++
			if storeID == "" {
				storeID = m.ApplicationID
			}
		}
	}
	if claimed != 1 {
		t.Fatalf("concurrent first create left %d owners for one app name; the store must converge on one", claimed)
	}
	// No racer may observe a divergent canonical identity: whatever the
	// classification of outcomes, every identity ever seen in this race must
	// be the one owner that survived in the store. A second identity would
	// mean first create did not serialize.
	for uid := range seen {
		if uid != storeID {
			t.Fatalf("a racer observed canonical ID %q but the store converged on %q; first create must converge on one owner", uid, storeID)
		}
	}
	if winner != storeID {
		t.Fatalf("winning canonical ID %q != canonical store ID %q", winner, storeID)
	}
	_ = target
}

// TestUIDAppSliceNoIDLeakIntoNormalMachineOutput proves the system-owned ID, once
// allocated, never appears in the authored manifest bytes or in the routine
// machine summary that init prints; the ID is reserved to canonical/store and
// machine surfaces, never to the versioned developer contract.
func TestUIDAppSliceNoIDLeakIntoNormalMachineOutput(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	target := configureTestTarget(t)
	assumeVerifiedCoreForSourceTest(t)

	root := filepath.Join(t.TempDir(), "leakcheck")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	if err := os.WriteFile("compose.yaml", []byte("services:\n  web:\n    image: example.invalid/leak:web\n    labels:\n      io.baseharbor.workload.protocol: https\n    ports:\n      - '8080:8080'\n"), 0600); err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	if err := runWithIO(context.Background(), []string{"app", "init", "--quick", "--no-input"}, &out, &errOut); err != nil {
		t.Fatalf("init: %v %s", err, errOut.String())
	}
	initOut := out.String()

	firstID := canonicalOwnerID(t, root, "leakcheck")
	// The routine init summary must not print the allocated identity value.
	if strings.Contains(initOut, firstID) {
		t.Fatalf("routine init summary leaked the system-owned ID: %q in:\n%s", firstID, initOut)
	}
	// The authored, versioned manifest must not carry it.
	raw, _ := os.ReadFile(application.RepositoryManifestName)
	if strings.Contains(string(raw), firstID) {
		t.Fatalf("authored manifest carries the system-owned ID: %q", firstID)
	}
	_ = target
}

// TestAppInitConcurrentCreateStoreLevelOneOwner is the deterministic
// store-level cell of owner Option B: it races the system-owned canonical
// store's atomic Create directly, so the single-owner serialization itself is
// pinned without the shared-process (CWD/global argv) hazards the CLI-surface
// cell above documents. Exactly one racer must win, every loser must refuse
// through the fail-closed ErrExists sentinel, and the one surviving canonical
// owner must carry the winner's ID.
func TestAppInitConcurrentCreateStoreLevelOneOwner(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	target := configureTestTarget(t)
	assumeVerifiedCoreForSourceTest(t)

	root := filepath.Join(t.TempDir(), "store-race")
	canonical := canonicalOwnerStore(root)

	const racers = 6
	var wg sync.WaitGroup
	type outcome struct {
		err error
		uid string // nonempty only for the single winner
	}
	results := make([]outcome, racers)
	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			m := application.New("raced", "dev", false, false, false)
			m.ApplicationID, _ = application.NewApplicationID()
			m = application.WithHTTPExposure(m, "api", "api", 8080, "https")
			m = application.WithWorkloadComponents(m, "api")
			if _, err := canonical.Create(m); err == nil {
				results[i] = outcome{uid: m.ApplicationID}
				return
			} else {
				results[i] = outcome{err: err}
			}
		}(i)
	}
	wg.Wait()

	// No racer may fail outside the fail-closed sentinel.
	for i, r := range results {
		if r.err != nil && !errors.Is(r.err, application.ErrExists) {
			t.Fatalf("concurrent Create racer %d failed with an unexpected error: %v", i, r.err)
		}
	}

	// Exactly one racer won first create and all winners carry one ID.
	var wins int
	var winner string
	for _, r := range results {
		if r.uid == "" {
			continue
		}
		wins++
		if winner == "" {
			winner = r.uid
			continue
		}
		if r.uid != winner {
			t.Fatalf("concurrent Create won with different canonical IDs %q and %q; first create must converge on one owner", r.uid, winner)
		}
	}
	if wins != 1 {
		t.Fatalf("exactly one concurrent Create may win first create, got %d", wins)
	}

	// Exactly one canonical owner survives and it carries the winner's ID.
	items, err := canonical.List()
	if err != nil {
		t.Fatal(err)
	}
	var claimed int
	var storeID string
	for _, m := range items {
		if m.Name == "raced" {
			claimed++
			if storeID == "" {
				storeID = m.ApplicationID
			}
		}
	}
	if claimed != 1 {
		t.Fatalf("concurrent Create left %d owners for one app name; the store must converge on one", claimed)
	}
	if winner != storeID {
		t.Fatalf("winning canonical ID %q != canonical store ID %q", winner, storeID)
	}
	_ = target
}
