package identityprovider

import (
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/observability"
)

// keycloakMetricsSignalID is the per-project registry identity for the native
// OpenMetrics signal declared in capability/reference.go.
func keycloakMetricsSignalID(project string) string {
	return "keycloak:" + project
}

func keycloakSignalTestApp(t *testing.T, name string, metricsEnabled bool) application.Manifest {
	t.Helper()
	app := application.New(name, "dev", false, false, false)
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
	app := keycloakSignalTestApp(t, "alpha", true)

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
		app.Environment,
	)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	found := false
	for _, source := range sources {
		if source.Provider != capability.ProviderKeycloak || source.ID != keycloakMetricsSignalID(r.files.Project) {
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
	app := keycloakSignalTestApp(t, "alpha", false)

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
		app.Environment,
	)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, source := range sources {
		if source.Provider == capability.ProviderKeycloak && source.ID == keycloakMetricsSignalID(r.files.Project) {
			t.Fatalf("keycloak-metrics signal registered while metrics collection is disabled (fail-closed broken): %#v", source)
		}
	}
}

func TestKeycloakSignalRegistrationKeepsProjectsDistinct(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	apps := []application.Manifest{
		keycloakSignalTestApp(t, "alpha", true),
		keycloakSignalTestApp(t, "beta", true),
	}
	projects := []string{"identity-alpha", "identity-beta"}
	for i, app := range apps {
		r := &localKeycloakRealization{
			app:       app,
			namespace: "target",
			files:     KeycloakFiles{Project: projects[i], ConsumerNetwork: "baseharbor-identity"},
		}
		if err := r.registerKeycloakSignals(); err != nil {
			t.Fatalf("registerKeycloakSignals for %s: %v", app.Name, err)
		}
	}

	ids := make(map[string]bool, len(apps))
	for i, app := range apps {
		placement, err := application.ResolveProviderPlacement(app, capability.ProviderKeycloak)
		if err != nil {
			t.Fatalf("ResolveProviderPlacement for %s: %v", app.Name, err)
		}
		sources, err := observability.List(
			observability.SignalMetrics,
			placement,
			[]string{app.Name},
			false,
			true,
			true,
			app.Environment,
		)
		if err != nil {
			t.Fatalf("List for %s: %v", app.Name, err)
		}
		wantID := keycloakMetricsSignalID(projects[i])
		if ids[wantID] {
			t.Fatalf("project IDs unexpectedly collide: %q", wantID)
		}
		ids[wantID] = true
		found := false
		for _, source := range sources {
			if source.Provider == capability.ProviderKeycloak && source.ID == wantID {
				found = true
			}
		}
		if !found {
			t.Fatalf("Keycloak metrics source for project %q missing after registering both projects: %#v", projects[i], sources)
		}
	}
}
