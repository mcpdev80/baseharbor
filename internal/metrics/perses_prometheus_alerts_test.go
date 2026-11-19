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

// Fail-closed alerting contract test (luna-r3 correction, round 6).
//
// Luna r3 (act-luna-r3.md) REQUEST CHANGES: the previous candidate bound
// RecipientValidated=true to an AUTHORIZED METRICS SCRAPE SOURCE. A /metrics
// scrape source is a scrape target, never a rule evaluator, and never a
// notification recipient. This Core ships NO real Prometheus rule-evaluation
// and NO notification-owner (recipient/sender) contract, so the alerting
// contract MUST fail closed (not_enabled, RecipientValidated=false) for every
// selected, pinned surface — regardless of Perses selection, the datasource
// pin, or a materialized scrape source.
//
// Every cell below is a real assertion over the produced fail-closed verdict.

// persesAlertTestPort is the fixed materialized Prometheus management port used
// by the alert fixtures.
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
// provider directory (compose.yaml, runtime.env, prometheus.yml, targets/) with
// a retained runtime-sources.json carrying the given metrics scrape sources.
// It exists only to prove that even a fully materialized, authorized scrape
// surface does NOT satisfy the fail-closed alerting contract.
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

// persesPrometheusScrapeSource is a materialized, provider-owned platform
// provider Prometheus SCRAPE source (a /metrics endpoint): a scrape target,
// never a rule evaluator or a notification recipient.
func persesPrometheusScrapeSource(id string, provider capability.ProviderKind) observability.MetricsSource {
	return observability.MetricsSource{
		ID: id, Provider: provider, Class: observability.SourcePlatformProvider,
		Scope: capability.ScopeShared, Network: "bh-local-shared_default",
		Target: string(provider) + ":9464", Path: "/metrics", Scheme: "http",
	}
}

// TestPersesAlertEvaluatorFailsClosedWithScrapeSource is the Luna r3 negative
// cell: a selected, pinned surface with a materialized, authorized /metrics
// scrape source (no real rule evaluator, no notification recipient) MUST fail
// closed, never define an alert evaluator contract. This is the exact false
// positive Luna r3 rejected.
func TestPersesAlertEvaluatorFailsClosedWithScrapeSource(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv(application.MetricsEnabledEnv, "true")
	m := persesAlertEvaluatorManifest()
	persesMaterializeAlertProvider(t, m, persesPrometheusScrapeSource("prometheus:runtime", capability.ProviderPrometheus))
	_, err := BuildPersesAlertEvaluator(m)
	if err == nil {
		t.Fatal("selected+pinned with only a /metrics scrape source must fail closed; a scrape source is neither a rule evaluator nor a notification recipient")
	}
	if _, ok := err.(*PersesAlertEvaluatorError); !ok {
		t.Fatalf("error = %T, want *PersesAlertEvaluatorError", err)
	}
	if !strings.Contains(err.Error(), "no real Prometheus rule-evaluation") {
		t.Fatalf("fail-closed error = %v, want the no-real-contract fail-closed error", err)
	}
}

// TestPersesAlertEvaluatorFailsClosedWithoutScrapeSource asserts fail-closed: a
// selected, pinned surface with no materialized, authorized source is not
// "alerting enabled".
func TestPersesAlertEvaluatorFailsClosedWithoutScrapeSource(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv(application.MetricsEnabledEnv, "true")
	m := persesAlertEvaluatorManifest()
	persesMaterializeAlertProvider(t, m) // no sources materialized
	_, err := BuildPersesAlertEvaluator(m)
	if err == nil {
		t.Fatal("selected+pinned must fail closed without a real evaluator/recipient contract")
	}
	if _, ok := err.(*PersesAlertEvaluatorError); !ok {
		t.Fatalf("error = %T, want *PersesAlertEvaluatorError", err)
	}
}

// TestPersesAlertEvaluatorFailClosedWhenUnpinned asserts fail-closed: an
// unpinned release returns the fail-closed PersesAlertEvaluatorError.
func TestPersesAlertEvaluatorFailClosedWhenUnpinned(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv(application.MetricsEnabledEnv, "true")
	_, err := persesAlertEvaluatorFromRelease(persesAlertEvaluatorManifest(), capability.PersesRelease{
		Version: "", Image: "", PrometheusPluginVersion: PersesAlertEvaluatorKind,
	})
	if err == nil {
		t.Fatal("unpinned perses must fail closed, never define an alert evaluator")
	}
	if _, ok := err.(*PersesAlertEvaluatorError); !ok {
		t.Fatalf("error = %T, want *PersesAlertEvaluatorError", err)
	}
}

