package metrics

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
)

// TestPersesDashboardProvisionWritesEligibleDashboard proves the export
// CONSUMER of card 0428-181 (P4 of review 1650): a selected, VERIFIED Perses
// surface with a non-forgeable identity witness exports its eligible dashboard
// JSON under providers/management-ui/perses/provisioning/dashboards/ — the same
// sibling-of-runtime projection root the datasource materializer writes beside
// (0428-083 F1) — and the written resource is a kind-loaded Perses Dashboard.
//
// This is the "ZERO production consumers" fix: the catalog was built and unit
// tested, but never wrote a single dashboard. The gate is the REAL per-target
// probe (overridden to "verified" here) plus the identity witness, so the
// positive case exercises the real provision path end to end.
func TestPersesDashboardProvisionWritesEligibleDashboard(t *testing.T) {
	state := t.TempDir()
	m := application.New("perses-identity", "dev", true, false, false)
	m.Services.ObservabilityManagementUI = true
	m.Services.Perses = true
	m.Services.Identity = true

	runtimeDir := filepath.Join(filepath.Clean(state), m.Name, "runtime")
	if err := os.MkdirAll(runtimeDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// The gate: a "verified" backend probe AND a non-forgeable identity
	// witness. Both seams are stubbed so the positive path is deterministic
	// and probe-observable without a live backend.
	originalProbe := persesBackendProbe
	originalWitness := identitySelectionWitness
	t.Cleanup(func() {
		persesBackendProbe = originalProbe
		identitySelectionWitness = originalWitness
	})
	persesBackendProbe = func() persesInfraProbe { return verifiedPersesInfraProbe{} }
	identitySelectionWitness = func(m application.Manifest) capability.IdentitySelectionWitness {
		return capability.NewIdentitySelectionWitness(true, true, true, true)
	}

	root := datasourceProjectionRoot(runtimeDir)
	eligible := []string{"keycloak-identity"}
	if err := writePersesDashboardProjection(m, eligible, root); err != nil {
		t.Fatalf("writePersesDashboardProjection: %v", err)
	}

	// The dashboard JSON must sit at the derivation the seam owns, and the
	// writer/reader must agree on the path (F1).
	dir := filepath.Join(root, persesDatasourceProvisioningDir, persesDashboardProvisioningFolder)
	path := filepath.Join(dir, "keycloak-identity.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("eligible dashboard not exported: %v", err)
	}

	// The written resource is a kind-loaded Perses Dashboard: kind + metadata
	// resolve by name, exactly the loader's resolution key.
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var resource struct {
		Kind     string `json:"kind"`
		Metadata struct {
			Name string `json:"name"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(raw, &resource); err != nil {
		t.Fatalf("exported dashboard is not valid Perses JSON: %v", err)
	}
	if resource.Kind != PersesDashboardKind {
		t.Fatalf("exported dashboard kind = %q, want %q", resource.Kind, PersesDashboardKind)
	}
	if resource.Metadata.Name != "keycloak-identity" {
		t.Fatalf("exported dashboard name = %q, want %q", resource.Metadata.Name, "keycloak-identity")
	}
}

// TestPersesDashboardProvisionDeselectedWritesNothing proves the fail-closed
// selection axis: a deselected Perses surface exports no dashboard and clears
// any stale dashboard (a dropped-but-unverified dashboard must never reach
// Perses).
func TestPersesDashboardProvisionDeselectedWritesNothing(t *testing.T) {
	state := t.TempDir()
	m := application.New("perses-deselect", "dev", false, false, false)
	m.Services.ObservabilityManagementUI = true
	m.Services.Perses = false

	runtimeDir := filepath.Join(filepath.Clean(state), m.Name, "runtime")
	if err := os.MkdirAll(runtimeDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Seed a stale dashboard to prove the deselected surface clears it.
	root := datasourceProjectionRoot(runtimeDir)
	staleDir := filepath.Join(root, persesDatasourceProvisioningDir, persesDashboardProvisioningFolder)
	if err := os.MkdirAll(staleDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staleDir, "keycloak-identity.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	originalProbe := persesBackendProbe
	originalWitness := identitySelectionWitness
	t.Cleanup(func() {
		persesBackendProbe = originalProbe
		identitySelectionWitness = originalWitness
	})
	persesBackendProbe = func() persesInfraProbe { return verifiedPersesInfraProbe{} }
	identitySelectionWitness = func(m application.Manifest) capability.IdentitySelectionWitness {
		return capability.NewIdentitySelectionWitness(true, true, true, true)
	}

	if err := persesDashboardProvisionForLifecycle(m, runtimeDir, ""); err != nil {
		t.Fatalf("provision: %v", err)
	}
	if _, err := os.Stat(filepath.Join(staleDir, "keycloak-identity.json")); !os.IsNotExist(err) {
		t.Fatalf("deselected surface must clear stale dashboard, file still present: %v", err)
	}
}

// TestPersesDashboardProvisionUnverifiedWritesNothing proves the fail-closed
// probe axis: a selected but UNVERIFIED Perses surface (the real per-target
// probe answers no rows) exports no dashboard and clears any stale dashboard —
// "verified" is a probe result only, never an assumed exporter.
func TestPersesDashboardProvisionUnverifiedWritesNothing(t *testing.T) {
	state := t.TempDir()
	m := application.New("perses-unverified", "dev", true, false, false)
	m.Services.ObservabilityManagementUI = true
	m.Services.Perses = true
	m.Services.Identity = true

	runtimeDir := filepath.Join(filepath.Clean(state), m.Name, "runtime")
	if err := os.MkdirAll(runtimeDir, 0o755); err != nil {
		t.Fatal(err)
	}

	root := datasourceProjectionRoot(runtimeDir)
	staleDir := filepath.Join(root, persesDatasourceProvisioningDir, persesDashboardProvisioningFolder)
	if err := os.MkdirAll(staleDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staleDir, "prometheus-targets.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	originalProbe := persesBackendProbe
	originalWitness := identitySelectionWitness
	t.Cleanup(func() {
		persesBackendProbe = originalProbe
		identitySelectionWitness = originalWitness
	})
	// "unverified" probe: every panel series answers no rows, so the surface
	// never reports verified and exports nothing.
	persesBackendProbe = func() persesInfraProbe { return unavailablePersesInfraProbe{} }
	identitySelectionWitness = func(m application.Manifest) capability.IdentitySelectionWitness {
		return capability.NewIdentitySelectionWitness(true, true, true, true)
	}

	if err := persesDashboardProvisionForLifecycle(m, runtimeDir, ""); err != nil {
		t.Fatalf("provision: %v", err)
	}
	if _, err := os.Stat(filepath.Join(staleDir, "prometheus-targets.json")); !os.IsNotExist(err) {
		t.Fatalf("unverified surface must clear stale dashboard, file still present: %v", err)
	}
}

// persesInfraProbe stub answering "verified" for every panel series.
type verifiedPersesInfraProbe struct{}

func (verifiedPersesInfraProbe) querySeries(context.Context, string, string) (bool, error) {
	return true, nil
}
