package identityprovider

import (
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/observability"
)

// keycloakMetricsSignalID mirrors the native openmetrics signal declared for the
// Keycloak provider in capability/reference.go; the witness reads this ID.
const keycloakMetricsSignalUnitTestID = "keycloak-metrics"

func keycloakSignalTestApp(t *testing.T, metricsEnabled bool) application.Manifest {
	t.Helper()
	app := application.New("alpha", "dev", false, false, false)
	app = application.WithIdentity(app)
	if metricsEnabled {
		app.Metrics.Sources = []application.MetricsSourceRequirement{{Name: "app", Service: "api", Port: 8080, Path: "/metrics"}}
	} else {
		t.Setenv("BASEHARBOR_METRICS_ENABLED", "false")
	}
	return app
}

// TestKeycloakSignalRegistrationRegistersNativeMetricsSignal proves the positive
// path: with the identity provider selected and metrics collection enabled,
// registerKeycloakSignals writes the keycloak-metrics native signal into the
// observability signal registry so the runtime fan-in (and identity
// eligibility gate) can resolve a real Keycloak metrics source.
func TestKeycloakSignalRegistrationRegistersNativeMetricsSignal(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", dir)
	app := keycloakSignalTestApp(t, true)

	r := &localKeycloakRealization{
		app:       app,
		namespace: "target",
		files:     KeycloakFiles{Project: "identity-provider", ConsumerNetwork: "baseharbor-identity"},
	}
	if err := r.registerKeycloakSignals(); err != nil {
		t.Fatalf("registerKeycloakSignals: %v", err)
	}

	placement, err := application.ResolveProviderPlacement(app, capability.ProviderKeycloak)
	if err != nil {
		t.Fatalf("ResolveProviderPlacement: %v", err)
	}
	sources, err := observability.List(
		observability.SignalMetrics,
		placement,
		[]string{app.Name},
		false,
		true,
		true,
	)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	found := false
	for _, source := range sources {
		if source.Provider != capability.ProviderKeycloak || source.ID != keycloakMetricsSignalUnitTestID {
			continue
		}
		found = true
		if source.Kind != observability.SignalMetrics || source.Protocol != "openmetrics" || source.Path != "/metrics" {
			t.Fatalf("registered keycloak-metrics source mismatch: %#v", source)
		}
	}
	if !found {
		t.Fatalf("keycloak-metrics native signal not registered; sources = %#v", sources)
	}
}

// TestKeycloakSignalRegistrationFailsClosedWithoutMetrics proves the fail-closed
// regression: with metrics collection disabled, registerKeycloakSignals must not
// register any keycloak-metrics source, so every identity gate stays unavailable
// (no regression of the 080 non-forgeability invariant).
func TestKeycloakSignalRegistrationFailsClosedWithoutMetrics(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", dir)
	app := keycloakSignalTestApp(t, false)

	r := &localKeycloakRealization{
		app:       app,
		namespace: "target",
		files:     KeycloakFiles{Project: "identity-provider", ConsumerNetwork: "baseharbor-identity"},
	}
	if err := r.registerKeycloakSignals(); err != nil {
		t.Fatalf("registerKeycloakSignals: %v", err)
	}

	placement, err := application.ResolveProviderPlacement(app, capability.ProviderKeycloak)
	if err != nil {
		t.Fatalf("ResolveProviderPlacement: %v", err)
	}
	sources, err := observability.List(
		observability.SignalMetrics,
		placement,
		[]string{app.Name},
		false,
		true,
		true,
	)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, source := range sources {
		if source.Provider == capability.ProviderKeycloak && source.ID == keycloakMetricsSignalUnitTestID {
			t.Fatalf("keycloak-metrics signal registered while metrics collection is disabled (fail-closed broken): %#v", source)
		}
	}
}
