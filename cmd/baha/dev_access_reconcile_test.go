package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/devaccess"
	"github.com/mcpdev80/baseharbor/internal/openbao"
	platformopenbao "github.com/mcpdev80/baseharbor/internal/openbao"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// nilRuntime returns nil runtime/files for the openBaoRuntime seam so the
// reconcile path reaches the managed seam without a real runtime.
func nilRuntime(context.Context) (bhruntime.RuntimeProvider, bhruntime.Files, error) {
	return nil, bhruntime.Files{}, nil
}

// initializedInspect returns an initialized, unsealed OpenBao state for the
// openBaoInspect seam.
func initializedInspect(context.Context, openbao.Executor, bhruntime.Files) (platformopenbao.State, error) {
	return platformopenbao.State{Initialized: true, Sealed: false}, nil
}

// countedManaged records the managed Get/Set pairs exercised by one reconcile
// and the value currently held in the managed authority. It stands in for the
// production OpenBao managed-secret authority on the dev-access reconcile path.
type countedManaged struct {
	gets     int
	sets     int
	value    []byte
	notfound bool // when true the next Get reports ErrManagedCredentialNotFound
	// concurrent, when set, simulates a managed object created after the first
	// not-found lookup. The create-only/CAS seed then no-ops (returns
	// ErrManagedCredentialAlreadyExists) and the existing value wins.
	concurrent []byte
	// mutant disables the create-only guard: the seed always overwrites,
	// modelling the round-2 unconditional SetManagedCredential write.
	mutant bool
}

func (c *countedManaged) get(_ context.Context, _ openbao.Executor, _ bhruntime.Files, _ string) ([]byte, error) {
	c.gets++
	if c.notfound {
		c.notfound = false
		return nil, platformopenbao.ErrManagedCredentialNotFound
	}
	if c.value == nil {
		return nil, platformopenbao.ErrManagedCredentialNotFound
	}
	out := make([]byte, len(c.value))
	copy(out, c.value)
	return out, nil
}

// set simulates the create-only/CAS seed of SetManagedCredentialIfAbsent. With
// the guard in place (mutant false) a concurrent value was injected between the
// Exists check and the seed makes the write a no-op: the existing managed value
// wins, no Set is counted, and the concurrent winner becomes authoritative so
// the caller's read-back returns it. With the guard removed (mutant true) the
// seed always overwrites with the stale local candidate — the round-2 defect
// round-3 closes.
func (c *countedManaged) set(_ context.Context, _ openbao.Executor, _ bhruntime.Files, _ string, value []byte) error {
	if c.mutant {
		c.sets++
		c.value = append([]byte(nil), value...)
		return nil
	}
	if c.concurrent != nil {
		c.value = append([]byte(nil), c.concurrent...)
		return platformopenbao.ErrManagedCredentialAlreadyExists
	}
	c.sets++
	c.value = append([]byte(nil), value...)
	return nil
}

// restoreReconcileSeams returns the production seams after each test.
func restoreReconcileSeams(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		openBaoRuntime = defaultOpenBaoRuntime
		managedCredentialSeam = managedCredentialSeamDefaults
		devRecoveryFactory = defaultDevRecoveryFactory
	})
}

func seedLocalOptIn(t *testing.T, target string) {
	t.Helper()
	if _, err := devaccess.Configure(target, "dev", "developer", "local-opt-in"); err != nil {
		t.Fatalf("seed local opt-in credential: %v", err)
	}
}

// TestReconcileSeedsManagedAuthorityExactlyOnce asserts the fixed fresh-device
// path issues exactly one managed Set (the single seed) and returns the
// actually-managed value, never a second write.
func TestReconcileSeedsManagedAuthorityExactlyOnce(t *testing.T) {
	root := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", root)
	seedLocalOptIn(t, "freshdevice")

	managed := &countedManaged{}
	openBaoRuntime = nilRuntime
	openBaoInspect = initializedInspect
	openBaoCheckManager = func(context.Context, openbao.Executor, bhruntime.Files) error { return nil }
	managedCredentialSeam = managedCredentialSeamFuncs{
		get: managed.get,
		set: managed.set,
	}
	devRecoveryFactory = func() devaccess.Recovery {
		return devaccess.Recovery{
			Managed: &reconcileManagedProbe{managed: managed},
			Store:   &reconcileLocalStore{path: filepath.Join(root, "dev", "credentials.env"), managed: managed},
		}
	}
	restoreReconcileSeams(t)

	result, err := reconcileDeveloperCredentialAuthority(context.Background(), "freshdevice", "dev", devaccess.Credentials{Username: "candidate", Password: "stale"}, false)
	if err != nil {
		t.Fatalf("reconcile returned error: %v", err)
	}
	if managed.sets != 1 {
		t.Fatalf("managed Set calls = %d, want exactly 1 (single seed, no second write)", managed.sets)
	}
	if managed.gets < 1 {
		t.Fatalf("managed Get calls = %d, want at least 1 read-back after seed", managed.gets)
	}
	if result.Username != "developer" || result.Password != "local-opt-in" {
		t.Fatalf("reconcile returned credential %+v, want the actually-managed seeded value", result)
	}
}