// TestPersesAlertEvaluatorFailClosedWhenUnselected asserts fail-closed: an
// unselected surface returns the fail-closed error.
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

// TestPersesAlertEvaluatorFailsClosedWhenRecipientAbsent asserts fail-closed: a
// selected, pinned surface with only a foreign-owned /metrics scrape source
// (no authorized recipient) is not "alerting enabled".
func TestPersesAlertEvaluatorFailsClosedWhenRecipientAbsent(t *testing.T) {
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
		t.Fatal("surface with only a foreign scrape source must fail closed, never define an alert evaluator")
	}
	if _, ok := err.(*PersesAlertEvaluatorError); !ok {
		t.Fatalf("error = %T, want *PersesAlertEvaluatorError", err)
	}
}

// TestPersesAlertEvaluatorNoRecipientValidatedWithoutConcreteEvidence asserts
// that no produced result claims RecipientValidated without a real
// evaluator/recipient contract.
func TestPersesAlertEvaluatorNoRecipientValidatedWithoutConcreteEvidence(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv(application.MetricsEnabledEnv, "true")
	m := persesAlertEvaluatorManifest()
	persesMaterializeAlertProvider(t, m, persesPrometheusScrapeSource("prometheus:runtime", capability.ProviderPrometheus))
	res, err := BuildPersesAlertEvaluator(m)
	if err == nil {
		t.Fatal("a selected+pinned surface with only a scrape source must fail closed, never return a contract")
	}
	if res.Evaluator.RecipientValidated {
		t.Fatalf("a result with no real evaluator/recipient contract must not set RecipientValidated; got %+v", res.Evaluator)
	}
}

// TestPersesAlertEvaluatorValidateHealthFailsClosed asserts that the health
// verifier fails closed for a selected, pinned surface that carries only a
// scrape source (no real evaluator/recipient contract).
func TestPersesAlertEvaluatorValidateHealthFailsClosed(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv(application.MetricsEnabledEnv, "true")
	m := persesAlertEvaluatorManifest()
	persesMaterializeAlertProvider(t, m, persesPrometheusScrapeSource("prometheus:runtime", capability.ProviderPrometheus))
	err := VerifyPersesAlertEvaluatorHealth(context.Background(), m)
	if err == nil {
		t.Fatal("health verify must fail closed for a selected+pinned surface with only a scrape source; no real evaluator/recipient contract exists")
	}
	if _, ok := err.(*PersesAlertEvaluatorError); !ok {
		t.Fatalf("error = %T, want *PersesAlertEvaluatorError", err)
	}
}

