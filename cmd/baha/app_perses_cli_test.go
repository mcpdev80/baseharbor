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
	"github.com/mcpdev80/baseharbor/internal/deployment"
)

// The r3 contract (0428-075, akte act-1575-review-r2.md): the Perses seam is
// proven by EXECUTING the genuine baha command path — one command path like a
// user, with the status output parsed (precedent 0427-175 r8). Every cell
// below runs runWithIO (production argument parsing, context plumbing, command
// dispatch, effectiveTarget, resolver, render) over an isolated temporary root;
// none of them calls an internal seam helper in place of the command. No
// container is started and no Perses service is probed: this PR is the
// configuration-only seam, and reachability stays unverified.

const persesProbeCompose = "services:\n  api:\n    image: alpine:3.22\n    command: [sleep, infinity]\n"

// persesManagementSurface is one entry of the public management_ui surface list.
type persesManagementSurface struct {
	Service             string `json:"service"`
	Purpose             string `json:"purpose"`
	URL                 string `json:"url"`
	Authentication      string `json:"authentication"`
	AuthenticationClass string `json:"authentication_class"`
}

// persesDashboardAvail mirrors one entry of the public database_dashboards face
// of `baha status --json`: the availability verdict per dashboard key.
type persesDashboardAvail struct {
	Key        string `json:"key"`
	Backend    string `json:"backend"`
	Available  string `json:"available"`
	Datasource string `json:"datasource,omitempty"`
	Reason     string `json:"reason,omitempty"`
}

// the management_ui surface list and the readiness checks the CLI renders.
type persesStatusReport struct {
	State              string                          `json:"state"`
	Ready              bool                            `json:"ready"`
	Perses             *string                         `json:"perses"`
	ManagementUI       []persesManagementSurface       `json:"management_ui"`
	DatabaseDashboards map[string]persesDashboardAvail `json:"database_dashboards"`
	Checks             []struct {
		Name   string `json:"name"`
		OK     bool   `json:"ok"`
		State  string `json:"state"`
		Detail string `json:"detail"`
	} `json:"checks"`
}

// runPersesStatusCommand executes `baha status --json` through the production
// CLI entry (runWithIO) in the CURRENT working directory and decodes the
// public report. A command failure fails the cell: the seam never turns a
// declared configuration into an error state.
func runPersesStatusCommand(t *testing.T, ctx context.Context) persesStatusReport {
	t.Helper()
	var out, errOut bytes.Buffer
	err := runWithIOAsTestOperator(ctx, []string{"status", "--json"}, &out, &errOut)
	if err != nil && !strings.Contains(err.Error(), "container runtime orchestration not found") {
		t.Fatalf("baha status --json failed: %v\nstdout: %s\nstderr: %s", err, out.String(), errOut.String())
	}
	var report persesStatusReport
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("baha status --json output is not the public report: %v\n%s", err, out.String())
	}
	return report
}

// observabilitySurface returns the observability management surface the status
// report declares, or nil when the CLI declares none.
func observabilitySurface(report persesStatusReport) *persesManagementSurface {
	for i := range report.ManagementUI {
		if report.ManagementUI[i].Service == "observability" {
			return &report.ManagementUI[i]
		}
	}
	return nil
}

// assertPersesSurfaceUnverified pins the split axes on the parsed CLI output:
// reachability is unverified (the seam ships no health probe, so a surface
// entry with an unauthenticated/unsupported access class can never claim
// authorized access) and no check reports the observability management UI as
// READY.
func assertPersesSurfaceUnverified(t *testing.T, report persesStatusReport) {
	t.Helper()
	if surface := observabilitySurface(report); surface != nil {
		if surface.Authentication != "" {
			t.Fatalf("Perses surface reports authentication %q: reachability must stay unverified without a real health path", surface.Authentication)
		}
		if surface.AuthenticationClass != string(application.ManagementAuthUnsupported) {
			t.Fatalf("Perses surface auth class = %q, want %q (the seam never claims authorized access)", surface.AuthenticationClass, application.ManagementAuthUnsupported)
		}
	}
	for _, check := range report.Checks {
		if !strings.HasPrefix(check.Name, "management-ui") {
			continue
		}
		if check.State == "ready" || check.State == "verified" {
			t.Fatalf("check %s reports %q: the configuration-only seam must never verify management UI reachability", check.Name, check.State)
		}
	}
}

