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
	"github.com/mcpdev80/baseharbor/internal/capability"
)

// The 0428-082 infrastructure-dashboard availability face: `baha status --json`
// carries infrastructure_dashboards, computed by the production status path
// (collectApplicationStatusResult → metricsprovider.PersesInfraDashboardAvailability
// → VerifyPersesInfraDashboard over the materialized Prometheus endpoint). The
// verdicts are derived ONLY from the dashboard's own real per-panel
// /api/v1/query answers (or the fail-closed error naming why nothing was
// observed) — never from a caller-supplied status (owner decision 2026-11-16).
// Precedent face tests: app_perses_host_containers_cli_test.go (077), the 076
// database_dashboards face.

type persesInfraDashboardAvail struct {
	Key        string `json:"key"`
	Backend    string `json:"backend"`
	Available  string `json:"available"`
	Datasource string `json:"datasource,omitempty"`
	Query      string `json:"query,omitempty"`
	Reason     string `json:"reason,omitempty"`
}

type persesInfraStatusReport struct {
	State                    string                               `json:"state"`
	Perses                   *string                              `json:"perses"`
	InfrastructureDashboards map[string]persesInfraDashboardAvail `json:"infrastructure_dashboards"`
}

func runPersesInfraStatusCommand(t *testing.T, ctx context.Context) persesInfraStatusReport {
	t.Helper()
	var out, errOut bytes.Buffer
	err := runWithIO(ctx, []string{"status", "--json"}, &out, &errOut)
	if err != nil && !strings.Contains(err.Error(), "container runtime orchestration not found") {
		t.Fatalf("baha status --json failed: %v\nstdout: %s\nstderr: %s", err, out.String(), errOut.String())
	}
	var report persesInfraStatusReport
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("baha status --json output is not the public report: %v\n%s", err, out.String())
	}
	return report
}

// persesInfraApp authors an initialized repository whose observability surface
// optionally selects Perses (with a real metrics source on the workload), so
// the production status command computes the infrastructure-dashboard face.
// applied=false keeps the not-applied face.
func persesInfraAppReport(t *testing.T, ctx context.Context, perses, observabilityUI bool) persesInfraStatusReport {
	t.Helper()
	// Probe resolution reads the ambient Target selection, so neutralise any
	// BASEHARBOR_TARGET/ENV activation or host-persisted organization
	// selection leaked into this package process before the probe resolves
	// its application.
	testIsolatedSelectionContext(t)
	root := t.TempDir()
	m := application.New("perses-infra-cli", "dev", false, false, false)
	m = application.WithWorkloadComponents(m, "api")
	m = application.WithMetricsSource(m, "application", "api", 8080, "/metrics")
	m.Services.ObservabilityManagementUI = observabilityUI
	m.Services.Perses = perses && observabilityUI
	if err := os.WriteFile(filepath.Join(root, "compose.yaml"), []byte(persesProbeCompose), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeInitializedAuthoredManifest(t, filepath.Join(root, application.RepositoryManifestName), m, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	if _, err := resolveApplication(ctx, application.DefaultStore(), nil, "status"); err != nil {
		t.Fatalf("resolve probe application: %v", err)
	}
	return runPersesInfraStatusCommand(t, ctx)
}

// TestBahaStatusInfrastructureDashboardsExposesEveryPanel proves the face is
// the complete, explicit per-panel inventory: all four infra panels present,
// each with a verdict, never a silent omission. Without a materialized
// Prometheus provider the entries are all UNAVAILABLE with a concrete reason
// (fail-closed), so no positive availability is fabricated from the manifest
// declaration alone.
func TestBahaStatusInfrastructureDashboardsExposesEveryPanel(t *testing.T) {
	ctx := context.Background()
	report := persesInfraAppReport(t, ctx, true, true)
	if len(report.InfrastructureDashboards) != 4 {
		t.Fatalf("infrastructure_dashboards must expose all four infra panels, got %d: %+v", len(report.InfrastructureDashboards), report.InfrastructureDashboards)
	}
	for _, key := range []string{"cpu", "memory", "disk", "network"} {
		entry, ok := report.InfrastructureDashboards[key]
		if !ok {
			t.Fatalf("infrastructure_dashboards must expose %q explicitly", key)
		}
		if entry.Key != key || entry.Backend != string(capability.ProviderPrometheus) {
			t.Fatalf("entry %q identity = %+v, want key=%q backend=%q", key, entry, key, string(capability.ProviderPrometheus))
		}
		switch entry.Available {
		case "available", "unavailable":
		default:
			t.Fatalf("entry %q availability %q is not the two-verdict contract", key, entry.Available)
		}
	}
	// No Prometheus provider was ever materialized for this isolated root, so
	// the face must be fail-closed unavailable everywhere, with the concrete
	// reason — never a silent omission and never a positive derived from the
	// manifest declaration.
	for key, entry := range report.InfrastructureDashboards {
		if entry.Available != "unavailable" {
			t.Fatalf("without a materialized Prometheus backend panel %q must be unavailable, got %+v", key, entry)
		}
		if strings.TrimSpace(entry.Reason) == "" {
			t.Fatalf("unavailable panel %q must name the concrete reason", key)
		}
	}
}

// TestBahaStatusInfrastructureDashboardsNotSelectedOmitsFace proves the face
// is tied to the observability-surface selection that realizes Perses (the
// manifest gate `ObservabilityManagementUI && Perses`, exactly like the
// three-state Perses field): an application whose observability management UI
// is not selected declares no infrastructure dashboards at all — no dashboard
// surface for an unselected surface, never a fabricated availability.
func TestBahaStatusInfrastructureDashboardsNotSelectedOmitsFace(t *testing.T) {
	ctx := context.Background()
	report := persesInfraAppReport(t, ctx, false, false)
	if len(report.InfrastructureDashboards) != 0 {
		t.Fatalf("without the Perses selection the status face must expose no infrastructure dashboards, got %+v", report.InfrastructureDashboards)
	}
}