// TestReconcilePreservesConcurrentManagedValue exercises the race window
// round-3 closes: the initial managed Get returns NOT_FOUND, Recovery runs
// through to its Exists + seed step, and a concurrent managed value appears
// INSIDE the test between Exists and the seed. The create-only/CAS seed then
// no-ops without overwriting, the existing managed value wins, and exactly zero
// overwriting Sets issue on the reconcile.
func TestReconcilePreservesConcurrentManagedValue(t *testing.T) {
	root := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", root)
	seedLocalOptIn(t, "concurrentdevice")

	managed := &countedManaged{
		// The concurrent value is installed only as c.concurrent (not as
		// c.value) so the first Get still returns NOT_FOUND and Recovery runs
		// through to its seed step; the seed itself then sees the concurrent
		// winner and no-ops.
		concurrent: mustEncode("concurrent", "concurrent-secret"),
	}
	openBaoRuntime = nilRuntime
	openBaoInspect = initializedInspect
	openBaoCheckManager = func(context.Context, openbao.Executor, bhruntime.Files) error { return nil }
	managedCredentialSeam = managedCredentialSeamFuncs{
		get: managed.get,
		set: managed.set,
	}
	devRecoveryFactory = func() devaccess.Recovery {
		return devaccess.Recovery{
			Managed: &reconcileManagedProbe{managed: managed},
			Store:   &reconcileLocalStore{path: filepath.Join(root, "dev", "credentials.env"), managed: managed},
		}
	}
	restoreReconcileSeams(t)

	result, err := reconcileDeveloperCredentialAuthority(context.Background(), "concurrentdevice", "dev", devaccess.Credentials{Username: "candidate", Password: "stale"}, false)
	if err != nil {
		t.Fatalf("reconcile returned error: %v", err)
	}
	if managed.sets != 0 {
		t.Fatalf("managed Set calls = %d, want 0 (concurrent managed value wins, seed no-op)", managed.sets)
	}
	if result.Username != "concurrent" || result.Password != "concurrent-secret" {
		t.Fatalf("reconcile returned credential %+v, want the concurrent managed value to win", result)
	}
}

// TestReconcileMutantUnconditionalSeedOverwritesConcurrentValue proves the
// create-only guard is load-bearing: the round-2 unconditional seed (the
// mutant that round-3 removes) clobbers the concurrent winner with the stale
// local candidate. This is the RED cell the mutant must fail.
func TestReconcileMutantUnconditionalSeedOverwritesConcurrentValue(t *testing.T) {
	root := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", root)
	seedLocalOptIn(t, "concurrentdevice")

	// The mutant seed always succeeds and overwrites c.value with the local
	// opt-in credential that Persist writes, clobbering the concurrent managed
	// winner instead of preserving it.
	managed := &countedManaged{
		concurrent: mustEncode("concurrent", "concurrent-secret"),
		mutant:     true,
	}
	openBaoRuntime = nilRuntime
	openBaoInspect = initializedInspect
	openBaoCheckManager = func(context.Context, openbao.Executor, bhruntime.Files) error { return nil }
	managedCredentialSeam = managedCredentialSeamFuncs{
		get: managed.get,
		set: managed.set,
	}
	devRecoveryFactory = func() devaccess.Recovery {
		return devaccess.Recovery{
			Managed: &reconcileManagedProbe{managed: managed},
			Store:   &reconcileLocalStore{path: filepath.Join(root, "dev", "credentials.env"), managed: managed},
		}
	}
	restoreReconcileSeams(t)

	_, err := reconcileDeveloperCredentialAuthority(context.Background(), "concurrentdevice", "dev", devaccess.Credentials{Username: "candidate", Password: "stale"}, false)
	if err != nil {
		t.Fatalf("reconcile returned error: %v", err)
	}
	if managed.sets != 1 {
		t.Fatalf("mutant managed Set calls = %d, want 1 (unconditional seed overwrites)", managed.sets)
	}
	if string(managed.value) != string(mustEncode("developer", "local-opt-in")) {
		t.Fatalf("mutant managed value = %q, want the local opt-in credential to have clobbered the concurrent winner", managed.value)
	}
}

// TestReconcileFreshDeviceSeedsExactlyOnce confirms the uncontended fresh path
// seeds the managed authority exactly once with the local opt-in value.
func TestReconcileFreshDeviceSeedsExactlyOnce(t *testing.T) {
	root := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", root)
	seedLocalOptIn(t, "freshdevice")

	managed := &countedManaged{}
	openBaoRuntime = nilRuntime
	openBaoInspect = initializedInspect
	openBaoCheckManager = func(context.Context, openbao.Executor, bhruntime.Files) error { return nil }
	managedCredentialSeam = managedCredentialSeamFuncs{
		get: managed.get,
		set: managed.set,
	}
	devRecoveryFactory = func() devaccess.Recovery {
		return devaccess.Recovery{
			Managed: &reconcileManagedProbe{managed: managed},
			Store:   &reconcileLocalStore{path: filepath.Join(root, "dev", "credentials.env"), managed: managed},
		}
	}
	restoreReconcileSeams(t)

	result, err := reconcileDeveloperCredentialAuthority(context.Background(), "freshdevice", "dev", devaccess.Credentials{Username: "candidate", Password: "stale"}, false)
	if err != nil {
		t.Fatalf("reconcile returned error: %v", err)
	}
	if managed.sets != 1 {
		t.Fatalf("managed Set calls = %d, want exactly 1 (single create-only seed)", managed.sets)
	}
	if string(managed.value) != string(mustEncode("developer", "local-opt-in")) {
		t.Fatalf("managed value = %q, want the seeded local opt-in value", managed.value)
	}
	if result.Username != "developer" || result.Password != "local-opt-in" {
		t.Fatalf("reconcile returned credential %+v, want the actually-managed seeded value", result)
	}
}

