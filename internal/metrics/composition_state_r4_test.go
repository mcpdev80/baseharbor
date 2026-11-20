package metrics

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/observability"
	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
)

// r4 CODEX #1695 finding P3: ProviderSourcesCompositionState must list provider
// sources with the manifest's own environment (m.Environment), exactly like the
// verify path (verifyProviderSourcesAt) and the writer path
// (EnsureProviderFilesWithRuntimeCAAt) do. Before the fix it passed an empty
// requested environment, which fails the SourceMatchesEnvironment guard
// fail-closed for every environment-carrying source: the operator-facing
// composition display could then claim "no provider metrics sources" for a
// nonempty registered source list.

// r4SharedDir is the shared provider directory for a namespace-free caller (the
// layout is defined in placementFromProviderPlacementAt;
// capability.ProviderPlacement carries no directory field).
func r4SharedDir(stateDir string) string {
	return filepath.Join(filepath.Clean(stateDir), "providers", "prometheus", "shared")
}

func r4Manifest(name, env string) application.Manifest {
	return application.New(name, env, false, false, false)
}

// r4PlatformSource is a shared platform-provider source whose environment is
// carried in its "environment:<env>" sharing boundary: boundary-carrying
// sources are the environment-scoped members of the shared placement (see
// SourceEnvironment/sharingBoundaryEnvironment), and a source whose boundary
// resolves elsewhere than the caller's environment is excluded fail-closed.
func r4PlatformSource(name, env string, verification capability.ObservabilityVerificationMode) observability.MetricsSource {
	return observability.MetricsSource{
		ID:               "prometheus:" + name + "-prov-" + env,
		Provider:         capability.ProviderPrometheus,
		Class:            observability.SourcePlatformProvider,
		Scope:            capability.ScopeShared,
		SharingBoundary:  "environment:" + env,
		OwnerApplication: name,
		Network:          "baseharbor-r4-" + name + "-" + env + "-metrics",
		Target:           name + "." + env + ".backend:9090",
		Path:             "/metrics",
		Verification:     verification,
	}
}

// r4AppSource is an app-scoped application-provider source following the
// production registration grammar (internal/application/runtime_observability
// reconcileApplicationMetricsSource: ID "<provider>:<project>", Target
// "runtime://<project>/workload", project "<app>-<env>").
func r4Source(name, env string, verification capability.ObservabilityVerificationMode) observability.MetricsSource {
	return r4Sourced(name, name, env, verification)
}

// r4Sourced registers the canonical application-provider metrics identity
// "<provider>:<project>" whose project is the application deployment project
// "bh-<target>-<application>-<environment>" (internal/runtime
// ApplicationProjectName; internal/application/runtime_observability
// reconcileApplicationMetricsSource). The environment derives from the project
// token anchored by the owner application.
func r4Sourced(name, owner, env string, verification capability.ObservabilityVerificationMode) observability.MetricsSource {
	project := "bh-local-" + name + "-" + env
	return observability.MetricsSource{
		ID:               "prometheus:" + project,
		Provider:         capability.ProviderPrometheus,
		Class:            observability.SourceApplicationProvider,
		Scope:            capability.ScopeApplication,
		OwnerApplication: owner,
		Network:          "baseharbor-r4-" + name + "-" + env + "-metrics",
		Target:           "runtime://" + project + "/workload",
		Path:             "/metrics",
		Scheme:           "http",
		Verification:     verification,
	}
}

