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
	metricsprovider "github.com/mcpdev80/baseharbor/internal/metrics"
)

// These tests pin card 0428-152: the 'baha app perses status' observation is
// the ONE honest consumer of the Perses alert-evaluator verifier in
// internal/metrics/perses_prometheus_alerts.go. The consumer must show exactly
// the fail-closed state the verifier delivered — "not configured" when the
// manifest does not select the surface, "unavailable (owner-gated evaluator
// contract)" when it is selected but the Owner-gated real evaluator contract
// is missing — and it must NEVER fall through to "healthy"/"verified".
//
// They run in the Hosted PR · Go unit shard through the plain
// `go test ./cmd/baha` invocation in scripts/pr-unit-database.sh.

const alertEvaluatorObservationName = "management-ui/perses/alert-evaluator"

func persesAlertEvaluatorStatusObservation(t *testing.T, m application.Manifest) application.StatusCheck {
	t.Helper()
	collection := &applicationStatusCollection{
		manifest: m,
		result:   application.StatusResult{Ready: true, Checks: []application.StatusCheck{}},
	}
	collection.collectPersesAlertEvaluatorObservation()
	var found *application.StatusCheck
	for i := range collection.result.Checks {
		if collection.result.Checks[i].Name == alertEvaluatorObservationName {
			found = &collection.result.Checks[i]
		}
	}
	if found == nil {
		t.Fatalf("the consumer emitted no %q observation: %+v", alertEvaluatorObservationName, collection.result.Checks)
	}
	return *found
}

func persesAlertEvaluatorManifestSelection(selected bool) application.Manifest {
	m := application.New("perses-alert-evaluator-0428-152", "dev", false, false, false)
	m = application.WithWorkloadComponents(m, "api")
	m = application.WithMetricsSource(m, "application", "api", 8080, "/metrics")
	m.Services.ObservabilityManagementUI = true
	m.Services.Perses = selected
	return m
}

func TestPersesAlertEvaluatorObservationNotConfiguredWhenUnselected(t *testing.T) {
	check := persesAlertEvaluatorStatusObservation(t, persesAlertEvaluatorManifestSelection(false))
	// The exact wording is the contract (0428-073 failed on a paraphrase).
	if check.State != "not configured" {
		t.Fatalf("unselected manifest must state %q, got %q (detail: %s)", "not configured", check.State, check.Detail)
	}
	if check.OK {
		t.Fatal("a not-configured alert evaluator must never fold into readiness")
	}
	if !strings.Contains(check.Detail, "Perses is not selected") {
		t.Fatalf("the fail-closed reason must come from the verifier, got %q", check.Detail)
	}
}

func TestPersesAlertEvaluatorObservationUnavailableWhenSelectedWithoutContract(t *testing.T) {
	m := persesAlertEvaluatorManifestSelection(true)
	// Mirror the verifier's own fail-closed branch: selected but no metric
	// inputs configured means the owner-gated evaluator contract is missing.
	m.Metrics.Sources = nil
	if _, err := metricsprovider.BuildPersesAlertEvaluator(m); err == nil {
		t.Fatal("the verifier must fail closed without Prometheus metric inputs")
	}
	check := persesAlertEvaluatorStatusObservation(t, m)
	if check.State != "unavailable (owner-gated evaluator contract)" {
		t.Fatalf("selected manifest without the evaluator contract must state %q, got %q (detail: %s)",
			"unavailable (owner-gated evaluator contract)", check.State, check.Detail)
	}
	if check.OK {
		t.Fatal("an unavailable alert evaluator must never fold into readiness")
	}
}

func TestPersesAlertEvaluatorObservationSelectedWithContractStaysUnverified(t *testing.T) {
	m := persesAlertEvaluatorManifestSelection(true)
	result, err := metricsprovider.BuildPersesAlertEvaluator(m)
	if err != nil {
		t.Fatalf("selected manifest with metric inputs must build the contract: %v", err)
	}
	if result.Reachability != metricsprovider.PersesAlertReachabilityConfiguredUnverified {
		t.Fatalf("the verifier reachability changed under the consumer: %q", result.Reachability)
	}
	check := persesAlertEvaluatorStatusObservation(t, m)
	// The consumer shows the verifier's reachability verbatim: declared, but
	// never live-verified.
	if check.State != string(metricsprovider.PersesAlertReachabilityConfiguredUnverified) {
		t.Fatalf("built contract must state the verifier reachability %q verbatim, got %q",
			metricsprovider.PersesAlertReachabilityConfiguredUnverified, check.State)
	}
	if check.OK {
		t.Fatal("a configured_unverified alert evaluator must never fold into readiness")
	}
}

