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
	"github.com/mcpdev80/baseharbor/internal/identityprovider"
	metricsprovider "github.com/mcpdev80/baseharbor/internal/metrics"
	"github.com/mcpdev80/baseharbor/internal/observability"
	"github.com/mcpdev80/baseharbor/internal/operatorauth"
)

// newIdentityGateManifest builds a manifest with the identity provider selected
// and a BaseHarbor-owned Keycloak placement (the default resolved placement),
// used by the verified-only gate predicate positive case.
func newIdentityGateManifest(t *testing.T) application.Manifest {
	t.Helper()
	m := application.Manifest{
		Name:          "perses-identity-gate",
		Environment:   "test",
		ApplicationID: "app-perses-identity-gate",
	}
	m.Services.Identity = true
	return m
}

// identityStatusJSON mirrors the two machine-readable fields the identity
// -dashboard gate publishes on the public `baha status --json` face: the
// authoritative Perses three-state value and the non-forgeable
// identity_dashboard_eligible boolean that the gate (app_status_result.go:166-172)
// derives from it.
type identityStatusJSON struct {
	Perses                    *string `json:"perses"`
	IdentityDashboardEligible bool    `json:"identity_dashboard_eligible"`
}

// seedIdentityMetricsWitness registers the real Keycloak native metrics signal
// for the configured test target, so a selected identity provider + selected
// Perses surface yields a non-forgeable witness and the gate may emit true.
func seedIdentityMetricsWitness(t *testing.T, m application.Manifest, targetName string) {
	t.Helper()
	placement, err := application.ResolveProviderPlacement(m, capability.ProviderKeycloak)
	if err != nil {
		t.Fatalf("resolve Keycloak placement: %v", err)
	}
	root, err := deployment.TargetStateRoot(targetName)
	if err != nil {
		t.Fatalf("resolve Target state root: %v", err)
	}
	id, err := identityprovider.KeycloakMetricsRegistrationIDAt(m, placement, root, targetName)
	if err != nil {
		t.Fatalf("resolve Keycloak registration ID: %v", err)
	}
	class := observability.SourcePlatformProvider
	if placement.Scope == capability.ScopeApplication {
		class = observability.SourceApplicationProvider
	}
	source := observability.SignalSource{
		ID: id, Kind: observability.SignalMetrics, Provider: capability.ProviderKeycloak,
		Class: class, Scope: placement.Scope, SharingBoundary: placement.SharingBoundary,
		OwnerApplication: m.Name, Network: "baseharbor-identity", Target: "keycloak:9000",
		Protocol: "openmetrics", Path: "/metrics", Mode: capability.ObservabilityNative,
		Verification: capability.ObservabilityVerifyBackend,
	}
	if err := source.Validate(); err != nil {
		t.Fatalf("seed Keycloak signal validate: %v", err)
	}
	if err := observability.UpdateSignal(source); err != nil {
		t.Fatalf("seed Keycloak signal: %v", err)
	}
}

// driveIdentityStatusJSON authors an initialized repository whose identity
// provider and (optionally) Perses surface are selected, materializes the
// applied deployment record, seeds the real Keycloak witness when requested,
// then executes the production `baha status --json` and decodes the two gated
// fields. This drives the REAL status production path, not a path helper.
func driveIdentityStatusJSON(t *testing.T, ctx context.Context, perses bool) identityStatusJSON {
	t.Helper()
	root := t.TempDir()
	m := application.New("perses-identity-gate", "dev", false, false, false)
	m.Services.Identity = true
	m.Services.ObservabilityManagementUI = true
	m.Services.Perses = perses
	m = application.WithWorkloadComponents(m, "api")
	m = application.WithMetricsSource(m, "application", "api", 8080, "/metrics")
	if err := os.WriteFile(filepath.Join(root, "compose.yaml"), []byte(persesProbeCompose), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeInitializedAuthoredManifest(t, filepath.Join(root, application.RepositoryManifestName), m, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	resolved, err := resolveApplication(ctx, application.DefaultStore(), nil, "status")
	if err != nil {
		t.Fatalf("resolve identity gate application: %v", err)
	}
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
		t.Fatalf("record applied identity gate deployment: %v", err)
	}

	var out, errOut bytes.Buffer
	if err := runWithIO(ctx, []string{"status", "--json"}, &out, &errOut); err != nil &&
		!strings.Contains(err.Error(), "container runtime orchestration not found") {
		t.Fatalf("baha status --json failed: %v\nstdout: %s\nstderr: %s", err, out.String(), errOut.String())
	}
	var decoded identityStatusJSON
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatalf("baha status --json is not the public JSON report: %v\n%s", err, out.String())
	}
	return decoded
}

