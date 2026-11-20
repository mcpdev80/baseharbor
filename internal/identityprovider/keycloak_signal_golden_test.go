package identityprovider

import (
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/observability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// TestKeycloakSignalIDGoldenSingleTokenStable is the backward-compat guard.
// Two single-token application names (no "." / "-" boundary) keep their EXACT
// registrationID after a second registration - a golden test that must stay
// GREEN even if the ID formula changes, and that goes RED if the formula is
// rewritten so the existing distinct names no longer resolve to a stable
// identity. It also proves the common case still lets two distinct owners
// coexist.
func TestKeycloakSignalIDGoldenSingleTokenStable(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())

	build := func(name string) application.Manifest {
		app := keycloakSignalTestApp(t, name, true)
		return app
	}
	appA := build("alpha")
	appB := build("beta")

	projectA := bhruntime.ApplicationProjectName("cluster", "alpha", "prod")
	projectB := bhruntime.ApplicationProjectName("cluster", "beta", "prod")
	goldenA := keycloakMetricsSignalID(projectA)
	goldenB := keycloakMetricsSignalID(projectB)
	if goldenA == goldenB {
		t.Fatalf("golden precondition: distinct single-token names must not collapse to one registrationID")
	}

	rA := &localKeycloakRealization{app: appA, namespace: "target", files: KeycloakFiles{Project: projectA, ConsumerNetwork: "baseharbor-identity"}}
	if err := rA.registerKeycloakSignals(); err != nil {
		t.Fatalf("register A: %v", err)
	}
	rB := &localKeycloakRealization{app: appB, namespace: "target", files: KeycloakFiles{Project: projectB, ConsumerNetwork: "baseharbor-identity"}}
	if err := rB.registerKeycloakSignals(); err != nil {
		t.Fatalf("register B: %v", err)
	}

	sources, err := observability.ListAll()
	if err != nil {
		t.Fatalf("ListAll: %v", err)
	}
	seenA, seenB := false, false
	for _, s := range sources {
		switch s.ID {
		case goldenA:
			seenA = true
			if s.OwnerApplication != appA.Name {
				t.Fatalf("golden A owner changed: %q", s.OwnerApplication)
			}
		case goldenB:
			seenB = true
			if s.OwnerApplication != appB.Name {
				t.Fatalf("golden B owner changed: %q", s.OwnerApplication)
			}
		}
	}
	if !seenA {
		t.Fatalf("golden A id %q vanished after B registration", goldenA)
	}
	if !seenB {
		t.Fatalf("golden B id %q vanished after A registration", goldenB)
	}
}