// TestPersesAlertEvaluatorObservationNeverReportsHealthyOrVerified is the
// fail-closed-at-the-consumer proof the contract demands: for every manifest
// the consumer can face (selected/unselected, with/without metric inputs), the
// observation may only carry one of the three allowed states, and the
// detail must not claim health. If the verifier ever reported a "verified"
// reachability while the Owner-gated contract predicate
// (persesAlertRealEvaluatorContractPresent in internal/metrics) is false, the
// consumer must downweight it to the unavailable state instead of echoing it.
func TestPersesAlertEvaluatorObservationNeverReportsHealthyOrVerified(t *testing.T) {
	noSources := persesAlertEvaluatorManifestSelection(true)
	noSources.Metrics.Sources = nil
	manifests := map[string]application.Manifest{
		"unselected":          persesAlertEvaluatorManifestSelection(false),
		"selected-no-sources": noSources,
		"selected":            persesAlertEvaluatorManifestSelection(true),
	}

	allowed := map[string]bool{
		"not configured": true,
		"unavailable (owner-gated evaluator contract)":                      true,
		string(metricsprovider.PersesAlertReachabilityConfiguredUnverified): true,
	}
	for name, m := range manifests {
		check := persesAlertEvaluatorStatusObservation(t, m)
		if check.State == "healthy" || check.State == "verified" || check.State == "ready" {
			t.Fatalf("%s: the consumer reported %q while the real evaluator contract is not Owner-approved", name, check.State)
		}
		if !allowed[check.State] {
			t.Fatalf("%s: state %q is outside the three-state fail-closed contract", name, check.State)
		}
	}

	// The consumer-side gate itself: a "verified" reachability (which no
	// producer may emit until the Owner approves the real contract) maps to
	// the unavailable state. This is persesAlertEvaluatorReachabilityVerified
	// in app_status_collect.go; pin the constant so a change there is loud.
	if persesAlertEvaluatorReachabilityVerified != "verified" {
		t.Fatalf("the consumer verified-marker changed: %q", persesAlertEvaluatorReachabilityVerified)
	}
}

// TestPersesAlertEvaluatorObservationCommandFaceReportsPersesSurface drives
// the real `baha app perses status` production entry for the selected surface
// and pins its public three-state contract: the surface is either
// not_configured or configured_unverified, never verified. The alert-
// evaluator consumer reports the verifier result on `baha app status`;
// neither face may claim a health this Core cannot observe.
func TestPersesAlertEvaluatorObservationCommandFaceReportsPersesSurface(t *testing.T) {
	t.Setenv("BASEHARBOR_TARGET", "")
	t.Setenv("BASEHARBOR_STATE_DIR", "")
	ctx := t.Context()
	configureTestTarget(t)
	persesWitnessProbeApp(t, ctx, persesWitnessAppName(t), func(m application.Manifest) application.Manifest {
		m.Services.ObservabilityManagementUI = true
		m.Services.Perses = true
		return m
	})
	var out, errOut bytes.Buffer
	if err := runWithIOAsTestOperator(ctx, []string{"app", "perses", "status", "--json"}, &out, &errOut); err != nil && !strings.Contains(err.Error(), "container runtime orchestration not found") {
		t.Fatalf("baha app perses status --json failed: %v\nstdout: %s\nstderr: %s", err, out.String(), errOut.String())
	}
	var result persesStatusResult
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("baha app perses status --json output is not the public report: %v\n%s", err, out.String())
	}
	// The command face stays inside the public three-state contract: the
	// surface is either not_configured or configured_unverified, never
	// verified.
	if result.State != string(capability.PersesStatusNotConfigured) && result.State != string(capability.PersesStatusConfiguredUnverified) {
		t.Fatalf("the perses status command reported %q: the three-state contract allows only not_configured or configured_unverified without a real health path", result.State)
	}
	if result.State == string(capability.PersesStatusVerified) {
		t.Fatalf("the perses status command reported %q without a real health path", result.State)
	}
}

