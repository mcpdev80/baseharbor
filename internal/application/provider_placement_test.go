package application

import (
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
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

func TestResolveProviderPlacementRejectsUnsupportedOverride(t *testing.T) {
	t.Setenv(ProviderScopeEnv(capability.ProviderPostgreSQL), "shared")
	_, err := ResolveProviderPlacement(
		Manifest{Name: "demo", Environment: "dev"},
		capability.ProviderPostgreSQL,
	)
	if err == nil || !strings.Contains(err.Error(), "does not support placement scope") {
		t.Fatalf("expected unsupported placement error, got %v", err)
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
	if err == nil || !strings.Contains(err.Error(), "named shared boundaries are implemented for Prometheus only") {
		t.Fatalf("expected unsupported adapter boundary rejection, got %v", err)
	}
}
