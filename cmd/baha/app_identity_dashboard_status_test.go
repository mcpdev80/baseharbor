package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	metricsprovider "github.com/mcpdev80/baseharbor/internal/metrics"
)

// identityDashboardStatusReport mirrors the public `baha app status --json`
// identity-keycloak dashboard face: identity_dashboard_eligible is derived from
// the SAME verified Perses three-state surface the status probe resolved
// (collectApplicationStatusResult → identityDashboardBackendEligible →
// IdentityDashboardWitnessAt). The verdict is derived ONLY from real, target-
// bound evidence, never from a caller-supplied boolean (0428-078, CODEX-REVIEW
// content correction: per-Target binding must actually be asserted).
type identityDashboardStatusReport struct {
	State                     string  `json:"state"`
	Perses                    *string `json:"perses"`
	IdentityDashboardEligible bool    `json:"identity_dashboard_eligible"`
}

// runIdentityDashboardStatusCommand executes `baha app status --json` through
// the production CLI entry and decodes the public report.
func runIdentityDashboardStatusCommand(t *testing.T, ctx context.Context) identityDashboardStatusReport {
	t.Helper()
	var out, errOut bytes.Buffer
	if err := runWithIOAsTestOperator(ctx, []string{"app", "status", "--json"}, &out, &errOut); err != nil &&
		!strings.Contains(err.Error(), "container runtime orchestration not found") {
		t.Fatalf("baha app status --json failed: %v\nstdout: %s\nstderr: %s", err, out.String(), errOut.String())
	}
	var report identityDashboardStatusReport
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("baha app status --json output is not the public report: %v\n%s", err, out.String())
	}
	return report
}

// identityDashboardStatusProbe authors an isolated repository whose
// observability surface optionally selects Perses (pinned, verified release
// string) and resolves through the production status command so the identity
// dashboard face is computed end to end.
func identityDashboardStatusProbe(t *testing.T, ctx context.Context, perses bool) identityDashboardStatusReport {
	t.Helper()
	configureTestTarget(t)
	root := t.TempDir()
	m := application.New("perses-identity", "dev", false, false, false)
	m = application.WithWorkloadComponents(m, "api")
	m = application.WithMetricsSource(m, "application", "api", 8080, "/metrics")
	m.Services.ObservabilityManagementUI = true
	m.Services.Perses = perses
	const probeCompose = "services:\n  api:\n    image: alpine:3.22\n    command: [sleep, infinity]\n"
	if err := os.WriteFile(filepath.Join(root, "compose.yaml"), []byte(probeCompose), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeInitializedAuthoredManifest(t, filepath.Join(root, application.RepositoryManifestName), m, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	if _, err := resolveApplication(ctx, application.DefaultStore(), nil, "status"); err != nil {
		t.Fatalf("resolve probe application: %v", err)
	}
	return runIdentityDashboardStatusCommand(t, ctx)
}

// TestIdentityDashboardEligibleNoDataFailsClosed is the no-data fixture for
// 0428-078: an observability surface that does NOT select Perses has no verified
// Keycloak surface at all, so the identity dashboard MUST be reported ineligible
// (identity_dashboard_eligible == false) — fail closed on the missing data, not
// absent from the report.
func TestIdentityDashboardEligibleNoDataFailsClosed(t *testing.T) {
	ctx := t.Context()
	report := identityDashboardStatusProbe(t, ctx, false)
	if report.IdentityDashboardEligible {
		t.Fatalf("observability unselected (no-data) but the identity dashboard reports eligible: %+v", report)
	}
	if report.Perses != nil && *report.Perses == "verified" {
		t.Fatalf("no-data must never report a verified Perses surface: %+v", report)
	}
}

// TestIdentityDashboardEligiblePinnedUnverifiedFailsClosed is the real-queried
// fixture for 0428-078: observability IS selected and Perses IS pinned, but the
// surface is NOT verified (no real Keycloak health proof), so the identity
// dashboard MUST stay ineligible — verified-only, fail closed on the
// configured_unverified surface. This is a real query through the production CLI,
// not a bare false assertion.
func TestIdentityDashboardEligiblePinnedUnverifiedFailsClosed(t *testing.T) {
	ctx := t.Context()
	report := identityDashboardStatusProbe(t, ctx, true)
	if report.IdentityDashboardEligible {
		t.Fatalf("Perses pinned but unverified, yet the identity dashboard reports eligible: %+v", report)
	}
	// The real query must have resolved a machine-readable Perses state; it is
	// configured_unverified, never verified, without a real health proof.
	if report.Perses == nil {
		t.Fatalf("the identity dashboard real-queried face must still expose the machine-readable perses state: %+v", report)
	}
	if *report.Perses == "verified" {
		t.Fatalf("a pinned-but-unverified surface must not be reported verified: %+v", report)
	}
}

// TestIdentityDashboardCatalogShipsKeycloakIdentity is the dashboard-definition
// fixture for 0428-078: the shipped Perses panel catalog MUST carry the
// identity-keycloak dashboard (the content correction's finding 1), surfaced via
// the production availability path. Absent catalog entry means the dashboard
// definition is missing.
func TestIdentityDashboardCatalogShipsKeycloakIdentity(t *testing.T) {
	m := application.New("perses-identity", "dev", false, false, false)
	m = application.WithWorkloadComponents(m, "api")
	m.Services.ObservabilityManagementUI = true
	m.Services.Perses = true
	avail := metricsprovider.PersesDashboardAvailability(m)
	if _, ok := avail["keycloak-identity"]; !ok {
		t.Fatalf("the shipped Perses catalog must ship the identity-keycloak dashboard via the production availability path, missing keycloak-identity")
	}
}
