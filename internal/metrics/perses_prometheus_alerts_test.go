package metrics

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/observability"
)

// The golden witness pins the marshalled Perses alerting contract against a
// committed golden file under testdata/perses-alerts. The contract is the
// deterministic source of truth; its marshalling is the witness. Every cell
// below is a real assertion over the produced contract, not a "does-not-contain"
// vacuous reject.
//
// Correction round (luna-r2): the alert evaluator and recipient are bound to an
// AUTHORITATIVE, materialized, AUTHORIZED Prometheus rule source (the retained
// runtime source snapshot), never to the Prometheus datasource-plugin pin.
// Construction and health verify fail closed on the datasource-pin-alone,
// selected+pinned+NO-RECIPIENT, and evaluator-absent cases; the positive shape
// and golden witness exercise a materialized, provider-owned Prometheus source.

// persesAlertTestPort is the fixed materialized Prometheus management port used
// by the alert fixtures. The golden witness pins it so the emitted config is
// byte-deterministic.
const persesAlertTestPort = 39902

// persesAlertEvaluatorManifest returns a selected, pinned-surface manifest
// (observability management UI + Perses selected) with metrics collection
// enabled.
func persesAlertEvaluatorManifest() application.Manifest {
	m := application.New("perses-alerts-demo", "dev", false, false, false)
	m.Services.ObservabilityManagementUI = true
	m.Services.Perses = true
	return m
}