func configureIdentityStatusOperatorAuth(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	issuer := startManagedOperatorOIDCServer(t)
	t.Setenv(operatorauth.EnvIssuer, issuer)
	t.Setenv(operatorauth.EnvClientID, "baseharbor-cli")
	t.Setenv(operatorauth.EnvScopes, "openid,profile")
	t.Setenv(operatorauth.EnvCallbackPort, "0")
	t.Setenv("BASEHARBOR_NO_BROWSER", "1")
	var out, errOut bytes.Buffer
	ctx := operatorauth.WithEnforcement(context.Background())
	codePath := os.Getenv("BASEHARBOR_OPERATOR_OIDC_AUTH_CODE_FILE")
	if err := operatorLoginCommand().Run(ctx, []string{"-e", "dev", "--no-browser", "--auth-code-file", codePath}, &out, &errOut); err != nil {
		t.Fatalf("authenticate test operator: %v\nstdout: %s\nstderr: %s", err, out.String(), errOut.String())
	}
}

// TestIdentityDashboardEligibleRequiresConfiguredVerifiedPerses is the gate
// regression the reviewer required. It drives the genuine local status command
// for BOTH manifest faces and asserts identity_dashboard_eligible is true ONLY
// when the gate condition (app_status_result.go:166-172) holds: Perses selected
// (configured_unverified/verified) AND a real Keycloak witness. An unselected
// Perses must never emit true.
func TestIdentityDashboardEligibleRequiresConfiguredVerifiedPerses(t *testing.T) {
	t.Setenv("BASEHARBOR_TARGET", "")
	t.Setenv("BASEHARBOR_STATE_DIR", "")
	ctx := t.Context()
	target := configureTestTarget(t)
	configureIdentityStatusOperatorAuth(t)
	m := application.New("perses-identity-gate", "dev", false, false, false)
	m.Services.Identity = true
	seedIdentityMetricsWitness(t, m, target.Name)

	// A selected Perses surface with a real Keycloak witness still reports
	// configured_unverified (no real Perses health probe exists locally), and the
	// identity dashboard must fail closed for that state: verified-only.
	t.Run("perses-selected-configured-unverified-fails-closed", func(t *testing.T) {
		decoded := driveIdentityStatusJSON(t, ctx, true)
		if *decoded.Perses != string(capability.PersesStatusConfiguredUnverified) {
			t.Fatalf("selected+no-health Perses must report configured_unverified, got %q", *decoded.Perses)
		}
		if decoded.IdentityDashboardEligible {
			t.Fatalf("identity_dashboard_eligible=true with Perses configured_unverified + real Keycloak witness: the gate must require verified, emit false\n%+v", decoded)
		}
	})

	t.Run("perses-unselected-fails-closed", func(t *testing.T) {
		decoded := driveIdentityStatusJSON(t, ctx, false)
		if *decoded.Perses != string(capability.PersesStatusNotConfigured) {
			t.Fatalf("unselected Perses must report not_configured, got %q", *decoded.Perses)
		}
		if decoded.IdentityDashboardEligible {
			t.Fatalf("identity_dashboard_eligible=true with Perses not_configured: the gate must fail closed\n%+v", decoded)
		}
	})

}

