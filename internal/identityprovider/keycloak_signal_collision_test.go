package identityprovider

import (
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/observability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// TestKeycloakSignalIDCollisionOnNormalizationBoundary is the regression the
// card exists for. Two DISTINCT application names that both normalize through
// projectToken (the "." -> "-" rule) to the SAME production files.Project build
// the SAME registrationID "keycloak:" + files.Project, so the second
// RegisterProviderSignals overwrites the first (dedupe by (ID, provider),
// registry.go). This test is RED on the unguarded baseline and GREEN only after
// the fix that keeps distinct Keycloak projects distinct.
//
// Fail-closed contract (the form this lane implements): the second
// registration for a registrationID already held by a DIFFERENT owner MUST
// fail, so the shadowing owner never silently vanishes. No new ID formula, no
// database query in the hot path.
func TestKeycloakSignalIDCollisionOnNormalizationBoundary(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())

	build := func(name string) application.Manifest {
		app := keycloakSignalTestApp(t, name, true)
		return app
	}
	// Two names that collapse to one production project via projectToken.
	appA := build("my.identity")
	appB := build("my-identity")

	// Same production files.Project for both (the "." / "-" normalization seam).
	projectA := bhruntime.ApplicationProjectName("cluster", "my.identity-prod", "prod")
	projectB := bhruntime.ApplicationProjectName("cluster", "my-identity-prod", "prod")
	if projectA != projectB {
		t.Fatalf("precondition failed: test names must collapse to one production files.Project, got %q vs %q", projectA, projectB)
	}

	// A's first registration succeeds.
	rA := &localKeycloakRealization{app: appA, namespace: "target", files: KeycloakFiles{Project: projectA, ConsumerNetwork: "baseharbor-identity"}}
	if err := rA.registerKeycloakSignals(); err != nil {
		t.Fatalf("register A: %v", err)
	}

	// B's registration for a colliding project must be REJECTED (fail-closed),
	// so A's source is never overwritten.
	rB := &localKeycloakRealization{app: appB, namespace: "target", files: KeycloakFiles{Project: projectB, ConsumerNetwork: "baseharbor-identity"}}
	if err := rB.registerKeycloakSignals(); err == nil {
		t.Fatalf("second registration for a colliding project must fail (fail-closed); got no error")
	}

	// A's source survives and keeps its owner.
	sources, err := observability.ListAll()
	if err != nil {
		t.Fatalf("ListAll: %v", err)
	}
	found := 0
	for _, s := range sources {
		if s.Provider == capability.ProviderKeycloak && s.ID == keycloakMetricsSignalID(projectA) {
			found++
			if s.OwnerApplication != appA.Name {
				t.Fatalf("expected owner %q, got %q", appA.Name, s.OwnerApplication)
			}
		}
	}
	if found != 1 {
		t.Fatalf("expected exactly one surviving Keycloak source for app A, got %d (sources=%#v)", found, sources)
	}

	// Regression guard: non-colliding names stay distinct (the fault class spans
	// only the "." / "-" boundary).
	for _, pair := range []struct{ a, b string }{
		{"alpha", "beta"},
		{"distinct-a", "distinct-b"},
	} {
		pa := bhruntime.ApplicationProjectName("cluster", pair.a, "prod")
		pb := bhruntime.ApplicationProjectName("cluster", pair.b, "prod")
		if pa == pb {
			t.Fatalf("regression guard: %q and %q should NOT collapse", pair.a, pair.b)
		}
	}
}
