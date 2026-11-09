package deployment

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/delivery"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/stableid"
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

// TestListDeploymentRecordsDiscardsMalformedDirectoryNames proves the F2
// canonical directory-name invariant through a proven owner: a non-UUID deploy
// directory (even one that carries the BF-03 damaged signature) is discarded by
// listDeploymentRecords, never advertised as a deployment, and a valid-UUIDv4
// damaged directory is still surfaced as damaged-state. This is the
// no-false-admission half of the F3 invariant: malformed names cannot masquerade
// as valid deployment IDs.
func TestListDeploymentRecordsDiscardsMalformedDirectoryNames(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	target := "local"
	depDir := deploymentsNamespaceForTest(t, target)
	// A proven Core owner so the one-installation-owner seam admits the valid
	// UUID directory and the damaged-signature check runs on the production
	// owner path (the raw pre-fix listing would still flag a malformed dir).
	provisionOwnerForTarget(t, target)

	// A real UUIDv4 damaged directory: retained state, no deployment.json. A
	// non-v4 placeholder would itself be discarded by the shape gate, so this
	// must be a genuine version-4 identifier.
	validUUID, err := stableid.NewUUIDv4("deployment")
	if err != nil {
		t.Fatalf("generate UUIDv4: %v", err)
	}
	validDir := filepath.Join(depDir, validUUID)
	if err := os.MkdirAll(validDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(validDir, "application.env"), []byte("X=Y\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	// A malformed (non-UUIDv4) directory carrying the SAME damaged signature.
	malformedDir := filepath.Join(depDir, "foreign-orphan")
	if err := os.MkdirAll(malformedDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(malformedDir, "application.env"), []byte("X=Y\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	records, warnings, err := ListDeploymentsForDisplay(target)
	if err != nil {
		t.Fatalf("listing: %v", err)
	}

	// The malformed directory must be discarded by the shape gate: it is neither
	// surfaced as damaged-state nor returned as a record.
	for _, w := range warnings {
		if damaged, ok := DamagedDeploymentState(w, nil); ok && damaged.Identity.DeploymentID == "foreign-orphan" {
			t.Fatalf("malformed directory name must not be advertised as deployment damage; warning = %v", w)
		}
	}
	for _, r := range records {
		if r.Identity.DeploymentID == "foreign-orphan" {
			t.Fatalf("malformed directory name must not be returned as a deployment record")
		}
	}

	// The valid-UUID damaged directory MUST be surfaced as a damaged-state
	// warning: with a proven owner the retained-directory-no-record signature is
	// recognised on the production owner path, so the entry is visible.
	var foundDamaged bool
	for _, w := range warnings {
		damaged, ok := DamagedDeploymentState(w, nil)
		if !ok {
			continue
		}
		if damaged.Identity.DeploymentID == validUUID {
			foundDamaged = true
			if damaged.Root != validDir {
				t.Fatalf("valid-UUID damaged entry must report its own directory root; got %q want %q", damaged.Root, validDir)
			}
		}
	}
	if !foundDamaged {
		t.Fatalf("valid-UUID damaged directory must be surfaced as a damaged-state warning; warnings=%d", len(warnings))
	}
}

// TestListDeploymentRecordsDiscardsRecordDirectoryMismatch proves the F2 record
// / directory identity boundary: a valid-UUIDv4 directory whose deployment.json
// records a DIFFERENT deployment id fails closed as an integrity mismatch and is
// neither reclassified as a damaged orphan nor silently accepted as a record.
func TestListDeploymentRecordsDiscardsRecordDirectoryMismatch(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	target := "local"
	depDir := deploymentsNamespaceForTest(t, target)
	provisionOwnerForTarget(t, target)

	// A valid-UUIDv4 directory whose record claims a DIFFERENT deployment id:
	// record.Identity.DeploymentID does not match its storage path.
	dirUUID, err := stableid.NewUUIDv4("deployment")
	if err != nil {
		t.Fatalf("generate UUIDv4: %v", err)
	}
	dir := filepath.Join(depDir, dirUUID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	other, err := stableid.NewUUIDv4("deployment")
	if err != nil {
		t.Fatalf("generate UUIDv4: %v", err)
	}
	// A structurally valid record whose identity is a DIFFERENT deployment id
	// than its storage directory: loadDeploymentRecordFile must reject it as an
	// integrity mismatch before it can ever be advertised as a record.
	record := DeploymentRecord{
		Version: DeploymentRecordVersion,
		Identity: DeploymentIdentity{
			Target:        target,
			DeploymentID:  other,
			ApplicationID: "00000000-0000-4000-8000-000000000000",
			Application:   "app",
			Environment:   "dev",
		},
		Applied: AppliedDeployment{Delivery: delivery.Direct()},
	}
	payload, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "deployment.json"), payload, 0o600); err != nil {
		t.Fatal(err)
	}

	records, warnings, err := ListDeploymentsForDisplay(target)
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	// The mismatched record must not be returned as a valid record.
	for _, r := range records {
		if r.Identity.DeploymentID == dirUUID {
			t.Fatalf("record/directory identity mismatch must not be accepted as a deployment record")
		}
	}
	// It must surface as an integrity failure, not as a damaged orphan or an
	// accepted incomplete record.
	var surfaced bool
	for _, w := range warnings {
		if damaged, ok := DamagedDeploymentState(w, nil); ok {
			t.Fatalf("record/directory mismatch must not be reclassified as a damaged orphan; warning = %v", damaged)
		}
		if strings.Contains(strings.ToLower(w.Error()), "does not match") {
			surfaced = true
		}
	}
	if !surfaced {
		t.Fatalf("record/directory identity mismatch must surface as an integrity mismatch warning; warnings=%v", warnings)
	}
}