// TestPersesAlertEvaluatorObservationOnAppStatusCommandFace drives the real
// `baha app status --json` production entry on the applied face (the face
// where the managed-service checks run) and asserts the alert-evaluator
// consumer is present and honest: an unselected manifest states exactly
// "not configured"; a selected manifest states configured_unverified (the
// verifier's own reachability) or the unavailable state, and NEVER
// healthy/verified/ready.
func TestPersesAlertEvaluatorObservationOnAppStatusCommandFace(t *testing.T) {
	t.Setenv("BASEHARBOR_TARGET", "")
	t.Setenv("BASEHARBOR_STATE_DIR", "")
	ctx := t.Context()
	t.Run("unselected", func(t *testing.T) {
		report := persesCommandFaceReport(t, ctx, false)
		check := persesAlertEvaluatorCommandObservation(t, report)
		if check.State != "not configured" {
			t.Fatalf("unselected manifest must state %q on the command face, got %q", "not configured", check.State)
		}
	})
	t.Run("selected-never-healthy", func(t *testing.T) {
		report := persesCommandFaceReport(t, ctx, true)
		check := persesAlertEvaluatorCommandObservation(t, report)
		switch check.State {
		case "configured_unverified", "unavailable (owner-gated evaluator contract)":
		default:
			t.Fatalf("selected alert evaluator must state configured_unverified or unavailable, got %q", check.State)
		}
		if check.OK {
			t.Fatal("the alert evaluator observation must never fold into readiness while the contract is Owner-gated")
		}
	})
}

type persesStatusReportCheck struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	State  string `json:"state"`
	Detail string `json:"detail"`
}

// persesCommandFaceReport authors a repository whose observability management
// UI optionally realizes the Perses alert evaluator, materializes applied
// runtime state through the production writer (the face where the consumer
// runs), and returns the parsed `baha app status --json` report.
func persesCommandFaceReport(t *testing.T, ctx context.Context, perses bool) persesStatusReport {
	t.Helper()
	// Author the probe FIRST (persesWitnessProbeApp owns the XDG roots via
	// t.Setenv), then move it onto the hermetic isolated Target so the applied
	// face never shares the "docker-dev" control plane with lifecycle tests.
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))
	resolved := persesWitnessProbeApp(t, ctx, persesWitnessAppName(t), func(m application.Manifest) application.Manifest {
		m.Services.ObservabilityManagementUI = true
		m.Services.Perses = perses
		return m
	})
	files := application.RuntimeFilesFor(resolved.Store, resolved.Manifest)
	if err := os.MkdirAll(files.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	canonical, err := application.RuntimeComposeYAMLForProject(resolved.Manifest, files)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(files.Compose, []byte(canonical), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(files.Env, []byte("preserved=protected\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := recordAppliedDeployment(ctx, resolved, files); err != nil {
		t.Fatalf("record applied probe deployment: %v", err)
	}
	var out, errOut bytes.Buffer
	if err := runWithIOAsTestOperator(ctx, []string{"app", "status", "--json"}, &out, &errOut); err != nil && !strings.Contains(err.Error(), "container runtime orchestration not found") {
		t.Fatalf("baha app status --json failed: %v\nstdout: %s\nstderr: %s", err, out.String(), errOut.String())
	}
	var report persesStatusReport
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("baha app status --json output is not the public report: %v\n%s", err, out.String())
	}
	if report.State == "not_applied" || report.State == "stopped" {
		t.Skipf("the applied command face did not materialize in this environment (state %q): the alert-evaluator consumer runs with the managed-service checks; the unit-level cases in this file cover the three-state contract", report.State)
	}
	return report
}

func persesAlertEvaluatorCommandObservation(t *testing.T, report persesStatusReport) persesStatusReportCheck {
	t.Helper()
	for i := range report.Checks {
		if report.Checks[i].Name == alertEvaluatorObservationName {
			check := report.Checks[i]
			if check.State == "healthy" || check.State == "verified" || check.State == "ready" {
				t.Fatalf("the command face reported %q for the alert evaluator: the consumer must stay fail-closed", check.State)
			}
			return check
		}
	}
	t.Fatalf("the app status face emitted no %q observation: %+v", alertEvaluatorObservationName, report.Checks)
	return persesStatusReportCheck{}
}