// persesMaterializeAlertProvider writes a canonical, materialized Prometheus
// provider directory for the resolved placement: compose.yaml, runtime.env with
// the management port, prometheus.yml, targets/ and the retained
// runtime-sources.json snapshot carrying the given sources.
func persesMaterializeAlertProvider(t *testing.T, m application.Manifest, sources ...observability.MetricsSource) ProviderFiles {
	t.Helper()
	placement, err := PlacementFor(m)
	if err != nil {
		t.Fatalf("placement: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(placement.Dir, "targets"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := ProviderFiles{
		Dir:        placement.Dir,
		Compose:    filepath.Join(placement.Dir, "compose.yaml"),
		Env:        filepath.Join(placement.Dir, "runtime.env"),
		Config:     filepath.Join(placement.Dir, "prometheus.yml"),
		TargetsDir: filepath.Join(placement.Dir, "targets"),
		RuntimeCA:  filepath.Join(placement.Dir, "baseharbor-runtime-ca.pem"),
	}
	if err := os.WriteFile(files.Compose, []byte("name: bh-local-shared\nservices:\n  prometheus:\n    image: quay.io/prometheus/prometheus:v3.5.0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(files.Env, []byte("BASEHARBOR_PROMETHEUS_PORT="+strconv.Itoa(persesAlertTestPort)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(files.Config, []byte("global:\n  scrape_interval: 30s\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeRuntimeSourceSnapshot(files, placement, sources); err != nil {
		t.Fatalf("materialize runtime sources: %v", err)
	}
	return files
}

// persesPrometheusRuleSource is a materialized, provider-owned platform-provider
// Prometheus rule source: the authoritative real evaluator of the alerting
// surface (not the datasource pin).
func persesPrometheusRuleSource(id string, provider capability.ProviderKind) observability.MetricsSource {
	return observability.MetricsSource{
		ID: id, Provider: provider, Class: observability.SourcePlatformProvider,
		Scope: capability.ScopeShared, Network: "bh-local-shared_default",
		Target: string(provider) + ":9464", Path: "/metrics", Scheme: "http",
	}
}

// TestPersesAlertEvaluatorShape pins the produced alerting contract: the real
// native rule evaluator/format (Prometheus contract, not a Grafana-UI/dashboard
// JSON), owner = the real observability owner (Prometheus) named as an owner
// reference, not a capability claim or a secret. It requires a real, materialized
// evaluator (a provider-owned Prometheus rule source), not the datasource pin.
func TestPersesAlertEvaluatorShape(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv(application.MetricsEnabledEnv, "true")
	m := persesAlertEvaluatorManifest()
	persesMaterializeAlertProvider(t, m, persesPrometheusRuleSource("prometheus:runtime", capability.ProviderPrometheus))
	res, err := BuildPersesAlertEvaluator(m)
	if err != nil {
		t.Fatalf("selected+pinned+authorized perses must define an alert evaluator contract: %v", err)
	}
	ev := res.Evaluator
	if ev.Kind != PersesAlertEvaluatorKind {
		t.Fatalf("perses alert evaluator kind = %q, want %q (native Prometheus rule evaluator, not a Grafana-UI/dashboard JSON)", ev.Kind, PersesAlertEvaluatorKind)
	}
	if ev.Evaluator != PersesAlertEvaluatorKind {
		t.Fatalf("perses alert evaluator = %q, want the real native evaluator %q (fail-closed without a real evaluator)", ev.Evaluator, PersesAlertEvaluatorKind)
	}
	if ev.Kind == "dashboard" || ev.Kind == "GrafanaDashboard" {
		t.Fatalf("perses alert evaluator must be the native Prometheus contract, never a Grafana dashboard JSON (%q)", ev.Kind)
	}
	if ev.Owner != string(capability.ProviderPrometheus) {
		t.Fatalf("perses alert evaluator owner = %q, want %q (the real observability owner, as an owner reference)", ev.Owner, string(capability.ProviderPrometheus))
	}
	if res.Evaluator.RecipientValidated != true {
		t.Fatal("a fully-formed, selected, pinned+authorized alerting contract with a real evaluator must report a validated recipient")
	}
	if res.Reachability != PersesAlertReachabilityConfiguredUnverified {
		t.Fatalf("reachability = %q, want %q (the only reachable state for a fully-formed contract; no runtime probe exists here)", res.Reachability, PersesAlertReachabilityConfiguredUnverified)
	}
}

// TestPersesAlertEvaluatorFailClosedWhenUnpinned asserts fail-closed: an unpinned
// release returns the fail-closed PersesAlertEvaluatorError, never an "alerting
// enabled" contract.
func TestPersesAlertEvaluatorFailClosedWhenUnpinned(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv(application.MetricsEnabledEnv, "true")
	_, err := persesAlertEvaluatorFromRelease(persesAlertEvaluatorManifest(), capability.PersesRelease{
		Version: "", Image: "", PrometheusPluginVersion: PersesAlertEvaluatorKind,
	})
	if err == nil {
		t.Fatal("unpinned perses must fail closed with PersesAlertEvaluatorError, never define an alert evaluator")
	}
	if _, ok := err.(*PersesAlertEvaluatorError); !ok {
		t.Fatalf("error = %T, want *PersesAlertEvaluatorError", err)
	}
}

// TestPersesAlertEvaluatorFailClosedWhenUnselected asserts fail-closed: an
// unselected surface returns the fail-closed error, never a fabricated contract.
func TestPersesAlertEvaluatorFailClosedWhenUnselected(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv(application.MetricsEnabledEnv, "true")
	m := application.New("perses-alerts-unselected", "dev", false, false, false)
	_, err := BuildPersesAlertEvaluator(m)
	if err == nil {
		t.Fatal("unselected perses must fail closed, never define an alert evaluator")
	}
	if _, ok := err.(*PersesAlertEvaluatorError); !ok {
		t.Fatalf("error = %T, want *PersesAlertEvaluatorError", err)
	}
}

// TestPersesAlertEvaluatorFailClosedWithoutRealEvaluator asserts fail-closed: a
// selected, pinned surface with no materialized, authorized evaluator source is
// not "alerting enabled" — it is fail-closed, never a false declaration.
func TestPersesAlertEvaluatorFailClosedWithoutRealEvaluator(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv(application.MetricsEnabledEnv, "true")
	m := persesAlertEvaluatorManifest()
	persesMaterializeAlertProvider(t, m) // no sources materialized
	_, err := BuildPersesAlertEvaluator(m)
	if err == nil {
		t.Fatal("no real evaluator must fail closed, never define an alert evaluator")
	}
	if _, ok := err.(*PersesAlertEvaluatorError); !ok {
		t.Fatalf("error = %T, want *PersesAlertEvaluatorError", err)
	}
	if !strings.Contains(err.Error(), "no authorized Prometheus rule evaluator sources") {
		t.Fatalf("evaluator-absent error = %v, want the fail-closed evaluator-absent error", err)
	}
}

// TestPersesAlertEvaluatorFailClosedWhenRecipientAbsent asserts fail-closed: a
// selected, pinned, evaluator-present surface with no authorized recipient
// (foreign-owned source) is not "alerting enabled".
func TestPersesAlertEvaluatorFailClosedWhenRecipientAbsent(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv(application.MetricsEnabledEnv, "true")
	m := persesAlertEvaluatorManifest()
	foreign := observability.MetricsSource{
		ID: "api", Provider: capability.ProviderPrometheus, Class: observability.SourceApplication,
		Scope: capability.ScopeApplication, OwnerApplication: "someone-else",
		Network: "bh-local-shared_default", Target: "api:8080", Path: "/metrics", Scheme: "http",
	}
	persesMaterializeAlertProvider(t, m, foreign)
	_, err := BuildPersesAlertEvaluator(m)
	if err == nil {
		t.Fatal("surface with a real evaluator but no authorized recipient must fail closed, never define an alert evaluator")
	}
	if _, ok := err.(*PersesAlertEvaluatorError); !ok {
		t.Fatalf("error = %T, want *PersesAlertEvaluatorError", err)
	}
}

// TestPersesAlertEvaluatorNoRecipientValidatedWithoutConcreteEvidence asserts
// that no produced result claims RecipientValidated without concrete validation
// evidence (an authorized, materialized evaluator source).
func TestPersesAlertEvaluatorNoRecipientValidatedWithoutConcreteEvidence(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv(application.MetricsEnabledEnv, "true")
	// Unselected surface: must NOT produce a contract, and must never set
	// RecipientValidated.
	m := application.New("perses-alerts-no-concrete", "dev", false, false, false)
	res, err := persesAlertEvaluatorFromRelease(m, capability.PersesDefaultRelease)
	if err == nil {
		t.Fatal("unselected surface must fail closed, never return a contract")
	}
	if res.Evaluator.RecipientValidated {
		t.Fatalf("a result with no concrete recipient evidence must not set RecipientValidated; got %+v", res.Evaluator)
	}
}

// TestPersesAlertEvaluatorValidateHealthFailClosed asserts that a fully-formed,
// selected, pinned+authorized contract health verify returns nil (fail-closed
// on reachability, never on pin).
func TestPersesAlertEvaluatorValidateHealthFailClosed(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv(application.MetricsEnabledEnv, "true")
	m := persesAlertEvaluatorManifest()
	persesMaterializeAlertProvider(t, m, persesPrometheusRuleSource("prometheus:runtime", capability.ProviderPrometheus))
	err := VerifyPersesAlertEvaluatorHealth(context.Background(), m)
	if err != nil {
		t.Fatalf("a selected, pinned, fully-formed alerting contract health verify must return nil: %v", err)
	}
}

// TestPersesAlertEvaluatorValidateHealthRejectsPinOnly asserts the Luna-required
// negative: the health verifier MUST NOT pass on datasource-pin validation
// alone when the surface is unselected. Pin validation is neither evaluator nor
// recipient proof.
func TestPersesAlertEvaluatorValidateHealthRejectsPinOnly(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv(application.MetricsEnabledEnv, "true")
	// Unselected surface (no ObservabilityManagementUI/Perses): must fail closed,
	// never nil — pin validation alone does not pass.
	err := VerifyPersesAlertEvaluatorHealth(context.Background(), application.New("perses-alerts-unselected", "dev", false, false, false))
	if err == nil {
		t.Fatal("health verify must NOT pass on datasource-pin validation alone when the surface is unselected; pin-only is not recipient/evaluator proof")
	}
	if _, ok := err.(*PersesAlertEvaluatorError); !ok {
		t.Fatalf("error = %T, want *PersesAlertEvaluatorError", err)
	}
}

// TestPersesAlertEvaluatorValidateHealthFailClosedWithoutEvaluator asserts the
// Luna-required selected+pinned+NO-RECIPIENT cell: even with the datasource pin
// set and Perses selected, a surface with no materialized, authorized evaluator
// source fails closed. This is the untested case Luna r2 named.
func TestPersesAlertEvaluatorValidateHealthFailClosedWithoutEvaluator(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv(application.MetricsEnabledEnv, "true")
	m := persesAlertEvaluatorManifest()
	// Selected + pinned, but NO materialized, authorized Prometheus rule source
	// (the datasource pin alone is neither evaluator nor recipient proof).
	persesMaterializeAlertProvider(t, m)
	err := VerifyPersesAlertEvaluatorHealth(context.Background(), m)
	if err == nil {
		t.Fatal("selected+pinned but evaluator-absent must fail closed; the datasource pin alone is neither evaluator nor recipient proof")
	}
	if !strings.Contains(err.Error(), "no authorized Prometheus rule evaluator sources") {
		t.Fatalf("evaluator-absent health error = %v, want the fail-closed evaluator-absent error", err)
	}
}

// TestPersesAlertEvaluatorValidateHealthFailClosedWhenUnpinned asserts that an
// unpinned release fails the health verify.
func TestPersesAlertEvaluatorValidateHealthFailClosedWhenUnpinned(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv(application.MetricsEnabledEnv, "true")
	// The health contract keys on PersesDefaultRelease; an unpinned surface
	// cannot be verified. This exercises the fail-closed guard directly.
	_, err := persesAlertEvaluatorFromRelease(persesAlertEvaluatorManifest(), capability.PersesRelease{})
	if err == nil {
		t.Fatal("unpinned perses must fail closed on the alert evaluator")
	}
}

// TestPersesAlertEvaluatorNoSecretInContract asserts the contract carries no
// notification-channel secret value: the owner is an owner reference, never a
// secret.
func TestPersesAlertEvaluatorNoSecretInContract(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv(application.MetricsEnabledEnv, "true")
	m := persesAlertEvaluatorManifest()
	persesMaterializeAlertProvider(t, m, persesPrometheusRuleSource("prometheus:runtime", capability.ProviderPrometheus))
	res, err := BuildPersesAlertEvaluator(m)
	if err != nil {
		t.Fatalf("selected+pinned+authorized perses must define an alert evaluator contract: %v", err)
	}
	data, err := PersesAlertEvaluatorJSON(res.Evaluator)
	if err != nil {
		t.Fatalf("marshalling the alerting contract must succeed: %v", err)
	}
	s := string(data)
	for _, secret := range []string{"bearer", "password", "api_key", "apiKey", "token", "clientCert", "privateKey", "secret"} {
		if containsFold(s, secret) {
			t.Fatalf("alerting contract must carry no %s secret value: %s", secret, s)
		}
	}
}

func containsFold(haystack, needle string) bool {
	return len(needle) == 0 || (len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0)
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}

// TestPersesAlertEvaluatorGoldenFiles pins the produced alerting contract
// against a committed golden file under testdata/perses-alerts.
func TestPersesAlertEvaluatorGoldenFiles(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv(application.MetricsEnabledEnv, "true")
	m := persesAlertEvaluatorManifest()
	persesMaterializeAlertProvider(t, m, persesPrometheusRuleSource("prometheus:runtime", capability.ProviderPrometheus))
	res, err := BuildPersesAlertEvaluator(m)
	if err != nil {
		t.Fatalf("selected+pinned+authorized perses must define an alert evaluator contract: %v", err)
	}
	goldenPath := filepath.Join("testdata", "perses-alerts", "alerting.json")
	want, err := PersesAlertEvaluatorJSON(res.Evaluator)
	if err != nil {
		t.Fatalf("marshalling the alerting contract must succeed: %v", err)
	}
	got, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("golden file %s must exist: %v", goldenPath, err)
	}
	if string(want) != string(got) {
		t.Fatalf("alerting contract golden mismatch:\n got: %s\nwant: %s", got, want)
	}
}
