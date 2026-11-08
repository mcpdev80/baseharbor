package deployment

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// fakeNetworkInspector is a test seam that returns a fixed InspectNetwork
// result for any network. It satisfies the minimal networkInspector surface the
// scoped scan needs without importing the full RuntimeProvider interface.
type fakeNetworkInspector struct {
	state bhruntime.RuntimeNetworkState
}

func (f fakeNetworkInspector) InspectNetwork(context.Context, string) (bhruntime.RuntimeNetworkState, error) {
	return f.state, nil
}

// TestDamagedDeploymentNamespaceScopedNamesNetworkWhenAbsent verifies that the
// app-scoped scan reports BOTH halves of the observed BF-03 damage: the missing
// record AND the absent workload runtime network for THIS app's identity.
func TestDamagedDeploymentNamespaceScopedNamesNetworkWhenAbsent(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	target := "local"
	depDir := deploymentsNamespaceForTest(t, target)

	// A retained deploy directory with orphaned runtime state but no record.
	damagedDir := filepath.Join(depDir, "damaged-net")
	if err := os.MkdirAll(damagedDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(damagedDir, "application.env"), []byte("X=Y\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	scoped := fakeNetworkInspector{state: bhruntime.RuntimeNetworkStateAbsent}
	damaged, root, networkName, state, err := DamagedDeploymentNamespaceScoped(
		target, "scoped-absent", "dev", "hx3app", scoped)
	if err != nil {
		t.Fatalf("scoped scan (network absent): %v", err)
	}
	if !damaged || root != damagedDir {
		t.Fatalf("damaged=%v root=%q; want damaged=true root=%q", damaged, root, damagedDir)
	}
	// The derived workload network name for app hx3app / dev is
	// baseharbor-dev-workload-baseharbor-hx3app-dev.
	if networkName != "baseharbor-dev-workload-baseharbor-hx3app-dev" {
		t.Fatalf("network name = %q, want %q", networkName, "baseharbor-dev-workload-baseharbor-hx3app-dev")
	}
	if state != bhruntime.RuntimeNetworkStateAbsent {
		t.Fatalf("network state = %q, want %q", state, bhruntime.RuntimeNetworkStateAbsent)
	}
}

// TestDamagedDeploymentNamespaceScopedPresentNamesOnlyRecord verifies that a
// present workload network still flags the record damage (record alone is
// enough) but reports neither the absent-network nor a fabricated network name.
func TestDamagedDeploymentNamespaceScopedPresentNamesOnlyRecord(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	target := "local"
	depDir := deploymentsNamespaceForTest(t, target)

	damagedDir := filepath.Join(depDir, "damaged-present")
	if err := os.MkdirAll(damagedDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(damagedDir, "application.env"), []byte("X=Y\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	scoped := fakeNetworkInspector{state: bhruntime.RuntimeNetworkStatePresent}
	damaged, root, networkName, state, err := DamagedDeploymentNamespaceScoped(
		target, "scoped-present", "dev", "hx3app", scoped)
	if err != nil {
		t.Fatalf("scoped scan (network present): %v", err)
	}
	if !damaged || root != damagedDir {
		t.Fatalf("damaged=%v root=%q; want damaged=true root=%q", damaged, root, damagedDir)
	}
	// Present network is not THIS app's damage: the returned damage must not be
	// the absent-network state.
	if state == bhruntime.RuntimeNetworkStateAbsent {
		t.Fatalf("present network must not be reported as absent; got %q", state)
	}
	// A present network is still derived (the app identity is known); only the
	// absent state is dropped from the damage, never fabricated.
	if networkName != "baseharbor-dev-workload-baseharbor-hx3app-dev" {
		t.Fatalf("network name = %q, want the derived workload network", networkName)
	}
}

// TestDamagedDeploymentNamespaceScopedNoRuntimeFallsBackToRecordOnly verifies
// that a nil inspector still flags the record damage and never fabricates a
// missing network (namespace-only fallback, unchanged behaviour).
func TestDamagedDeploymentNamespaceScopedNoRuntimeFallsBackToRecordOnly(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	target := "local"
	depDir := deploymentsNamespaceForTest(t, target)

	damagedDir := filepath.Join(depDir, "damaged-nil")
	if err := os.MkdirAll(damagedDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(damagedDir, "application.env"), []byte("X=Y\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	damaged, root, networkName, state, err := DamagedDeploymentNamespaceScoped(
		target, "scoped-nil", "dev", "hx3app", nil)
	if err != nil {
		t.Fatalf("scoped scan (nil inspector): %v", err)
	}
	if !damaged || root != damagedDir {
		t.Fatalf("damaged=%v root=%q; want damaged=true root=%q", damaged, root, damagedDir)
	}
	if networkName != "" || state != "" {
		t.Fatalf("nil inspector must not fabricate a network; got name=%q state=%q", networkName, state)
	}
}

// TestDamagedDeploymentNamespaceScopedNoFalseAttributionToForeignApp verifies
// that the scan flags a foreign orphaned directory as damaged (record half) but
// never attributes the resolved app's own present network to it, and never
// reports a foreign project's network as THIS app's absent network.
func TestDamagedDeploymentNamespaceScopedNoFalseAttributionToForeignApp(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	target := "local"
	depDir := deploymentsNamespaceForTest(t, target)

	// Foreign orphan: retained state, no record for a DIFFERENT app.
	foreignDir := filepath.Join(depDir, "foreign-orphan")
	if err := os.MkdirAll(foreignDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(foreignDir, "application.env"), []byte("X=Y\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	// The resolved app (scoped-app) has a PRESENT network. The scan must flag
	// the foreign orphan as damaged (record half) but must not attribute the
	// resolved app's present network as absent damage.
	scoped := fakeNetworkInspector{state: bhruntime.RuntimeNetworkStatePresent}
	damaged, root, networkName, state, err := DamagedDeploymentNamespaceScoped(
		target, "scoped-app", "dev", "scoped-app", scoped)
	if err != nil {
		t.Fatalf("scoped scan (foreign orphan): %v", err)
	}
	if !damaged || root != foreignDir {
		t.Fatalf("damaged=%v root=%q; want damaged=true root=%q", damaged, root, foreignDir)
	}
	// The foreign orphan is damaged on the record half only; the resolved app's
	// present network is NOT reported as absent.
	if state == bhruntime.RuntimeNetworkStateAbsent {
		t.Fatalf("foreign orphan must not carry the resolved app's absent network; got %q", state)
	}
	// The derived network name for the resolved app is still reported; only the
	// absent state is dropped, never fabricated against the foreign orphan.
	if networkName != "baseharbor-dev-workload-baseharbor-scoped-app-dev" {
		t.Fatalf("network name = %q, want the derived workload network for the resolved app", networkName)
	}
}
