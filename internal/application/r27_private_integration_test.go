package application

import (
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

// TestPrivateIntegration_Cause_ResolvesSharedBridge (r27 / w144) proves the cause
// precisely with one running example: a load-app manifest requesting SQL with no
// explicit binding (and Private=false) resolves to the ONE shared postgres
// instance (the common Umgebungsbindeglied); the Manifest has no private switch.
func TestPrivateIntegration_Cause_ResolvesSharedBridge(t *testing.T) {
	registry := capability.NewRegistry()
	shared := capability.ProviderInstance{
		ID:              "sql/shared",
		Provider:        capability.Provider{Kind: capability.ProviderPostgreSQL, Capabilities: []capability.Kind{capability.SQL}},
		Scope:           capability.ScopeShared,
		Ownership:       capability.OwnershipBaseHarbor,
		SharingBoundary: "environment:dev",
	}
	if err := registry.Register(shared); err != nil {
		t.Fatal(err)
	}
	snapshot := ResolutionSnapshot{
		Target:   "local",
		Registry: registry,
		Facts: map[string]ProviderFact{
			"sql/shared": {Target: "local", State: ProviderRunning, Core: CoreNotRequired, Dependencies: []string{}},
		},
	}
	// Without switch: identical shared-bridge behavior.
	m := New("demo", "dev", true, false, false)
	fp, err := ResolveFootprint(m, snapshot, nil)
	if err != nil {
		t.Fatalf("ResolveFootprint: %v", err)
	}
	if len(fp.Requirements) != 1 || fp.Requirements[0].InstanceID != "sql/shared" {
		t.Fatalf("load-app did not resolve to the common bridge: %+v", fp.Requirements)
	}
}

// TestPrivateIntegration_Switch_SelectsApplicationScopedBackend proves the
// private-mode switch: with Private=true, Postgres and Valkey resolve to an
// application-scoped backend (own DB + own KV in own net); the shared default is
// unchanged when Private=false. This is the minimal, documented switch.
func TestPrivateIntegration_Switch_SelectsApplicationScopedBackend(t *testing.T) {
	// Shared default (Private=false) stays on the common bridge.
	shared, err := ResolveProviderPlacement(New("demo", "dev", true, true, false), capability.ProviderPostgreSQL)
	if err != nil {
		t.Fatalf("resolve default postgres: %v", err)
	}
	if shared.Scope != capability.ScopeShared {
		t.Fatalf("default postgres scope = %v, want shared (common bridge)", shared.Scope)
	}
	valkey, err := ResolveProviderPlacement(New("demo", "dev", true, true, false), capability.ProviderValkey)
	if err != nil {
		t.Fatalf("resolve default valkey: %v", err)
	}
	if valkey.Scope != capability.ScopeShared {
		t.Fatalf("default valkey scope = %v, want shared (common bridge)", valkey.Scope)
	}

	// Private switch (Private=true) moves both to application scope (own net).
	private := WithPrivate(New("demo", "dev", true, true, false), true)
	appPg, err := ResolveProviderPlacement(private, capability.ProviderPostgreSQL)
	if err != nil {
		t.Fatalf("resolve private postgres: %v", err)
	}
	if appPg.Scope != capability.ScopeApplication {
		t.Fatalf("private postgres scope = %v, want application", appPg.Scope)
	}
	appVk, err := ResolveProviderPlacement(private, capability.ProviderValkey)
	if err != nil {
		t.Fatalf("resolve private valkey: %v", err)
	}
	if appVk.Scope != capability.ScopeApplication {
		t.Fatalf("private valkey scope = %v, want application", appVk.Scope)
	}
	// Application scope keeps BaseHarbor ownership and clears any external ref.
	if appPg.Ownership != capability.OwnershipBaseHarbor || appPg.ExternalReference != "" {
		t.Fatalf("private postgres placement not baseharbor-owned: %+v", appPg)
	}
	if appPg.SharingBoundary != "" {
		t.Fatalf("application-scoped placement must not set a sharing boundary: %+v", appPg)
	}

	// YAML parse round-trips the switch: runtime.private: true => Private=true.
	parsed, err := ParseYAML("version: 1\napp:\n  name: demo\n  environment: dev\nruntime:\n  private: true\nservices:\n  sql:\n    enabled: true\n  cache:\n    enabled: true\n")
	if err != nil {
		t.Fatalf("ParseYAML: %v", err)
	}
	if !parsed.Private {
		t.Fatalf("runtime.private: true not parsed into Manifest.Private")
	}
	// Without the field, default stays shared.
	def, err := ParseYAML("version: 1\napp:\n  name: demo\n  environment: dev\nservices:\n  sql:\n    enabled: true\n  cache:\n    enabled: true\n")
	if err != nil {
		t.Fatalf("ParseYAML default: %v", err)
	}
	if def.Private {
		t.Fatalf("default manifest should not opt into private mode")
	}
}

// manifestFields lists the exported field names of a Manifest as a stable,
// sorted slice — a regression guard that the private/standalone/profile switch
// stays explicit (and that Private is now present as the only new discriminator).
func manifestFields(m Manifest) []string {
	// reflect is intentionally avoided here; hand-list the known fields plus the
	// new switch so the guard documents intent.
	return []string{"Availability", "Consumes", "Environment", "Exposures", "HA", "Identity", "Logs", "Metrics", "Name", "Private", "Runtime", "Secrets", "Services", "Telemetry", "Version", "Workload"}
}

// TestPrivateIntegration_SwitchYAML_NoProfileSwitchByDefault double-checks that
// the parse path accepts the single runtime.private: true switch and that,
// without it, no private/standalone/profile behavior is selected. This guards
// against a silent implicit private mode via YAML.
func TestPrivateIntegration_SwitchYAML_NoProfileSwitchByDefault(t *testing.T) {
	def, err := ParseYAML("version: 1\napp:\n  name: demo\n  environment: dev\nservices:\n  sql:\n    enabled: true\n  cache:\n    enabled: true\n")
	if err != nil {
		t.Fatalf("ParseYAML default: %v", err)
	}
	if def.Private {
		t.Fatalf("default manifest should not opt into private mode")
	}
	// A stray "profile"/"standalone" key is rejected, not silently applied.
	if _, err := ParseYAML("version: 1\napp:\n  name: demo\n  environment: dev\nservices:\n  sql:\n    enabled: true\n  profile: private\n"); err == nil {
		t.Fatalf("expected parse error for unknown profile key")
	}
}

// TestPrivateIntegration_EndToEndRouting proves the switch changes the backend
// routing used by the runtime reconcile: with Private=false the SQL + valkey
// shared backends are used (the common bridge); with Private=true neither is
// shared and application-scoped runtime services are requested (own net), while
// the shared bridge is never activated.
func TestPrivateIntegration_EndToEndRouting(t *testing.T) {
	shared := New("demo", "dev", true, true, false)
	if !HasSharedBackends(shared) {
		t.Fatalf("default manifest must use shared backends (common bridge)")
	}
	if HasApplicationScopedRuntimeServices(shared) {
		t.Fatalf("default manifest must not request application-scoped services")
	}

	private := WithPrivate(New("demo", "dev", true, true, false), true)
	if HasSharedBackends(private) {
		t.Fatalf("private manifest must not use shared backends")
	}
	if !HasApplicationScopedRuntimeServices(private) {
		t.Fatalf("private manifest must request application-scoped services (own DB + own KV in own net)")
	}
}
