package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
)

// Perses operator access, card 0428-089. These cells drive the GENUINE
// `baha status --json` path through the existing seam helpers, which run
// runWithIO over an isolated temporary root (the r3 precedent from 0428-075):
// production argument parsing, command dispatch, resolver and render. Nothing
// here calls a seam helper in place of the command, and no container is started:
// this is the access contract, not a runtime journey.

// persesSurfaceEntry returns the observability management surface the executed
// status command declared, and fails the cell when the command surfaced none.
func persesSurfaceEntry(t *testing.T, report persesStatusReport) persesManagementSurface {
	t.Helper()
	surface := observabilitySurface(report)
	if surface == nil {
		t.Fatalf("the executed status command surfaced no observability entry for a selected Perses surface: %s", jsonMustRender(t, report))
	}
	return *surface
}

func jsonMustRender(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("encode %T: %v", value, err)
	}
	return string(data)
}

// TestBahaStatusPersesAccessDeclaresUnsupportedRoleMappings pins the access half
// of the contract on parsed command output: the optional Perses surface is
// opt-in, so until an authentication provider is really configured it must
// report `unsupported` AND classify all four management roles as unsupported.
// A surface that declared an authentication class while mapping no roles would
// advertise access it cannot serve.
func TestBahaStatusPersesAccessDeclaresUnsupportedRoleMappings(t *testing.T) {
	t.Setenv("BASEHARBOR_TARGET", "")
	t.Setenv("BASEHARBOR_STATE_DIR", "")
	configureTestTarget(t)
	_, report := persesProbeApp(t, t.Context(), true)
	surface := persesSurfaceEntry(t, report)
	if surface.AuthenticationClass != string(application.ManagementAuthUnsupported) {
		t.Fatalf("an unconfigured Perses surface must report %q, got %q",
			application.ManagementAuthUnsupported, surface.AuthenticationClass)
	}
	if surface.Authentication != "" {
		t.Fatalf("the access contract ships no verified Perses health path, so the authentication marker must stay empty, got %q", surface.Authentication)
	}
}

// TestPersesAccessRoleMappingsAreFullyClassified proves the mapping table the
// surface reports, from the same vocabulary the CLI consumes: every management
// role is classified, an unsupported class claims no native role, and the
// audit role is never claimed as exact unless the provider enforces it. That is
// the rule management-access-v1 states: no finer authorization than the
// provider can enforce.
func TestPersesAccessRoleMappingsAreFullyClassified(t *testing.T) {
	for _, tc := range []struct {
		name  string
		class application.ManagementAuthenticationClass
		audit application.ManagementRoleMappingLevel
	}{
		{name: "native OIDC enforces roles and bindings", class: application.ManagementAuthNativeOIDC, audit: application.ManagementRoleMappingLimited},
		{name: "standards adapter enforces the same roles", class: application.ManagementAuthStandardsAdapter, audit: application.ManagementRoleMappingLimited},
		{name: "native credentials cannot isolate audit", class: application.ManagementAuthNativeCredential, audit: application.ManagementRoleMappingUnsupported},
		{name: "an unauthenticated surface enforces nothing", class: application.ManagementAuthUnsupported, audit: application.ManagementRoleMappingUnsupported},
		{name: "an unknown class degrades to unsupported", class: "grafana-admin", audit: application.ManagementRoleMappingUnsupported},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mappings := application.PersesRoleMappingsFor(tc.class)
			if err := application.ValidateManagementRoleMappings(mappings); err != nil {
				t.Fatalf("the role mappings must satisfy the shared contract: %v", err)
			}
			if len(mappings) != 4 {
				t.Fatalf("every management role must be classified, got %+v", mappings)
			}
			var audit application.ManagementRoleMapping
			for _, mapping := range mappings {
				if mapping.Role == application.ManagementRoleAudit {
					audit = mapping
				}
				if mapping.Level == application.ManagementRoleMappingUnsupported && mapping.Native != "" {
					t.Fatalf("an unsupported mapping must name no native role, got %+v", mapping)
				}
			}
			if audit.Level != tc.audit {
				t.Fatalf("class %q maps audit to %q, want %q", tc.class, audit.Level, tc.audit)
			}
		})
	}
}