// TestReconcileExistingManagedReadsWithoutDuplicate asserts the existing-managed
// path returns the managed value with zero Sets (no duplicate write).
func TestReconcileExistingManagedReadsWithoutDuplicate(t *testing.T) {
	root := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", root)
	seedLocalOptIn(t, "existingdevice")

	managed := &countedManaged{value: mustEncode("existing", "existing-secret")}
	openBaoRuntime = nilRuntime
	openBaoInspect = initializedInspect
	openBaoCheckManager = func(context.Context, openbao.Executor, bhruntime.Files) error { return nil }
	managedCredentialSeam = managedCredentialSeamFuncs{
		get: managed.get,
		set: managed.set,
	}
	devRecoveryFactory = func() devaccess.Recovery {
		return devaccess.Recovery{
			Managed: &reconcileManagedProbe{managed: managed},
			Store:   &reconcileLocalStore{path: filepath.Join(root, "dev", "credentials.env"), managed: managed},
		}
	}
	restoreReconcileSeams(t)

	result, err := reconcileDeveloperCredentialAuthority(context.Background(), "existingdevice", "dev", devaccess.Credentials{Username: "candidate", Password: "stale"}, false)
	if err != nil {
		t.Fatalf("reconcile returned error: %v", err)
	}
	if managed.sets != 0 {
		t.Fatalf("managed Set calls = %d, want 0 (existing managed object used as-is)", managed.sets)
	}
	if result.Username != "existing" || result.Password != "existing-secret" {
		t.Fatalf("reconcile returned credential %+v, want the existing managed value", result)
	}
}

// TestReconcileFailsClosedWithoutOptIn retains the fail-closed cell: no local
// opt-in and no managed object must fail closed with no Set.
func TestReconcileFailsClosedWithoutOptIn(t *testing.T) {
	root := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", root)

	managed := &countedManaged{notfound: true}
	openBaoRuntime = nilRuntime
	openBaoInspect = initializedInspect
	openBaoCheckManager = func(context.Context, openbao.Executor, bhruntime.Files) error { return nil }
	managedCredentialSeam = managedCredentialSeamFuncs{
		get: managed.get,
		set: managed.set,
	}
	devRecoveryFactory = func() devaccess.Recovery {
		return devaccess.Recovery{
			Managed: &reconcileManagedProbe{managed: managed},
			Store:   &reconcileLocalStore{path: filepath.Join(root, "dev", "credentials.env"), managed: managed},
		}
	}
	restoreReconcileSeams(t)

	_, err := reconcileDeveloperCredentialAuthority(context.Background(), "absenttarget", "dev", devaccess.Credentials{Username: "candidate", Password: "stale"}, false)
	if !errors.Is(err, devaccess.ErrNoRecoverableCredential) {
		t.Fatalf("reconcile err = %v, want ErrNoRecoverableCredential (fail-closed)", err)
	}
	if managed.sets != 0 {
		t.Fatalf("managed Set calls = %d, want 0 on the fail-closed path", managed.sets)
	}
}

// reconcileManagedProbe adapts countedManaged to devaccess.ManagedAuthorityChecker.
type reconcileManagedProbe struct{ managed *countedManaged }

func (p *reconcileManagedProbe) Exists(_ context.Context, _ string) (bool, error) {
	if p.managed.value == nil {
		return false, nil
	}
	return true, nil
}

// reconcileLocalStore writes the local opt-in credential to the managed authority
// exactly once (one Set), mirroring the production seed via
// openBaoManagedCredentialStore.Persist, and to a temp file so the same store
// can be re-read on reconcile via the real devaccess.Load.
type reconcileLocalStore struct {
	path    string
	managed *countedManaged
}

func (s *reconcileLocalStore) Persist(_ context.Context, target, username, password string) error {
	data, err := json.Marshal(struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}{Username: username, Password: password})
	if err != nil {
		return err
	}
	if s.managed != nil {
		if setErr := s.managed.set(context.Background(), nil, bhruntime.Files{}, "ref", data); setErr != nil {
			return setErr
		}
	}
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return os.WriteFile(s.path, []byte("USERNAME="+username+"\nPASSWORD="+password+"\n"), 0o600)
}

func mustEncode(username, password string) []byte {
	data, err := json.Marshal(struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}{Username: username, Password: password})
	if err != nil {
		panic(fmt.Sprintf("encode managed value: %v", err))
	}
	return data
}
