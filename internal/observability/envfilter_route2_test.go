package observability

import (
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

// envfilter_route2_test.go — regression for card 0428-121 (route 2, FAIL-CLOSED
// fix). SourceMatchesEnvironment must compare the source's derivable environment
// against the caller's requested environment, not just resolve it. The four
// cases below fail open on the route-1 baseline (both the unresolvable leak and
// the same application in prod+stage both passing) and pass with the comparing
// rule. A mutation test pins that the exclusion is structural, not accidental.

// Case A: the same application registered in prod AND stage. List with
// requestedEnvironment "prod" yields only the prod source.
func TestRoute2SameAppProdStageProdCaller(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())

	for _, environment := range []string{"prod", "stage"} {
		source := MetricsSource{
			ID:               ApplicationMetricsSourceID("demo", environment, "web"),
			Provider:         capability.ProviderPrometheus,
			Class:            SourceApplication,
			Scope:            capability.ScopeApplication,
			OwnerApplication: "demo",
			Network:          "baseharbor-demo-" + environment + "-metrics",
			Target:           "bhm-abc:9464",
			Path:             "/metrics",
			Scheme:           "http",
			Verification:     capability.ObservabilityVerifyBackend,
		}
		if err := Update(source); err != nil {
			t.Fatalf("register %s: %v", environment, err)
		}
	}

	prodCaller, err := ListApplicationMetrics(
		capability.ProviderPlacement{Scope: capability.ScopeApplication, Ownership: capability.OwnershipBaseHarbor},
		[]string{"demo"},
		"prod",
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(prodCaller) != 1 || prodCaller[0].ID != "application:metrics:demo:prod:web" {
		t.Fatalf("prod caller must see only the prod source, got %d", len(prodCaller))
	}

	stageCaller, err := ListApplicationMetrics(
		capability.ProviderPlacement{Scope: capability.ScopeApplication, Ownership: capability.OwnershipBaseHarbor},
		[]string{"demo"},
		"stage",
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(stageCaller) != 1 || stageCaller[0].ID != "application:metrics:demo:stage:web" {
		t.Fatalf("stage caller must see only the stage source, got %d", len(stageCaller))
	}
}

// Case B: requestedEnvironment "" excludes every app-scoped source (explicit
// fail-closed) — it never leaks all of them.
func TestRoute2EmptyRequestedEnvironmentFailsClosed(t *testing.T) {
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

	listed, err := ListApplicationMetrics(
		capability.ProviderPlacement{Scope: capability.ScopeApplication, Ownership: capability.OwnershipBaseHarbor},
		[]string{"demo"},
		"",
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 0 {
		t.Fatalf("empty requested environment must fail closed for app sources, got %d", len(listed))
	}
}

// Case C: a provider source with a canonical project name (bh-<target>-<app>-<env>)
// is visible only to the caller whose requested environment matches.
func TestRoute2ProviderEnvMatch(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())

	if err := UpdateSignal(SignalSource{
		ID:               "loki:bh-local-demo-prod",
		Kind:             SignalLogs,
		Provider:         capability.ProviderLoki,
		Class:            SourceApplicationProvider,
		Scope:            capability.ScopeApplication,
		OwnerApplication: "demo",
		Network:          "baseharbor-demo-prod-logs",
		Target:           RuntimeTarget("baseharbor-demo-prod", "loki"),
		Protocol:         "syslog-rfc5424",
	}); err != nil {
		t.Fatal(err)
	}

	prodCaller, err := ListLogs(
		capability.ProviderPlacement{Scope: capability.ScopeApplication},
		[]string{"demo"},
		true, false,
		"prod",
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(prodCaller) != 1 {
		t.Fatalf("prod caller must see the prod provider source, got %d", len(prodCaller))
	}

	devCaller, err := ListLogs(
		capability.ProviderPlacement{Scope: capability.ScopeApplication},
		[]string{"demo"},
		true, false,
		"dev",
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(devCaller) != 0 {
		t.Fatalf("dev caller must not see the prod provider source, got %d", len(devCaller))
	}
}

// Case D: fail-closed for an unresolvable / ambiguous app-scoped source.
func TestRoute2UnresolvableAppSourceFailsClosed(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())

	// bare ID, no environment grammar -> unresolvable
	if err := Update(MetricsSource{
		ID:               "demo-metrics",
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
	// hyphenated legacy project name "prod-west" -> ambiguous -> fails closed
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

	listed, err := ListApplicationMetrics(
		capability.ProviderPlacement{Scope: capability.ScopeApplication, Ownership: capability.OwnershipBaseHarbor},
		[]string{"demo"},
		"prod",
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 0 {
		t.Fatalf("unresolvable/ambiguous app sources must fail closed, got %d", len(listed))
	}
}

// Mutation control: flipping the requested environment of a valid caller must
// drop its source. If the exclusion above were accidental (e.g. a fixed index
// or a constant), mutating the environment would not change the outcome.
func TestRoute2MutationEnvDropExcludes(t *testing.T) {
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

	withMatch, err := ListApplicationMetrics(
		capability.ProviderPlacement{Scope: capability.ScopeApplication, Ownership: capability.OwnershipBaseHarbor},
		[]string{"demo"},
		"prod",
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(withMatch) != 1 {
		t.Fatalf("mutation control baseline: prod caller must see 1 source, got %d", len(withMatch))
	}

	withMismatch, err := ListApplicationMetrics(
		capability.ProviderPlacement{Scope: capability.ScopeApplication, Ownership: capability.OwnershipBaseHarbor},
		[]string{"demo"},
		"stage",
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(withMismatch) != 0 {
		t.Fatalf("mutation control: changing requested env to 'stage' must exclude the prod source, got %d", len(withMismatch))
	}
}

// Case E (shared/platform): an environment-agnostic shared source stays visible
// to a shared caller regardless of the (empty) requested environment.
func TestRoute2SharedSourceStaysVisibleForSharedCaller(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())

	if err := UpdateSignal(SignalSource{
		ID:           "shared-default",
		Kind:         SignalMetrics,
		Provider:     capability.ProviderTempo,
		Class:        SourcePlatformProvider,
		Scope:        capability.ScopeShared,
		Network:      "trace",
		Target:       "tempo:3200",
		Path:         "/metrics",
		Protocol:     "openmetrics",
		Verification: capability.ObservabilityVerifyBackend,
	}); err != nil {
		t.Fatal(err)
	}

	got, err := List(
		SignalMetrics,
		capability.ProviderPlacement{Scope: capability.ScopeShared, Ownership: capability.OwnershipBaseHarbor},
		nil, true, false, true, "",
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("shared environment-agnostic source must stay visible for a shared caller, got %d", len(got))
	}
}