// TestIdentityDashboardVerifiedStatusResultSerializesPositive exercises the
// verified-only production predicate and the exact public JSON encoder. The
// current Perses status probe correctly cannot produce "verified" yet, so this
// pins the result boundary without making PersesStatusNow invent a health proof.
func TestIdentityDashboardVerifiedStatusResultSerializesPositive(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	m := newIdentityGateManifest(t)
	const targetName = "target-test"
	root, err := deployment.TargetStateRoot(targetName)
	if err != nil {
		t.Fatalf("resolve target state root: %v", err)
	}
	placement, err := application.ResolveProviderPlacement(m, capability.ProviderKeycloak)
	if err != nil {
		t.Fatalf("resolve Keycloak placement: %v", err)
	}
	seedIdentityMetricsWitness(t, m, targetName)
	witness := metricsprovider.IdentitySelectionWitnessAt(m, placement, root, targetName)
	if !witness.NonForgeableAvailable() {
		t.Fatalf("real target-bound Keycloak witness must be available: %+v", witness)
	}

	verified := capability.PersesStatusVerified
	eligible := identityDashboardBackendEligible(&verified, m, root, targetName)
	if !eligible {
		t.Fatal("verified Perses plus the real target-bound witness must pass the production predicate")
	}
	result := withIdentityDashboardStatus(applicationStatusResult{}, &verified, eligible)
	var encoded bytes.Buffer
	if err := writeJSON(&encoded, result); err != nil {
		t.Fatalf("encode public status JSON: %v", err)
	}
	var decoded identityStatusJSON
	if err := json.Unmarshal(encoded.Bytes(), &decoded); err != nil {
		t.Fatalf("decode public status JSON: %v", err)
	}
	if decoded.Perses == nil || *decoded.Perses != string(verified) || !decoded.IdentityDashboardEligible {
		t.Fatalf("verified status JSON must carry perses=verified and identity_dashboard_eligible=true: %s", encoded.String())
	}
	for _, status := range []capability.PersesStatus{capability.PersesStatusConfiguredUnverified, capability.PersesStatusNotConfigured} {
		if identityDashboardBackendEligible(&status, m, root, targetName) {
			t.Fatalf("status %q must remain ineligible", status)
		}
	}
}

// TestIdentityDashboardEligibleGatesOnKeycloakWitness proves the gate is not
// satisfied by Perses alone: without a real Keycloak native metrics signal the
// identity dashboard must stay ineligible even when Perses is selected. This is
// the direct corollary of the reviewer's stop criterion (the gate reads BOTH
// the Perses state AND the witness).
func TestIdentityDashboardEligibleGatesOnKeycloakWitness(t *testing.T) {
	t.Setenv("BASEHARBOR_TARGET", "")
	t.Setenv("BASEHARBOR_STATE_DIR", "")
	ctx := t.Context()
	configureTestTarget(t)
	configureIdentityStatusOperatorAuth(t)
	// Do NOT seed the witness: identity selected but no real Keycloak signal.
	decoded := driveIdentityStatusJSON(t, ctx, true)
	if *decoded.Perses != string(capability.PersesStatusConfiguredUnverified) {
		t.Fatalf("selected+no-health Perses must report configured_unverified, got %q", *decoded.Perses)
	}
	if decoded.IdentityDashboardEligible {
		t.Fatalf("identity_dashboard_eligible=true with Perses selected but NO real Keycloak witness: the gate must fail closed\n%+v", decoded)
	}
}

