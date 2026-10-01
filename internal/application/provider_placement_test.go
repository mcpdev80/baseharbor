package application

import (
	"reflect"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/orgconfig"
)

func TestResolveProviderPlacementUsesSimpleSafeDefault(t *testing.T) {
	placement, err := ResolveProviderPlacement(
		Manifest{Name: "demo", Environment: "dev"},
		capability.ProviderPrometheus,
	)
	if err != nil {
		t.Fatal(err)
	}
	if placement.Scope != capability.ScopeShared ||
		placement.Ownership != capability.OwnershipBaseHarbor ||
		placement.SharingBoundary != "" ||
		placement.ExternalReference != "" {
		t.Fatalf("placement=%#v", placement)
	}
}

func TestResolveProviderPlacementAllowsExplicitSupportedOverride(t *testing.T) {
	t.Setenv(ProviderScopeEnv(capability.ProviderPrometheus), "application")
	placement, err := ResolveProviderPlacement(
		Manifest{Name: "demo", Environment: "dev"},
		capability.ProviderPrometheus,
	)
	if err != nil {
		t.Fatal(err)
	}
	if placement.Scope != capability.ScopeApplication ||
		placement.Ownership != capability.OwnershipBaseHarbor {
		t.Fatalf("placement=%#v", placement)
	}
}

func TestResolveProviderPlacementAllowsSharedBoundary(t *testing.T) {
	t.Setenv(ProviderSharingBoundaryEnv(capability.ProviderPrometheus), "backend-team")
	placement, err := ResolveProviderPlacement(
		Manifest{Name: "demo", Environment: "dev"},
		capability.ProviderPrometheus,
	)
	if err != nil {
		t.Fatal(err)
	}
	if placement.Scope != capability.ScopeShared ||
		placement.SharingBoundary != "backend-team" {
		t.Fatalf("placement=%#v", placement)
	}
}

func TestResolveProviderPlacementDefaultsPostgresAndValkeyToShared(t *testing.T) {
	for _, provider := range []capability.ProviderKind{capability.ProviderPostgreSQL, capability.ProviderValkey} {
		placement, err := ResolveProviderPlacement(
			Manifest{Name: "demo", Environment: "dev"},
			provider,
		)
		if err != nil {
			t.Fatalf("%s placement error = %v", provider, err)
		}
		if placement.Scope != capability.ScopeShared || placement.Ownership != capability.OwnershipBaseHarbor {
			t.Fatalf("%s placement=%#v", provider, placement)
		}
	}
}

func TestResolveProviderPlacementAllowsDedicatedPostgresOverride(t *testing.T) {
	t.Setenv(ProviderScopeEnv(capability.ProviderPostgreSQL), "application")
	placement, err := ResolveProviderPlacement(
		Manifest{Name: "demo", Environment: "dev"},
		capability.ProviderPostgreSQL,
	)
	if err != nil {
		t.Fatal(err)
	}
	if placement.Scope != capability.ScopeApplication {
		t.Fatalf("placement=%#v", placement)
	}
}

func TestResolveProviderPlacementRejectsBoundaryOutsideShared(t *testing.T) {
	t.Setenv(ProviderScopeEnv(capability.ProviderPrometheus), "application")
	t.Setenv(ProviderSharingBoundaryEnv(capability.ProviderPrometheus), "team-a")
	_, err := ResolveProviderPlacement(
		Manifest{Name: "demo", Environment: "dev"},
		capability.ProviderPrometheus,
	)
	if err == nil || !strings.Contains(err.Error(), "cannot define a sharing boundary") {
		t.Fatalf("expected sharing boundary rejection, got %v", err)
	}
}

func TestPrometheusExternalPlacementFailsClosedUntilAdapterExists(t *testing.T) {
	t.Setenv(ProviderScopeEnv(capability.ProviderPrometheus), "external")
	t.Setenv(ProviderExternalReferenceEnv(capability.ProviderPrometheus), "metrics-prod")
	_, err := ResolveProviderPlacement(
		Manifest{Name: "demo", Environment: "production"},
		capability.ProviderPrometheus,
	)
	if err == nil || !strings.Contains(err.Error(), "does not support placement scope") {
		t.Fatalf("expected external placement to fail closed, got %v", err)
	}
}