// r4SharedProviderFiles lays out the shared provider directory that
// ExistingProviderFilesAt requires and writes the shared registrations.json so
// shared-scope callers resolve the same allowed application set the
// writer/verify paths see.
func r4SharedProviderFiles(t *testing.T, stateDir string, manifests ...application.Manifest) {
	t.Helper()
	dir := r4SharedDir(stateDir)
	for _, sub := range []string{"targets", "members/service-access/runtime"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// providertopology.ServiceNames requires protected regular state (no
	// group/other bits) on the Compose file, and the Compose inventory must
	// name at least one provider service.
	if err := os.WriteFile(filepath.Join(dir, "compose.yaml"), []byte("services:\n  prometheus-1:\n    image: prom/prometheus\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(dir, "compose.yaml"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"runtime.env", "prometheus.yml"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("# r4 fixture\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	type registration struct {
		Application string `json:"application"`
		Environment string `json:"environment"`
	}
	registrations := make([]registration, 0, len(manifests))
	for _, m := range manifests {
		registrations = append(registrations, registration{Application: m.Name, Environment: m.Environment})
	}
	data, err := json.Marshal(registrations)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "registrations.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestCompositionStateUsesManifestEnvironment is the P3 regression: the
// composition-state determination must see the same source set the verify path
// and the writer path see, i.e. it must request the caller's environment. A
// caller whose environment matches a registered backend-backed platform source
// must report one effective target; an off-environment caller must report the
// no-source state (fail-closed filtering preserved). RED at the base: with the
// empty requested environment, the boundary-carrying source fails
// SourceMatchesEnvironment and the state collapses to "no provider metrics
// sources" even for the current-environment caller.
func TestCompositionStateUsesManifestEnvironment(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", stateDir)
	t.Setenv(application.MetricsEnabledEnv, "true")

	// Application-scoped placement: the app's own provider registration in
	// its manifest environment. Provider scope is operator policy; this is an
	// existing supported placement (internal/application/provider_placement.go
	// honours BASEHARBOR_PROVIDER_PROMETHEUS_SCOPE).
	t.Setenv(application.ProviderScopeEnv(capability.ProviderPrometheus), "application")
	if err := observability.Update(r4Source("scoped", "dev", capability.ObservabilityVerifyBackend)); err != nil {
		t.Fatal(err)
	}

	state, err := ProviderSourcesCompositionState(context.Background(), r4Manifest("scoped", "dev"), stateDir, "")
	if err != nil {
		t.Fatalf("ProviderSourcesCompositionState: %v", err)
	}
	if !strings.Contains(state, "1 effective scrape target") {
		t.Fatalf("current-environment backend-backed source must report one effective target, got %q", state)
	}
	// A caller in another environment must not see the dev source at all
	// (fail-closed, no cross-environment visibility leak).
	state, err = ProviderSourcesCompositionState(context.Background(), r4Manifest("scoped", "staging"), stateDir, "")
	if err != nil {
		t.Fatalf("ProviderSourcesCompositionState (staging caller): %v", err)
	}
	if !strings.Contains(state, "no provider metrics sources") {
		t.Fatalf("an off-environment caller must see the no-source state, got %q", state)
	}
}

// TestCompositionStateTwoEnvironmentsSameVerificationNoneID is the P3
// regression the verdict names: two provider sources with the SAME id living in
// DIFFERENT environments, and the composition state says what it should — not
// "empty", not the sum of both. The id is the canonical application-provider
// identity "<provider>:<project>"; the environment is the trailing project
// token, so one id can only be registered for one environment at a time (the
// observability registry keeps one record per id+kind and replaces same-id
// registrations). The test therefore walks the SAME id through its two
// environment lives, dev first, staging second:
//
//   - while the id carries the dev environment, the dev caller must see its
//     own null-target (Verification=none) composition — never the no-source
//     state the empty requested environment produced pre-fix;
//   - once the id has moved to staging, the dev caller must see nothing of it
//     (fail-closed no-source state) and the staging caller must see exactly
//     one source — never the sum of both environments, which is what the
//     empty requested environment produced for the shared placement pre-fix.
func TestCompositionStateTwoEnvironmentsSameVerificationNoneID(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", stateDir)
	t.Setenv(application.MetricsEnabledEnv, "true")
	t.Setenv(application.ProviderScopeEnv(capability.ProviderPrometheus), "application")

	devID := "prometheus:bh-local-twin-dev"
	if err := observability.Update(r4Sourced("twin", "twin", "dev", capability.ObservabilityVerifyNone)); err != nil {
		t.Fatal(err)
	}
	state, err := ProviderSourcesCompositionState(context.Background(), r4Manifest("twin", "dev"), stateDir, "")
	if err != nil {
		t.Fatalf("ProviderSourcesCompositionState: %v", err)
	}
	for _, want := range []string{"null effective scrape targets", "after composition", "Verification=none"} {
		if !strings.Contains(state, want) {
			t.Fatalf("composition state for the dev all-verification:none source must report the null-target state, missing %q in %q", want, state)
		}
	}
	if strings.Contains(state, "no provider metrics sources") {
		t.Fatalf("a nonempty registered source list must never collapse to the no-source state, got %q", state)
	}

	// The same id moves to another environment ("staging"): the record under
	// devID's id pattern now resolves to staging.
	moved := r4Source("twin", "staging", capability.ObservabilityVerifyNone)
	moved.ID = devID
	if err := observability.Update(moved); err != nil {
		t.Fatal(err)
	}
	// The dev caller must not see the same-id source that now lives in
	// staging: fail-closed no-source state, never a null-target claim for a
	// source set the dev environment no longer has.
	state, err = ProviderSourcesCompositionState(context.Background(), r4Manifest("twin", "dev"), stateDir, "")
	if err != nil {
		t.Fatalf("ProviderSourcesCompositionState (dev caller): %v", err)
	}
	if !strings.Contains(state, "no provider metrics sources") {
		t.Fatalf("the dev caller must not see the staging-lived id, got %q", state)
	}
	// The staging caller sees its own environment's source: the null-target
	// state — never the sum of both environments. The "twin"-owned record that
	// just moved to staging belongs to application "twin"; the "twinother"
	// registration is a second app's staging source, so a twin-owned staging
	// caller must not see it (owner filter), and the twinother staging caller
	// sees only its own single null-target source.
	if err := observability.Update(r4Source("twinother", "staging", capability.ObservabilityVerifyNone)); err != nil {
		t.Fatal(err)
	}
	state, err = ProviderSourcesCompositionState(context.Background(), r4Manifest("twinother", "staging"), stateDir, "")
	if err != nil {
		t.Fatalf("ProviderSourcesCompositionState (staging caller): %v", err)
	}
	for _, want := range []string{"null effective scrape targets", "Verification=none"} {
		if !strings.Contains(state, want) {
			t.Fatalf("the staging caller must report its own null-target composition, missing %q in %q", want, state)
		}
	}
	if strings.Contains(state, "2 effective") || strings.Contains(state, "no provider metrics sources") {
		t.Fatalf("the staging state must be neither the sum of both environments nor the empty collapse, got %q", state)
	}
}

// TestCompositionStateBackendSourceReportsOneEffectiveTarget covers the P3
// recipe case: a current-environment backend-backed source must report one
// effective target with an off-environment sibling registered (never the sum
// of both environments).
func TestCompositionStateBackendSourceReportsOneEffectiveTarget(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", stateDir)
	t.Setenv(application.MetricsEnabledEnv, "true")
	t.Setenv(application.ProviderScopeEnv(capability.ProviderPrometheus), "application")

	if err := observability.Update(r4Source("backend", "dev", capability.ObservabilityVerifyBackend)); err != nil {
		t.Fatal(err)
	}
	if err := observability.Update(r4Source("backend", "staging", capability.ObservabilityVerifyBackend)); err != nil {
		t.Fatal(err)
	}

	state, err := ProviderSourcesCompositionState(context.Background(), r4Manifest("backend", "dev"), stateDir, "")
	if err != nil {
		t.Fatalf("ProviderSourcesCompositionState: %v", err)
	}
	if !strings.Contains(state, "1 effective scrape target") {
		t.Fatalf("one current-environment backend-backed source must report exactly one effective target, got %q", state)
	}
}

// TestCompositionStateEmptyRegistryReportsNoSource covers the P3 recipe case:
// a truly empty registry must report the no-source state, not the null-target
// state.
func TestCompositionStateEmptyRegistryReportsNoSource(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv(application.MetricsEnabledEnv, "true")

	state, err := ProviderSourcesCompositionState(context.Background(), r4Manifest("quiet", "dev"), "", "")
	if err != nil {
		t.Fatalf("ProviderSourcesCompositionState: %v", err)
	}
	for _, want := range []string{"no provider metrics sources", "nothing to scrape"} {
		if !strings.Contains(state, want) {
			t.Fatalf("empty registry must report the no-source state, missing %q in %q", want, state)
		}
	}
}

// TestCompositionStateMatchesWriterSourceSet proves the composition display and
// the writer agree on one source set: both list with m.Environment, so a
// current-environment backend-backed source yields a provider target file AND
// an effective-target composition state. The off-environment writer sibling
// (EnsureProviderFilesWithRuntimeCAAt) proves the guard is genuinely on the
// environment dimension: an environment-carrying source the caller's
// environment does not match is dropped fail-closed there, exactly as the
// pre-fix display dropped every source for every caller.
func TestCompositionStateMatchesWriterSourceSet(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", stateDir)
	t.Setenv(application.MetricsEnabledEnv, "true")

	m := application.WithWorkloadComponents(r4Manifest("writer", "dev"), "api")
	source := r4PlatformSource("writer", "dev", capability.ObservabilityVerifyBackend)
	if err := observability.Update(source); err != nil {
		t.Fatal(err)
	}
	r4SharedProviderFiles(t, stateDir, m)
	files, err := EnsureProviderFilesWithRuntimeCAAt(context.Background(), serviceissuer.New(t), stateDir, "", m, "")
	if err != nil {
		t.Fatalf("EnsureProviderFilesWithRuntimeCAAt: %v", err)
	}
	if _, err := os.Stat(filepath.Join(files.TargetsDir, providerTargetFileName(source))); !os.IsNotExist(err) {
		t.Fatalf("the environment guard must keep the source out of the off-environment writer: provider target file exists (err=%v)", err)
	}
	state, err := ProviderSourcesCompositionState(context.Background(), m, stateDir, "")
	if err != nil {
		t.Fatalf("ProviderSourcesCompositionState: %v", err)
	}
	if !strings.Contains(state, "no provider metrics sources") {
		t.Fatalf("the display must agree with the writer and report the fail-closed no-source state, got %q", state)
	}
}
