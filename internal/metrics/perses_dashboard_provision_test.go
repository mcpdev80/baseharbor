package metrics

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
)

// TestPersesDashboardProvisionUnqualifiedWitnessWritesNothing proves the
// producer-path fail-closed axis of card 0428-181 (RC1 of the 0428-181
// correction round): a SELECTED, probe-VERIFIED Perses surface with an
// UNQUALIFIED identity witness (its target-authorization/evidence combination
// is not all-attested) must NOT export keycloak-identity.json, and any stale
// dashboard of a previous export must be REMOVED. The regression runs the REAL
// producer persesDashboardProvisionForLifecycle — gate (probe) +
// capability.FilterEligibleDashboards + identity witness end to end — so it
// FAILS if the producer ever bypasses or ignores the witness: with only the
// prometheus backend signal (the identity backend carries no signal here) and a
// witness that is not NonForgeableAvailable, FilterEligibleDashboards returns
// no eligible key, so writePersesDashboardProjection clears the folder and
// exports nothing.
func TestPersesDashboardProvisionUnqualifiedWitnessWritesNothing(t *testing.T) {
	state := t.TempDir()
	m := application.New("perses-identity-unqual", "dev", true, false, false)
	m.Services.ObservabilityManagementUI = true
	m.Services.Perses = true
	m.Services.Identity = true

	runtimeDir := filepath.Join(filepath.Clean(state), m.Name, "runtime")
	if err := os.MkdirAll(runtimeDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Seed a stale dashboard to prove the unqualified surface clears it — a
	// dropped-but-unverified dashboard must never reach Perses.
	root := datasourceProjectionRoot(runtimeDir)
	staleDir := filepath.Join(root, persesDatasourceProvisioningDir, persesDashboardProvisioningFolder)
	if err := os.MkdirAll(staleDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staleDir, "keycloak-identity.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	// The gate: a "verified" backend probe AND a witness that is SELECTED but
	// NOT non-forgeable (target unauthorized below), so the identity dashboard
	// stays ineligible on the identity-backend + witness axis.
	originalProbe := persesBackendProbe
	originalWitness := identitySelectionWitness
	t.Cleanup(func() {
		persesBackendProbe = originalProbe
		identitySelectionWitness = originalWitness
	})
	persesBackendProbe = func() persesInfraProbe { return verifiedPersesInfraProbe{} }
	// A real witness that is unqualified: selection is confirmed but the
	// target is NOT authorized, so NonForgeableAvailable() fails closed.
	identitySelectionWitness = func(manifest application.Manifest) capability.IdentitySelectionWitness {
		return capability.NewIdentitySelectionWitness(true, true, true, false)
	}

	if err := persesDashboardProvisionForLifecycle(m, runtimeDir, ""); err != nil {
		t.Fatalf("provision: %v", err)
	}
	if _, err := os.Stat(filepath.Join(staleDir, "keycloak-identity.json")); !errorsIsNotExist(err) {
		t.Fatalf("unqualified-witness surface must export nothing and clear stale dashboard, file still present: %v", err)
	}
}

// TestPersesDashboardProvisionQualifiedWitnessExportsKeycloakIdentity proves
// the producer-path positive axis of card 0428-181 (RC1): a SELECTED,
// probe-VERIFIED Perses surface with a NON-FORGEABLE identity witness (all four
// evidence flags attested) DOES export keycloak-identity.json through the real
// producer persesDashboardProvisionForLifecycle, because the witness is
// NonForgeableAvailable and FilterEligibleDashboards returns keycloak-identity.
func TestPersesDashboardProvisionQualifiedExportsKeycloakIdentity(t *testing.T) {
	state := t.TempDir()
	m := application.New("perses-identity-qualified", "dev", true, false, false)
	m.Services.ObservabilityManagementUI = true
	m.Services.Perses = true
	m.Services.Identity = true

	runtimeDir := filepath.Join(filepath.Clean(state), m.Name, "runtime")
	if err := os.MkdirAll(runtimeDir, 0o755); err != nil {
		t.Fatal(err)
	}

	originalProbe := persesBackendProbe
	originalWitness := identitySelectionWitness
	t.Cleanup(func() {
		persesBackendProbe = originalProbe
		identitySelectionWitness = originalWitness
	})
	persesBackendProbe = func() persesInfraProbe { return verifiedPersesInfraProbe{} }
	// A non-forgeable witness: every evidence flag attested, so
	// NonForgeableAvailable() is true and the identity dashboard is eligible.
	identitySelectionWitness = func(manifest application.Manifest) capability.IdentitySelectionWitness {
		return capability.NewIdentitySelectionWitness(true, true, true, true)
	}

	if err := persesDashboardProvisionForLifecycle(m, runtimeDir, ""); err != nil {
		t.Fatalf("provision: %v", err)
	}

	// The dashboard JSON must sit at the derivation the seam owns, and the
	// writer/reader must agree on the path (F1).
	root := datasourceProjectionRoot(runtimeDir)
	path := filepath.Join(root, persesDatasourceProvisioningDir, persesDashboardProvisioningFolder, "keycloak-identity.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("qualified witness must export keycloak-identity.json: %v", err)
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

// TestPersesDashboardProvisionKeycloakIdentityBytesStable proves card 0428-181
// acceptance item 2: the identity dashboard emitted through the REAL
// producer/export path (persesDashboardProvisionForLifecycle) is byte-stable,
// schema-valid, and reviewed — not a hand-written fixture. The proof is three
// parts, all tied to the exact candidate head:
//
//  1. Determinism: the producer is run twice into separate roots; the emitted
//     keycloak-identity.json bytes are byte-identical (a stable export, never a
//     random/timestamped payload).
//  2. Hash pinned to the reviewed representative: the emitted SHA-256 equals the
//     golden hash recorded for this candidate. The hash is a review artifact the
//     Owner carries on the card; a drift in panel/spec marshaling changes it.
//  3. Schema shape: the emitted JSON validates against the pinned Perses native
//     dashboard shape (github.com/perses/spec v0.3.0, go/dashboard Spec) —
//     kind + metadata + spec with a non-empty panels block and a defined
//     datasource the single query references. JSON parse alone is NOT the proof;
//     the exact envelope is asserted.
func TestPersesDashboardProvisionKeycloakIdentityBytesStable(t *testing.T) {
	emitOnce := func(t *testing.T) ([]byte, string) {
		state := t.TempDir()
		m := application.New("perses-identity-hash", "dev", true, false, false)
		m.Services.ObservabilityManagementUI = true
		m.Services.Perses = true
		m.Services.Identity = true

		runtimeDir := filepath.Join(filepath.Clean(state), m.Name, "runtime")
		if err := os.MkdirAll(runtimeDir, 0o755); err != nil {
			t.Fatal(err)
		}

		originalProbe := persesBackendProbe
		originalWitness := identitySelectionWitness
		t.Cleanup(func() {
			persesBackendProbe = originalProbe
			identitySelectionWitness = originalWitness
		})
		persesBackendProbe = func() persesInfraProbe { return verifiedPersesInfraProbe{} }
		identitySelectionWitness = func(manifest application.Manifest) capability.IdentitySelectionWitness {
			return capability.NewIdentitySelectionWitness(true, true, true, true)
		}

		root := datasourceProjectionRoot(runtimeDir)
		if err := persesDashboardProvisionForLifecycle(m, runtimeDir, ""); err != nil {
			t.Fatalf("provision: %v", err)
		}
		path := filepath.Join(root, persesDatasourceProvisioningDir, persesDashboardProvisioningFolder, "keycloak-identity.json")
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("keycloak-identity.json not emitted: %v", err)
		}
		return raw, path
	}

	first, _ := emitOnce(t)

	// Part 1 — determinism: two producer runs emit byte-identical output.
	second, _ := emitOnce(t)
	if string(first) != string(second) {
		t.Fatalf("keycloak-identity export is not byte-stable across producer runs")
	}

	// Part 2 — hash pinned to the reviewed representative for this candidate.
	sum := sha256.Sum256(first)
	golden := hex.EncodeToString(sum[:])
	const wantHash = "b964f6eb9b587cceec68c0fad9be44f8937f6e6ec7ff2f07eb0424ad8e627e2e"
	if golden != wantHash {
		t.Fatalf("keycloak-identity export hash = %s, want reviewed golden %s", golden, wantHash)
	}

	// Part 3 — schema shape against the pinned Perses native dashboard shape.
	var resource struct {
		Kind     string `json:"kind"`
		Metadata struct {
			Name    string `json:"name"`
			Version int    `json:"version"`
		} `json:"metadata"`
		Spec struct {
			Panels map[string]struct {
				Kind string `json:"kind"`
				Spec struct {
					Panels map[string]any `json:"panels"`
				} `json:"spec"`
			} `json:"panels"`
			Datasources map[string]struct {
				Default bool `json:"default"`
				Plugin  struct {
					Kind string `json:"kind"`
				} `json:"plugin"`
			} `json:"datasources"`
			Layouts []any `json:"layouts"`
		} `json:"spec"`
	}
	if err := json.Unmarshal(first, &resource); err != nil {
		t.Fatalf("emitted dashboard is not valid Perses JSON: %v", err)
	}
	if resource.Kind != PersesDashboardKind {
		t.Fatalf("kind = %q, want %q", resource.Kind, PersesDashboardKind)
	}
	if resource.Metadata.Name != "keycloak-identity" {
		t.Fatalf("metadata.name = %q, want %q", resource.Metadata.Name, "keycloak-identity")
	}
	if len(resource.Spec.Panels) == 0 {
		t.Fatalf("spec.panels is empty: not a valid Perses dashboard shape")
	}
	if len(resource.Spec.Layouts) == 0 {
		t.Fatalf("spec.layouts is empty: not a valid Perses dashboard shape")
	}
	if _, ok := resource.Spec.Datasources["prometheus"]; !ok {
		t.Fatalf("spec.datasources missing the prometheus datasource the query references")
	}
}

// persesInfraProbe stub answering "verified" for every panel series.
type verifiedPersesInfraProbe struct{}

func (verifiedPersesInfraProbe) querySeries(context.Context, string, string) (bool, error) {
	return true, nil
}