func TestProviderPlacementNameTokenIsStableAndSafe(t *testing.T) {
	a := ProviderPlacementNameToken("Backend Team / Production")
	b := ProviderPlacementNameToken("Backend Team / Production")
	if a != b {
		t.Fatalf("token is not stable: %q != %q", a, b)
	}
	if strings.ContainsAny(a, " /") {
		t.Fatalf("token contains unsafe separators: %q", a)
	}
}

func TestResolveProviderPlacementRejectsBoundaryWhenAdapterCannotRealizeIt(t *testing.T) {
	t.Setenv(ProviderSharingBoundaryEnv(capability.ProviderOpenBao), "team-a")
	_, err := ResolveProviderPlacement(
		Manifest{Name: "demo", Environment: "dev"},
		capability.ProviderOpenBao,
	)
	if err == nil || !strings.Contains(err.Error(), "is not supported by the current openbao adapter") {
		t.Fatalf("expected unsupported adapter boundary rejection, got %v", err)
	}
}

func TestResolveProviderPlacementUsesOrganizationExternalDefaultWithoutChangingIntent(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	state := orgconfig.ActiveState{
		Resolution: orgconfig.Resolution{
			Source:         orgconfig.Source{Kind: orgconfig.SourceLocal, Location: "/managed/acme/organization.yaml"},
			ResolvedDigest: "sha256:" + strings.Repeat("a", 64),
			Provenance:     "file:///managed/acme/organization.yaml",
		},
		Config: orgconfig.Config{
			APIVersion:   orgconfig.ContractVersion,
			Organization: "acme",
			Providers: map[string]orgconfig.Reference{
				"company-postgres": {Reference: "external-provider:company-postgres"},
			},
			Defaults: orgconfig.EnvironmentDefaults{
				Providers: map[string]orgconfig.ProviderDefault{
					string(capability.SQL): {Provider: "company-postgres", Scope: "external"},
				},
			},
		},
	}
	if err := orgconfig.SaveActive(state); err != nil {
		t.Fatal(err)
	}

	manifest := Manifest{Name: "demo", Environment: "dev"}
	before := manifest
	placement, err := ResolveProviderPlacement(manifest, capability.ProviderPostgreSQL)
	if err != nil {
		t.Fatal(err)
	}
	if placement.Scope != capability.ScopeExternal ||
		placement.Ownership != capability.OwnershipExternal ||
		placement.ExternalReference != "company-postgres" {
		t.Fatalf("placement=%#v", placement)
	}
	if !reflect.DeepEqual(manifest, before) {
		t.Fatalf("organization placement mutated portable application intent: before=%#v after=%#v", before, manifest)
	}
}

func TestExplicitProviderPlacementOverridesOrganizationDefault(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	state := orgconfig.ActiveState{
		Resolution: orgconfig.Resolution{
			Source:         orgconfig.Source{Kind: orgconfig.SourceLocal, Location: "/managed/acme/organization.yaml"},
			ResolvedDigest: "sha256:" + strings.Repeat("b", 64),
			Provenance:     "file:///managed/acme/organization.yaml",
		},
		Config: orgconfig.Config{
			APIVersion:   orgconfig.ContractVersion,
			Organization: "acme",
			Providers: map[string]orgconfig.Reference{
				"company-postgres": {Reference: "external-provider:company-postgres"},
			},
			Defaults: orgconfig.EnvironmentDefaults{
				Providers: map[string]orgconfig.ProviderDefault{
					string(capability.SQL): {Provider: "company-postgres", Scope: "external"},
				},
			},
		},
	}
	if err := orgconfig.SaveActive(state); err != nil {
		t.Fatal(err)
	}
	t.Setenv(ProviderScopeEnv(capability.ProviderPostgreSQL), "application")
	placement, err := ResolveProviderPlacement(Manifest{Name: "demo", Environment: "dev"}, capability.ProviderPostgreSQL)
	if err != nil {
		t.Fatal(err)
	}
	if placement.Scope != capability.ScopeApplication || placement.Ownership != capability.OwnershipBaseHarbor || placement.ExternalReference != "" {
		t.Fatalf("explicit operator placement did not override organization default: %#v", placement)
	}
}
