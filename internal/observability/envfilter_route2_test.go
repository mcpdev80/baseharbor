package observability

import (
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

// envfilter_route2_test.go — regression for card 0428-121 (route 2 /
// caller-agnostic). The environment filter acts inside List on the source's OWN
// derivable environment only. An app-scoped source without an explicit or
// unambiguously derivable environment must fail closed (invisible to an
// app-scoped caller) — not leak into another environment. A shared/platform
// source without an environment stays visible (genuinely environment-agnostic).

// Route-2 fail-closed: an app-scoped metrics source whose ID does NOT carry the
// application:metrics: grammar (no derivable environment) must not appear in an
// app-scoped caller's result.
func TestRoute2AppSourceWithoutEnvironmentFailsClosed(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())

	if err := Update(MetricsSource{
		ID:               "demo-metrics", // bare ID, no environment grammar
		Provider:         capability.ProviderPrometheus,
		Class:            SourceApplication,
		Scope:            capability.ScopeApplication,
		OwnerApplication: "demo",
		Network:          "baseharbor-demo-metrics",
		Target:           "bhm-abc:9464",
		Path:             "/metrics",
		Scheme:           "http",
		Verification:     capability.ObservabilityVerifyBackend,
	}); err != nil {
		t.Fatal(err)
	}

	placement := capability.ProviderPlacement{Scope: capability.ScopeApplication, Ownership: capability.OwnershipBaseHarbor}
	listed, err := ListApplicationMetrics(placement, []string{"demo"})
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 0 {
		t.Fatalf("fail-closed violated: unresolvable app-scoped source visible (%d sources)", len(listed))
	}
}

// Route-2 fail-closed on a provider too: a bare-project-name application
// provider (no canonical bh-<target>-<app>-<env> grammar) fails closed.
func TestRoute2ProviderSourceWithoutEnvironmentFailsClosed(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())

	if err := UpdateSignal(SignalSource{
		ID:               "loki:legacy",
		Kind:             SignalLogs,
		Provider:         capability.ProviderLoki,
		Class:            SourceApplicationProvider,
		Scope:            capability.ScopeApplication,
		OwnerApplication: "demo",
		Network:          "baseharbor-demo-logs",
		Target:           "loki:3100",
		Protocol:         "syslog-rfc5424",
	}); err != nil {
		t.Fatal(err)
	}

	placement := capability.ProviderPlacement{Scope: capability.ScopeApplication}
	listed, err := ListLogs(placement, []string{"demo"}, true, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 0 {
		t.Fatalf("fail-closed violated: unresolvable app-provider source visible (%d sources)", len(listed))
	}
}

// Route-2 success path: an app-scoped metrics source that DOES carry a derivable
// environment (application:metrics: grammar) is visible to an app-scoped caller.
func TestRoute2AppSourceWithEnvironmentVisible(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())

	if err := Update(MetricsSource{
		ID:               ApplicationMetricsSourceID("demo", "prod", "web"),
		Provider:         capability.ProviderPrometheus,
		Class:            SourceApplication,
		Scope:            capability.ScopeApplication,
		OwnerApplication: "demo",
		Network:          "baseharbor-demo-prod-metrics",
		Target:           "bhm-abc:9464",
		Path:             "/metrics",
		Scheme:           "http",
		Verification:     capability.ObservabilityVerifyBackend,
	}); err != nil {
		t.Fatal(err)
	}

	placement := capability.ProviderPlacement{Scope: capability.ScopeApplication, Ownership: capability.OwnershipBaseHarbor}
	listed, err := ListApplicationMetrics(placement, []string{"demo"})
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].ID != "application:metrics:demo:prod:web" {
		t.Fatalf("app-scoped source with derivable env must be visible, got %d", len(listed))
	}
}

// Route-2 caller-agnostic visibility on the shared-platform caller path: a
// shared/platform source without an environment boundary is genuinely
// environment-agnostic and stays visible. The fail-closed guard only applies to
// app-scoped callers; a shared caller keeps every allowed source.
func TestRoute2SharedSourceWithoutEnvironmentStaysVisible(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())

	if err := UpdateSignal(SignalSource{
		ID:               "shared-default",
		Kind:             SignalMetrics,
		Provider:         capability.ProviderTempo,
		Class:            SourcePlatformProvider,
		Scope:            capability.ScopeShared,
		Network:          "trace",
		Target:           "tempo:3200",
		Path:             "/metrics",
		Protocol:         "openmetrics",
		Verification:     capability.ObservabilityVerifyBackend,
	}); err != nil {
		t.Fatal(err)
	}

	placement := capability.ProviderPlacement{Scope: capability.ScopeShared, Ownership: capability.OwnershipBaseHarbor}
	listed, err := List(SignalMetrics, placement, nil, true, false, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 {
		t.Fatalf("shared environment-agnostic source must stay visible on List, got %d", len(listed))
	}
}

// Route-2 hyphenated environment collision: a legacy project name ending in a
// reserved token (or a hyphenated trailing token such as "prod-west") is
// ambiguous and must fail closed, never resolve to a single environment.
func TestRoute2HyphenatedEnvironmentFailsClosed(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())

	if err := UpdateSignal(SignalSource{
		ID:               "bh-local-demo-prod-west",
		Kind:             SignalMetrics,
		Provider:         capability.ProviderPrometheus,
		Class:            SourceApplication,
		Scope:            capability.ScopeApplication,
		OwnerApplication: "demo",
		Network:          "net-bh-local-demo-prod-west",
		Target:           "bhm-abc:9464",
		Protocol:         "openmetrics",
		Path:             "/metrics",
		Verification:     capability.ObservabilityVerifyBackend,
	}); err != nil {
		t.Fatal(err)
	}

	placement := capability.ProviderPlacement{Scope: capability.ScopeApplication}
	listed, err := ListApplicationMetrics(placement, []string{"demo"})
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 0 {
		t.Fatalf("hyphenated/ambiguous env must fail closed, got %d sources", len(listed))
	}
}

// Route-2 reserved project token (e.g. "prometheus") fails closed, not to an
// environment-agnostic visibility.
func TestRoute2ReservedTokenFailsClosed(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())

	if err := UpdateSignal(SignalSource{
		ID:               "bh-local-demo-prometheus",
		Kind:             SignalMetrics,
		Provider:         capability.ProviderPrometheus,
		Class:            SourceApplication,
		Scope:            capability.ScopeApplication,
		OwnerApplication: "demo",
		Network:          "net-bh-local-demo-prometheus",
		Target:           "bhm-abc:9464",
		Protocol:         "openmetrics",
		Path:             "/metrics",
		Verification:     capability.ObservabilityVerifyBackend,
	}); err != nil {
		t.Fatal(err)
	}

	placement := capability.ProviderPlacement{Scope: capability.ScopeApplication}
	listed, err := ListApplicationMetrics(placement, []string{"demo"})
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 0 {
		t.Fatalf("reserved-token source leaked, got %d sources", len(listed))
	}
}