// remotePersesStatusFixture installs a Connector-backed deployment on the remote
// execution Target the way the production remote path does: the execution Target
// is the enrolled node and its authoritative state lives under that Target's
// root, so resolveApplication can resolve it without a live mTLS/Core session.
// It seeds a real Keycloak native metrics signal for the node Target when
// seeded, so a selected identity provider + selected Perses surface yields a
// non-forgeable witness and the gate may emit true.
func remotePersesStatusFixture(t *testing.T, seededWitness, perses bool) (context.Context, application.Store, string) {
	t.Helper()
	ctx, core, node := remoteApplicationCoreFixture(t)
	ctx = withCoreAuthority(ctx, core)
	stateRoot, err := deployment.TargetStateRoot(node.Name)
	if err != nil {
		t.Fatalf("remote status fixture target root: %v", err)
	}
	if err := os.MkdirAll(stateRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	m := application.New("perses-remote-gate", "dev", false, false, false)
	m.Services.Identity = true
	m.Services.ObservabilityManagementUI = true
	m.Services.Perses = perses
	m = application.WithWorkloadComponents(m, "api")
	m = application.WithMetricsSource(m, "application", "api", 8080, "/metrics")
	store := application.Store{Root: stateRoot}
	if _, err := store.Create(m); err != nil {
		t.Fatal(err)
	}
	reloaded, manifestPath, err := store.Load(m.Name)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := deployment.NewDeploymentIdentity(node.Name, reloaded.ApplicationID, reloaded.Name, reloaded.Environment)
	if err != nil {
		t.Fatal(err)
	}
	intent, err := json.Marshal(reloaded)
	if err != nil {
		t.Fatal(err)
	}
	if err := deployment.SaveDeploymentRecord(deployment.DeploymentRecord{
		Version:  deployment.DeploymentRecordVersion,
		Identity: identity,
		Source:   deployment.DeploymentSource{Kind: "repository", Repository: filepath.Dir(manifestPath), Manifest: manifestPath},
		Applied:  deployment.AppliedDeployment{Intent: intent, RuntimeProvider: node.RuntimeProvider},
	}); err != nil {
		t.Fatalf("remote status fixture save deployment record: %v", err)
	}
	if seededWitness {
		seedIdentityMetricsWitness(t, reloaded, node.Name)
	}
	return ctx, store, reloaded.Name
}

// remoteIdentityStatusJSON resolves the Connector-backed remote Target and runs
// the production collectApplicationStatusResult collector for it — the exact
// collector the CLI `baha status` command and the MCP status tool call — and
// serialises the production applicationStatusResult through the public
// writeJSON encoder used by `baha status --json`, then decodes the two
// gated fields. This drives the REAL remote production path (app_status_result.go
// remote early return via hasRemoteApplicationTarget), not a path helper, and
// needs no live mTLS/Core session: the gate fields are computed from the
// resolved manifest before the remote target exits the path
// (app_status_result.go:176-190).
func remoteIdentityStatusJSON(t *testing.T, ctx context.Context, store application.Store, name string, perses bool) identityStatusJSON {
	t.Helper()
	resolved, err := resolveApplication(ctx, store, []string{name}, "status")
	if err != nil {
		t.Fatalf("resolve remote gate application: %v", err)
	}
	if !hasRemoteApplicationTarget(resolved) {
		t.Fatalf("the remote gate fixture must resolve to a remote target, got access provider %q", resolved.Target.AccessProvider)
	}
	var encoded bytes.Buffer
	var errOut bytes.Buffer
	// Run the real app-status command handler, including its remote early return
	// and writeJSON encoder, against the Connector-backed target fixture.
	if err := appStatusCommand(store).Run(ctx, []string{name, "--json"}, &encoded, &errOut); err != nil {
		t.Fatalf("remote baha app status --json failed: %v\nstdout: %s\nstderr: %s", err, encoded.String(), errOut.String())
	}
	var decoded identityStatusJSON
	if err := json.Unmarshal(encoded.Bytes(), &decoded); err != nil {
		t.Fatalf("the remote status result is not the public JSON report: %v", err)
	}
	return decoded
}

// persesStatusToString normalises the *capability.PersesStatus carried on the
// status result to a *string for the machine-readable JSON field assertions,
// mirroring the serialised identity_dashboard_eligible boolean.
func persesStatusToString(status *capability.PersesStatus) *string {
	if status == nil {
		return nil
	}
	s := string(*status)
	return &s
}

// TestIdentityDashboardEligibleRequiresConfiguredVerifiedPersesRemoteTarget is
// the remote-target gate regression. It drives the genuine remote-target status
// command (non-local access provider) for BOTH manifest faces and asserts the
// serialised identity_dashboard_eligible field on the remote early return is
// true ONLY when the gate holds: Perses selected AND a real Keycloak witness.
// The local-path regression is covered by TestIdentityDashboardEligibleRequiresConfiguredVerifiedPerses.
func TestIdentityDashboardEligibleRequiresConfiguredVerifiedPersesRemoteTarget(t *testing.T) {
	t.Run("perses-selected-configured-unverified-fails-closed", func(t *testing.T) {
		ctx, store, name := remotePersesStatusFixture(t, true, true)
		decoded := remoteIdentityStatusJSON(t, ctx, store, name, true)
		if decoded.Perses == nil {
			t.Fatal("the remote status JSON must expose a machine-readable perses field")
		}
		if *decoded.Perses != string(capability.PersesStatusConfiguredUnverified) {
			t.Fatalf("selected+no-health Perses must report configured_unverified, got %q", *decoded.Perses)
		}
		if decoded.IdentityDashboardEligible {
			t.Fatalf("remote identity_dashboard_eligible=true with Perses configured_unverified + real Keycloak witness: the gate must require verified, emit false on the remote early return\n%+v", decoded)
		}
	})

	t.Run("perses-unselected-fails-closed", func(t *testing.T) {
		ctx, store, name := remotePersesStatusFixture(t, false, false)
		decoded := remoteIdentityStatusJSON(t, ctx, store, name, false)
		if *decoded.Perses != string(capability.PersesStatusNotConfigured) {
			t.Fatalf("unselected Perses must report not_configured, got %q", *decoded.Perses)
		}
		if decoded.IdentityDashboardEligible {
			t.Fatalf("remote identity_dashboard_eligible=true with Perses not_configured: the gate must fail closed on the remote early return\n%+v", decoded)
		}
	})
}