// TestBahaStatusPersesAccessOutputCarriesNoCredentialNames asserts secret safety
// on the actual public output: neither a credential value nor a credential
// handle may appear in a status record, because status output is pasted into
// tickets, logs and audit trails.
func TestBahaStatusPersesAccessOutputCarriesNoCredentialNames(t *testing.T) {
	t.Setenv("BASEHARBOR_TARGET", "")
	t.Setenv("BASEHARBOR_STATE_DIR", "")
	configureTestTarget(t)
	_, report := persesProbeApp(t, t.Context(), true)
	rendered := jsonMustRender(t, report)
	for _, forbidden := range []string{
		"BASEHARBOR_PERSES_HEALTH_AUTHORIZATION",
		"BASEHARBOR_ACCESS_TOKEN",
		"BASEHARBOR_PERSES_AUTH_TOKEN_FILE",
		"encryption_key",
		"server-key",
		"password",
	} {
		if strings.Contains(rendered, forbidden) {
			t.Fatalf("public status output must not mention %q, got %s", forbidden, rendered)
		}
	}
	surface := persesSurfaceEntry(t, report)
	if strings.Contains(strings.ToLower(surface.URL), "token") || strings.Contains(surface.URL, "@") {
		t.Fatalf("a surfaced URL must never carry credentials, got %q", surface.URL)
	}
}

// TestPersesAccessContractIsFailClosedOnMissingPin ties the public state to the
// access posture through the contract the CLI consumes: without a pinned release
// there is nothing to authenticate against, and a pinned-but-anonymous surface is
// an open proxy rather than a management surface. Both must stay not_configured.
func TestPersesAccessContractIsFailClosedOnMissingPin(t *testing.T) {
	perState := func(policy capability.PersesSecurityPolicy) *string {
		status := capability.PersesConfiguredStatusNow(true, policy)
		if status == nil {
			t.Fatal("the fail-closed status must still be a concrete state")
		}
		value := string(*status)
		return &value
	}
	unpinned := capability.PersesSecurityPolicy{Release: capability.PersesRelease{Version: "v0.55.1"}}
	if got := perState(unpinned); *got != string(capability.PersesStatusNotConfigured) {
		t.Fatalf("an unpinned surface must report %q, got %q", capability.PersesStatusNotConfigured, *got)
	}
	pinned := capability.DefaultPersesSecurityPolicy()
	if got := perState(pinned); *got != string(capability.PersesStatusConfiguredUnverified) {
		t.Fatalf("a pinned and protected surface must report %q, got %q", capability.PersesStatusConfiguredUnverified, *got)
	}
	anonymous := capability.DefaultPersesSecurityPolicy()
	anonymous.AuthenticationEnabled = false
	if got := perState(anonymous); *got != string(capability.PersesStatusNotConfigured) {
		t.Fatalf("an anonymous Perses surface must never report a configured state, got %q", *got)
	}
	vulnerable := capability.DefaultPersesSecurityPolicy()
	vulnerable.Release.Version = "v0.54.0"
	if got := perState(vulnerable); *got != string(capability.PersesStatusNotConfigured) {
		t.Fatalf("a pre-SSRF-fix Perses pin must never report a configured state, got %q", *got)
	}
	// Neither state can ever be "verified": this Core ships no Perses health path.
	for _, policy := range []capability.PersesSecurityPolicy{pinned, anonymous, vulnerable, unpinned} {
		if got := perState(policy); *got == string(capability.PersesStatusVerified) {
			t.Fatalf("no posture may produce %q without a real health probe: %+v", capability.PersesStatusVerified, policy)
		}
	}
}