// assertPersesConfiguredUnverified pins the machine-readable three-state contract
// on the parsed JSON: for a selected, validly pinned Perses surface WITHOUT a
// real health proof the CLI MUST report EXACTLY "configured_unverified"
// (is-equality — pin validation alone must never map into "verified"), and the
// public surface still carries no ready/verified check. This is a positive
// claim, not a "does-not-contain-verified" vacuous reject.
func assertPersesConfiguredUnverified(t *testing.T, report persesStatusReport) {
	t.Helper()
	if report.Perses == nil {
		t.Fatalf("the status JSON must expose a machine-readable perses field with exactly one of {not_configured, configured_unverified, verified}")
	}
	if *report.Perses != string(capability.PersesStatusConfiguredUnverified) {
		t.Fatalf("selected+valid Perses without a health probe must report %q, got %q (pin validation alone must never become verified)",
			capability.PersesStatusConfiguredUnverified, *report.Perses)
	}
	if *report.Perses == string(capability.PersesStatusVerified) {
		t.Fatalf("a selected Perses surface with no real health probe must never report %q", capability.PersesStatusVerified)
	}
	assertPersesSurfaceUnverified(t, report)
}

// persesProbeAppReport authors an initialized repository and returns its parsed
// status report through the production CLI entry (see persesProbeApp for the
// applied-state fixture; this form keeps the not-applied face, which is the
// fail-closed not_configured default).
func persesProbeAppReport(t *testing.T, ctx context.Context, perses bool) persesStatusReport {
	t.Helper()
	root := t.TempDir()
	m := application.New("perses-cli", "dev", false, false, false)
	m = application.WithWorkloadComponents(m, "api")
	m = application.WithMetricsSource(m, "application", "api", 8080, "/metrics")
	m.Services.ObservabilityManagementUI = true
	m.Services.Perses = perses
	if err := os.WriteFile(filepath.Join(root, "compose.yaml"), []byte(persesProbeCompose), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeInitializedAuthoredManifest(t, filepath.Join(root, application.RepositoryManifestName), m, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	_, err := resolveApplication(ctx, application.DefaultStore(), nil, "status")
	if err != nil {
		t.Fatalf("resolve probe application: %v", err)
	}
	return runPersesStatusCommand(t, ctx)
}

// persesProbeApp authors an initialized repository whose observability
// management UI optionally realizes the Perses seam, materializes applied
// runtime state and the durable deployment record through the production
// writer, then executes the production status command and returns its parsed
// report.
func persesProbeApp(t *testing.T, ctx context.Context, perses bool) (resolvedApplication, persesStatusReport) {
	t.Helper()
	root := t.TempDir()
	m := application.New("perses-cli", "dev", false, false, false)
	m = application.WithWorkloadComponents(m, "api")
	m = application.WithMetricsSource(m, "application", "api", 8080, "/metrics")
	m.Services.ObservabilityManagementUI = true
	m.Services.Perses = perses
	if err := os.WriteFile(filepath.Join(root, "compose.yaml"), []byte(persesProbeCompose), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeInitializedAuthoredManifest(t, filepath.Join(root, application.RepositoryManifestName), m, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	resolved, err := resolveApplication(ctx, application.DefaultStore(), nil, "status")
	if err != nil {
		t.Fatalf("resolve probe application: %v", err)
	}
	if resolved.Manifest.Services.Perses != perses {
		t.Fatalf("the authored manifest did not round-trip services.perses=%v", perses)
	}
	// A declared surface is only visible once the deployment was applied; the
	// not-applied face is the same seam's fail-closed default. Materialize
	// applied state and the record through the production writer.
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
	return resolved, runPersesStatusCommand(t, ctx)
}

// TestBahaStatusPersesSurfaceFailsClosedAndUnverified drives the genuine
// `baha status` command over a temporary root for both manifest selections.
// Selected Perses surfaces the observability management entry with the
// unsupported auth class and unverified reachability; an unselected Perses
// declares no observability surface at all (fail-closed default). The contract
// is parsed from the CLI's own JSON output, not from helper return values.
func TestBahaStatusPersesSurfaceFailsClosedAndUnverified(t *testing.T) {
	t.Setenv("BASEHARBOR_TARGET", "")
	t.Setenv("BASEHARBOR_STATE_DIR", "")
	ctx := t.Context()
	for _, tc := range []struct {
		name   string
		perses bool
	}{
		{name: "selected", perses: true},
		{name: "unselected-fail-closed", perses: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			configureTestTarget(t)
			_, report := persesProbeApp(t, ctx, tc.perses)
			surface := observabilitySurface(report)
			if tc.perses {
				if surface == nil {
					t.Fatalf("services.perses selected but `baha status` declares no observability surface: %+v", report.ManagementUI)
				}
				if surface.Purpose != string(application.ProviderInterfaceObservability) {
					t.Fatalf("surface purpose = %q, want %q", surface.Purpose, application.ProviderInterfaceObservability)
				}
			} else if surface != nil {
				t.Fatalf("services.perses unselected but `baha status` declares a surface: %+v", *surface)
			}
			assertPersesSurfaceUnverified(t, report)
		})
	}
}

// TestBahaStatusPersesSurfaceReachableOnlyWithObservabilityManagementUI drives
// the gate through the CLI: a repository without observability.management_ui
// declares no observability surface at all — not even in its not-applied face
// — while the pinned release itself stays valid. Fail-closed default by
// command output.
func TestBahaStatusPersesSurfaceReachableOnlyWithObservabilityManagementUI(t *testing.T) {
	t.Setenv("BASEHARBOR_TARGET", "")
	t.Setenv("BASEHARBOR_STATE_DIR", "")
	ctx := t.Context()
	configureTestTarget(t)
	root := t.TempDir()
	m := application.New("perses-gate", "dev", false, false, false)
	m = application.WithWorkloadComponents(m, "api")
	if err := os.WriteFile(filepath.Join(root, "compose.yaml"), []byte(persesProbeCompose), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeInitializedAuthoredManifest(t, filepath.Join(root, application.RepositoryManifestName), m, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	report := runPersesStatusCommand(t, ctx)
	if surface := observabilitySurface(report); surface != nil {
		t.Fatalf("no observability management_ui was selected but `baha status` declares %+v", *surface)
	}
	assertPersesSurfaceUnverified(t, report)
	if report.State != "not_applied" {
		t.Fatalf("the gate cell must exercise the not-applied face, got %q", report.State)
	}
	if err := capability.ValidatePersesRelease(capability.PersesDefaultRelease); err != nil {
		t.Fatalf("the pinned release must stay valid while the surface remains unselected: %v", err)
	}
}

// TestBahaStatusPersesCLIEntrypointResolvesWithoutContainer is the genuine
// execution of the entry point the earlier candidate only pretended to run: it
// drives BOTH public status entry forms (the repository-aware root `baha
// status` and `baha app status`) end to end over a temporary root and asserts
// the command result, not a path helper. The root form runs its full
// collection — runtime detection and readiness checks included; the seam
// itself resolves and declares without any container.
func TestBahaStatusPersesCLIEntrypointResolvesWithoutContainer(t *testing.T) {
	t.Setenv("BASEHARBOR_TARGET", "")
	t.Setenv("BASEHARBOR_STATE_DIR", "")
	ctx := t.Context()
	configureTestTarget(t)
	_, report := persesProbeApp(t, ctx, true)
	if report.State == "" {
		t.Fatal("the executed status command produced a report without a state")
	}
	surface := observabilitySurface(report)
	if surface == nil {
		t.Fatalf("`baha status` resolved no Perses surface without a container: %+v", report.ManagementUI)
	}
	assertPersesSurfaceUnverified(t, report)
	// The explicit app command runs the same production dispatch and must
	// declare the identical surface entry.
	var appOut, appErr bytes.Buffer
	if err := runWithIOAsTestOperator(ctx, []string{"app", "status", "--json"}, &appOut, &appErr); err != nil {
		t.Fatalf("baha app status --json failed: %v\nstdout: %s\nstderr: %s", err, appOut.String(), appErr.String())
	}
	var appReport persesStatusReport
	if err := json.Unmarshal(appOut.Bytes(), &appReport); err != nil {
		t.Fatalf("baha app status --json output is not the public report: %v\n%s", err, appOut.String())
	}
	appSurface := observabilitySurface(appReport)
	if appSurface == nil || *appSurface != *surface {
		t.Fatalf("the two public status entries disagree about the seam: app=%v root=%v", appSurface, surface)
	}
	// Positive is-equality: the explicit `app status --json` field must itself be
	// configured_unverified for the selected + no-probe case — direct coverage of
	// the app-status JSON field, not only via the root status surface. "verified"
	// is a reserved per-target probe result and never a static true
	// (0428-186, CODEX-REVIEW fail-closed correction).
	assertPersesConfiguredUnverified(t, appReport)
}

// TestBahaStatusPersesUnknownOwnerFailsClosedThroughCLI is the required
// unknown-owner case: a protected deploy directory whose authoritative record
// AND retained ownership band are gone has an unknown owner, and the genuine
// status command must fail closed — no report, no readiness, no surface claim.
// A damaged directory for an explicitly selected deployment id reports the
// damaged-state conflict naming the unknown identity; the name-resolved path
// likewise refuses to invent an owner.
func TestBahaStatusPersesUnknownOwnerFailsClosedThroughCLI(t *testing.T) {
	t.Setenv("BASEHARBOR_TARGET", "")
	t.Setenv("BASEHARBOR_STATE_DIR", "")
	ctx := t.Context()
	target := configureTestTarget(t)
	resolved, _ := persesProbeApp(t, ctx, true)
	depRoot, err := deployment.DeploymentRoot(resolved.DeploymentIdentity)
	if err != nil {
		t.Fatal(err)
	}
	// Retain protected runtime state, then truncate BOTH durable identity
	// sources: the deployment record and the retained ownership band. The
	// owner of this deploy directory is now unknown.
	if err := os.WriteFile(filepath.Join(depRoot, "application.env"), []byte("preserved=protected\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(depRoot, "deployment.json")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(depRoot, deployment.RetainedOwnershipBandName)); err != nil {
		t.Fatalf("the production writer must have left a band next to the record: %v", err)
	}

	// (1) In the repository, the status command fails closed with the explicit
	// damaged-state conflict naming an UNKNOWN owner (identity application
	// "unknown", blank id/environment): it never reports an attributed owner.
	var out, errOut bytes.Buffer
	statusErr := runWithIOAsTestOperator(ctx, []string{"app", "status", "--json"}, &out, &errOut)
	if statusErr == nil {
		t.Fatalf("a status command over an unknown-owner deployment must fail closed, got report: %s", out.String())
	}
	if out.Len() != 0 {
		t.Fatalf("the failed status command must not emit a report, got: %s", out.String())
	}

	if !strings.Contains(statusErr.Error(), "damaged deployment state") {
		t.Fatalf("the repository status command must surface the damaged-state conflict, got: %v\nstderr: %s", statusErr, errOut.String())
	}

	// (2) The uuid-addressed form addresses the orphaned directory directly and
	// must report the damaged-state conflict, never a resolved deployment.
	uuidErr := runWithIOAsTestOperator(ctx, []string{"app", "status", "--uuid", resolved.DeploymentIdentity.DeploymentID, "--json"}, &out, &errOut)
	if uuidErr == nil {
		t.Fatalf("the uuid-addressed status command must fail closed on an unknown owner, got: %s", out.String())
	}
	if !strings.Contains(uuidErr.Error(), "damaged deployment state") {
		t.Fatalf("unknown ownership must surface the damaged-state conflict, got: %v\nstderr: %s", uuidErr, errOut.String())
	}
	if !strings.Contains(uuidErr.Error(), "unknown") && !strings.Contains(uuidErr.Error(), "/ /") && !strings.Contains(uuidErr.Error(), "//") {
		t.Fatalf("the damaged-state conflict must disclose the unknown owner: %v", uuidErr)
	}

	// (3) The NAME-resolved read surface outside any repository refuses to
	// invent an owner for the band-less directory: it never reports the orphan
	// as an authorized deployment.
	t.Chdir(t.TempDir())
	var showOut, showErr bytes.Buffer
	nameErr := runWithIOAsTestOperator(ctx, []string{"app", "show", resolved.Manifest.Name}, &showOut, &showErr)
	if nameErr == nil {
		t.Fatalf("an unknown-owner orphan became an authorized listing: %s", showOut.String())
	}
	if strings.Contains(nameErr.Error(), resolved.DeploymentIdentity.DeploymentID) && !strings.Contains(nameErr.Error(), "unknown") {
		t.Fatalf("the unknown-owner conflict must not attribute a trusted identity to the orphan: %v", nameErr)
	}

	// (3) The registry truth behind the fail-closed verdict: the target-wide
	// display listing reports exactly one ownerless damaged entry, so the CLI
	// had no trusted identity to resolve.
	records, warnings, listErr := deployment.ListDeploymentsForDisplay(target.Name)
	if listErr != nil {
		t.Fatalf("display listing: %v", listErr)
	}
	if len(records) != 0 {
		t.Fatalf("an unknown-owner orphan must not stay a registered record, got %d records", len(records))
	}
	var damaged *deployment.DamagedDeploymentError
	for _, warning := range warnings {
		if candidate, ok := deployment.DamagedDeploymentState(warning, nil); ok {
			damaged = candidate
		}
	}
	if damaged == nil {
		t.Fatalf("the listing must disclose the unknown-owner damaged state, got warnings=%d", len(warnings))
	}
	if !damaged.Ownerless() {
		t.Fatalf("the orphan carries no trusted identity and must be reported ownerless: %+v", damaged.Identity)
	}
}

// TestBahaStatusPersesReportsUnsupportedAuthThroughCLI guards the
// authorization boundary through the parsed command output: with no Perses
// authentication contract in this Core, the surfaced entry reports the
// unsupported management-auth class and an empty access marker — never a false
// native-credential access — while reachability stays unverified.
func TestBahaStatusPersesReportsUnsupportedAuthThroughCLI(t *testing.T) {
	t.Setenv("BASEHARBOR_TARGET", "")
	t.Setenv("BASEHARBOR_STATE_DIR", "")
	ctx := t.Context()
	configureTestTarget(t)
	_, report := persesProbeApp(t, ctx, true)
	surface := observabilitySurface(report)
	if surface == nil {
		t.Fatal("the executed status command surfaced no observability entry")
	}
	if surface.AuthenticationClass != string(application.ManagementAuthUnsupported) {
		t.Fatalf("Perses auth class via CLI = %q, want %q (fail closed when authentication is not configured)",
			surface.AuthenticationClass, application.ManagementAuthUnsupported)
	}
	assertPersesSurfaceUnverified(t, report)
}

// TestBahaStatusPersesConfiguredUnverifiedPositive drives the genuine
// `baha status --json` for a selected, validly pinned Perses surface and asserts
// the machine-readable perses field is EXACTLY "configured_unverified"
// (is-equality), while the public surface is still not ready. "verified" is a
// real per-target health-probe result and Core ships no such probe, so a
// selected, pinned surface with a silent/not-probed target must NOT report
// "verified" (CODEX-REVIEW 0428-186 correction: verified is no longer derived
// from the static reference-catalog scan). The fail-closed variant
// (PersesConfiguredStatusNow) still ANDs operator authorization so no status
// path can report a configured surface that is not actually protected.
func TestBahaStatusPersesConfiguredUnverifiedPositive(t *testing.T) {
	t.Setenv("BASEHARBOR_TARGET", "")
	t.Setenv("BASEHARBOR_STATE_DIR", "")
	ctx := t.Context()
	configureTestTarget(t)
	_, report := persesProbeApp(t, ctx, true)
	assertPersesConfiguredUnverified(t, report)
}

// TestBahaStatusPersesNotConfiguredWhenUnselected drives the same JSON entry
// for an unselected Perses surface and asserts the machine-readable state is
// exactly "not_configured" (the fail-closed default), never configured or
// unverified.
func TestBahaStatusPersesNotConfiguredWhenUnselected(t *testing.T) {
	t.Setenv("BASEHARBOR_TARGET", "")
	t.Setenv("BASEHARBOR_STATE_DIR", "")
	ctx := t.Context()
	configureTestTarget(t)
	report := persesProbeAppReport(t, ctx, false)
	if report.Perses == nil {
		t.Fatalf("the status JSON must expose a machine-readable perses field even for the not-configured default")
	}
	if *report.Perses != string(capability.PersesStatusNotConfigured) {
		t.Fatalf("unselected Perses must report %q, got %q", capability.PersesStatusNotConfigured, *report.Perses)
	}
	if *report.Perses == string(capability.PersesStatusConfiguredUnverified) || *report.Perses == string(capability.PersesStatusVerified) {
		t.Fatalf("unselected Perses must never be reported as configured_unverified or verified, got %q", *report.Perses)
	}
}

// TestPersesReleaseFailClosedOnUnknownAuthClass guards the contract seam the
// executed CLI paths validate against: an unrecognizable authentication class
// fails closed in ValidatePersesRelease, so no command can surface a Perses
// "configured" state with a bogus auth class.
func TestPersesReleaseFailClosedOnUnknownAuthClass(t *testing.T) {
	err := capability.ValidatePersesRelease(capability.PersesRelease{
		Version:                 "v0.55.1",
		Image:                   "persesdev/perses:v0.55.1",
		PrometheusPluginVersion: "v0.6.0",
		AuthenticationClass:     "grafana-admin",
	})
	if err == nil {
		t.Fatal("unknown perses auth class must fail closed")
	}
}

// TestBahaStatusDatabaseDashboardsExposesUnavailableDatabases drives the genuine
// `baha status --json` for a manifest that selects Perses and asserts the
// database_dashboards face is deterministic and explicit: PostgreSQL and
// MongoDB are exposed as "unavailable" with a reason naming the missing signal,
// and are never silently omitted.
func TestBahaStatusDatabaseDashboardsExposesUnavailableDatabases(t *testing.T) {
	ctx := t.Context()
	report := persesProbeAppReport(t, ctx, true)

	if report.DatabaseDashboards == nil {
		t.Fatal("status JSON must expose the database_dashboards face")
	}
	for _, key := range []string{"postgresql-targets", "mongodb-targets"} {
		avail, ok := report.DatabaseDashboards[key]
		if !ok {
			t.Fatalf("database_dashboards must expose %q explicitly", key)
		}
		if avail.Available != "unavailable" {
			t.Fatalf("database %q must report unavailable, got %q", key, avail.Available)
		}
		if avail.Reason == "" {
			t.Fatalf("database %q must carry a concrete reason", key)
		}
	}
	// Positive witness: the three real backends are exposed as available.
	for _, key := range []string{"prometheus-targets", "loki-logs", "tempo-traces"} {
		avail, ok := report.DatabaseDashboards[key]
		if !ok {
			t.Fatalf("database_dashboards must expose %q", key)
		}
		if avail.Available != "available" {
			t.Fatalf("database %q must report available, got %q", key, avail.Available)
		}
	}
}

// TestBahaStatusDatabaseDashboardsUnverifiedTargetFailsClosed drives the
// genuine `baha status --json` for a manifest that does NOT select the
// observability management UI and asserts the target-resolved availability gate
// fails closed: every backend (including the three real ones) is reported
// "unavailable" with a concrete reason naming the missing verified observability
// signal, never an inferred "available". This is the target-verification
// negative path the review required (review-2 Finding 1): the verdict is derived
// from the RESOLVED manifest signal, never a caller-supplied boolean and never
// an empty-witness inference (0428-142 class).
func TestBahaStatusDatabaseDashboardsUnverifiedTargetFailsClosed(t *testing.T) {
	ctx := t.Context()
	root := t.TempDir()
	m := application.New("perses-cli", "dev", false, false, false)
	m = application.WithWorkloadComponents(m, "api")
	m = application.WithMetricsSource(m, "application", "api", 8080, "/metrics")
	// Observability management UI is NOT selected: the resolved manifest has no
	// verified observability signal.
	m.Services.ObservabilityManagementUI = false
	m.Services.Perses = false
	if err := os.WriteFile(filepath.Join(root, "compose.yaml"), []byte(persesProbeCompose), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeInitializedAuthoredManifest(t, filepath.Join(root, application.RepositoryManifestName), m, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	_, err := resolveApplication(ctx, application.DefaultStore(), nil, "status")
	if err != nil {
		t.Fatalf("resolve probe application: %v", err)
	}
	report := runPersesStatusCommand(t, ctx)
	if report.DatabaseDashboards == nil {
		t.Fatal("status JSON must expose the database_dashboards face")
	}
	for _, key := range []string{"prometheus-targets", "loki-logs", "tempo-traces", "postgresql-targets", "mongodb-targets"} {
		avail, ok := report.DatabaseDashboards[key]
		if !ok {
			t.Fatalf("database_dashboards must expose %q explicitly", key)
		}
		if avail.Available != "unavailable" {
			t.Fatalf("database %q must report unavailable when observability is unverified, got %q", key, avail.Available)
		}
		if avail.Reason == "" {
			t.Fatalf("database %q must carry a concrete reason when observability is unverified", key)
		}
	}
}
