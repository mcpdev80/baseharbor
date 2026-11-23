package main

import (
	"bytes"
	"encoding/json"
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
		resolved: resolvedApplication{Target: resolvedApplication{}.Target, Manifest: m},
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

// TestPersesAlertEvaluatorObservationCommandFaceKeepsManagementUIUnverified
// drives the real `baha app status` JSON face on an applied-but-stopped probe
// and asserts the surface observation never reports ready/verified, matching
// the surface contract the #1702 witness tests already pin.
func TestPersesAlertEvaluatorObservationCommandFaceKeepsManagementUIUnverified(t *testing.T) {
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
	// verified. The alert-evaluator consumer reports the verifier result on
	// `baha app status`; neither face may claim a health the Core cannot
	// observe.
	if result.State != string(capability.PersesStatusNotConfigured) && result.State != string(capability.PersesStatusConfiguredUnverified) {
		t.Fatalf("the perses status command reported %q: the three-state contract allows only not_configured or configured_unverified without a real health path", result.State)
	}
	if result.State == string(capability.PersesStatusVerified) {
		t.Fatalf("the perses status command reported %q without a real health path", result.State)
	}
}
