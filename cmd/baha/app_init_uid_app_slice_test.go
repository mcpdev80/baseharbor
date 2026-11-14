package main

import (
	"bytes"
	"context"
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

// TestUIDAppSliceConcurrentFirstCreateConvergesOnOneOwner runs the real init
// entry point from multiple goroutines pointed at the same fresh root; the
// system-owned store serializes first create so exactly one canonical identity
// survives. Mirrors the deployment-registry two-process guard (0427-175) through
// the actual CLI surface.
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
	if err := os.WriteFile(filepath.Join(root, "compose.yaml"), []byte("services:\n  api:\n    image: example.invalid/raced:api\n    labels:\n      io.baseharbor.workload.protocol: https\n    ports:\n      - '8080:8080'\n"), 0600); err != nil {
		t.Fatal(err)
	}

	const racers = 6
	var wg sync.WaitGroup
	errs := make([]error, racers)
	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			prev, _ := os.Getwd()
			if err := os.Chdir(root); err != nil {
				errs[i] = err
				return
			}
			var out, errOut bytes.Buffer
			errs[i] = runWithIO(context.Background(), []string{"app", "init", "--quick", "--no-input"}, &out, &errOut)
			// Restore the process CWD inside the goroutine (before the temp
			// dir is cleaned up) so a racer never parks in a deleted CWD.
			_ = os.Chdir(prev)
		}(i)
	}
	wg.Wait()

	var successes int
	for _, err := range errs {
		if err == nil {
			successes++
		}
	}
	if successes == 0 {
		t.Fatalf("no init succeeded; first create must converge on exactly one owner")
	}

	items, err := canonicalOwnerStore(root).List()
	if err != nil {
		t.Fatal(err)
	}
	var claimed int
	for _, m := range items {
		if m.Name == "raced" {
			claimed++
		}
	}
	if claimed != 1 {
		t.Fatalf("concurrent first create left %d owners for one app name", claimed)
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