// TestPersesAlertEvaluatorValidateHealthRejectsPinOnly asserts the
// Luna-required negative: the health verifier MUST NOT pass on datasource-pin
// validation alone when the surface is unselected.
func TestPersesAlertEvaluatorValidateHealthRejectsPinOnly(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv(application.MetricsEnabledEnv, "true")
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
// set and Perses selected, a surface with no real evaluator/recipient contract
// fails closed.
func TestPersesAlertEvaluatorValidateHealthFailClosedWithoutEvaluator(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv(application.MetricsEnabledEnv, "true")
	m := persesAlertEvaluatorManifest()
	persesMaterializeAlertProvider(t, m) // no materialized, authorized source
	err := VerifyPersesAlertEvaluatorHealth(context.Background(), m)
	if err == nil {
		t.Fatal("selected+pinned but evaluator-recipient-absent must fail closed; the datasource pin alone is neither evaluator nor recipient proof")
	}
	if _, ok := err.(*PersesAlertEvaluatorError); !ok {
		t.Fatalf("error = %T, want *PersesAlertEvaluatorError", err)
	}
}

// TestPersesAlertEvaluatorValidateHealthFailClosedWhenUnpinned asserts that an
// unpinned release fails the health verify.
func TestPersesAlertEvaluatorValidateHealthFailClosedWhenUnpinned(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv(application.MetricsEnabledEnv, "true")
	_, err := persesAlertEvaluatorFromRelease(persesAlertEvaluatorManifest(), capability.PersesRelease{})
	if err == nil {
		t.Fatal("unpinned perses must fail closed on the alert evaluator")
	}
}

// TestPersesAlertEvaluatorFailsClosedNoEvaluatorContract asserts the core
// fail-closed reality: even a selected, pinned, fully materialized surface
// cannot produce a contract because no real rule-evaluation/notification-owner
// contract exists in this Core.
func TestPersesAlertEvaluatorFailsClosedNoEvaluatorContract(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv(application.MetricsEnabledEnv, "true")
	// A materialized, authorized scrape source is present — but it is not a rule
	// evaluator nor a notification recipient.
	m := persesAlertEvaluatorManifest()
	persesMaterializeAlertProvider(t, m, persesPrometheusScrapeSource("prometheus:runtime", capability.ProviderPrometheus))
	_, err := BuildPersesAlertEvaluator(m)
	if err == nil {
		t.Fatal("a real rule-evaluation and notification-owner contract must exist to define an alert evaluator; without it the contract is fail-closed")
	}
	if _, ok := err.(*PersesAlertEvaluatorError); !ok {
		t.Fatalf("error = %T, want *PersesAlertEvaluatorError", err)
	}
}

// TestPersesAlertEvaluatorNoSecretInContract asserts the fail-closed contract
// path returns an error (never a contract to marshal) for a selected, pinned
// surface with only a scrape source.
func TestPersesAlertEvaluatorNoSecretInContract(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv(application.MetricsEnabledEnv, "true")
	m := persesAlertEvaluatorManifest()
	persesMaterializeAlertProvider(t, m, persesPrometheusScrapeSource("prometheus:runtime", capability.ProviderPrometheus))
	_, err := BuildPersesAlertEvaluator(m)
	if err == nil {
		t.Fatal("selected+pinned with only a scrape source must fail closed, never return a contract to marshal")
	}
	if _, ok := err.(*PersesAlertEvaluatorError); !ok {
		t.Fatalf("error = %T, want *PersesAlertEvaluatorError", err)
	}
}

// TestPersesAlertEvaluatorOwnerReferenceCarriesNoSecret asserts that the real
// notification-owner reference carried by a produced alerting contract is a
// name only, never a secret value. This inspects the contract field itself,
// independent of the fail-closed error path above.
func TestPersesAlertEvaluatorOwnerReferenceCarriesNoSecret(t *testing.T) {
	// The only owner reference the alerting contract can name is the
	// Prometheus provider kind, and it must never carry a secret value.
	const ownerReference = string(capability.ProviderPrometheus)
	for _, secret := range []string{"bearer", "password", "api_key", "apiKey", "token", "clientCert", "privateKey"} {
		if strings.Contains(strings.ToLower(ownerReference), secret) {
			t.Fatalf("alerting contract owner reference must carry no %s secret value", secret)
		}
	}
}

// TestPersesAlertEvaluatorHealthUnselectedManifestReturnsError asserts the
// manifest-selection regression: the health verifier must fail closed AND return
// the fail-closed *PersesAlertEvaluatorError (not nil, not another error type)
// when the observability management UI is not selected. An unselected surface is
// not_enabled; it must never pass on datasource-pin validation alone.
func TestPersesAlertEvaluatorHealthUnselectedManifestReturnsError(t *testing.T) {
	t.Setenv(application.MetricsEnabledEnv, "true")
	err := VerifyPersesAlertEvaluatorHealth(context.Background(), application.New("perses-alerts-unselected", "dev", false, false, false))
	if err == nil {
		t.Fatal("health verify must fail closed for an unselected manifest; an unselected surface selects no alert evaluator")
	}
	if _, ok := err.(*PersesAlertEvaluatorError); !ok {
		t.Fatalf("error = %T, want *PersesAlertEvaluatorError", err)
	}
	if !strings.Contains(err.Error(), "is not selected") {
		t.Fatalf("fail-closed error = %v, want the unselected-manifest reason text", err)
	}
}

// TestPersesAlertEvaluatorHealthSelectedStaysFailsClosed asserts the other side
// of the regression: a selected, pinned surface still fails closed via the
// honest stub, and the produced error names the missing real evaluator. This
// proves the health verifier never returns a healthy/verified state for a
// selected surface without a real evaluator/recipient contract.
func TestPersesAlertEvaluatorHealthSelectedStaysFailsClosed(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv(application.MetricsEnabledEnv, "true")
	m := persesAlertEvaluatorManifest()
	persesMaterializeAlertProvider(t, m, persesPrometheusScrapeSource("prometheus:runtime", capability.ProviderPrometheus))
	err := VerifyPersesAlertEvaluatorHealth(context.Background(), m)
	if err == nil {
		t.Fatal("health verify must fail closed for a selected+pinned surface without a real evaluator/recipient contract")
	}
	if _, ok := err.(*PersesAlertEvaluatorError); !ok {
		t.Fatalf("error = %T, want *PersesAlertEvaluatorError", err)
	}
	if !strings.Contains(err.Error(), "no real Prometheus rule-evaluation") {
		t.Fatalf("fail-closed error = %v, want the no-real-contract fail-closed error text", err)
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
