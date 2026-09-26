package metrics

import (
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
)

func TestProviderProjectFollowsPlacementScope(t *testing.T) {
	m := application.New("demo", "dev", false, false, false)

	t.Setenv(application.ProviderScopeEnv(capability.ProviderPrometheus), "shared")
	shared, err := PlacementForAt(t.TempDir(), "ci", m)
	if err != nil {
		t.Fatal(err)
	}
	if shared.Project != "bh-ci-shared" {
		t.Fatalf("shared project = %q", shared.Project)
	}

	t.Setenv(application.ProviderScopeEnv(capability.ProviderPrometheus), "application")
	app, err := PlacementForAt(t.TempDir(), "ci", m)
	if err != nil {
		t.Fatal(err)
	}
	if app.Project != "bh-ci-demo" {
		t.Fatalf("application project = %q", app.Project)
	}
}
