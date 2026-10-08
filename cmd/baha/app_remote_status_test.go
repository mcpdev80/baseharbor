package main

import (
	"errors"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/targetsession"
)

func TestRemoteApplicationStatusNeverInfersAbsenceFromLocalFiles(t *testing.T) {
	ctx, core, node := remoteApplicationCoreFixture(t)
	resolved := resolvedApplication{Target: node, Manifest: application.New("owned", "dev", false, false, false),
		Store: application.Store{Root: t.TempDir()}}
	for _, bound := range []bool{false, true} {
		selected := ctx
		if bound {
			selected = withCoreAuthority(ctx, core)
		}
		collection, _, err := newResolvedApplicationStatusCollection(selected, resolved)
		if err == nil || collection.result.State == "not_applied" || collection.result.Ready {
			t.Fatal("missing local files established remote absence or readiness", collection.result, err)
		}
		if bound && !errors.Is(err, targetsession.ErrUnavailable) {
			t.Fatal("missing live Connector session did not fail closed", err)
		}
	}
	changed := node
	changed.AccessReference = "different-node"
	resolved.Target = changed
	_, _, err := newResolvedApplicationStatusCollection(withCoreAuthority(ctx, core), resolved)
	var failure *machine.Error
	if !errors.As(err, &failure) || failure.Code != machine.ErrorPolicyDenied {
		t.Fatal("changed Node selection bypassed status binding", err)
	}
}

func TestRemoteProviderObservationsDoNotQualifyApplicationReadiness(t *testing.T) {
	resolved := resolvedApplication{Manifest: application.New("owned", "dev", true, false, false)}
	for _, services := range [][]targetsession.ProjectService{
		nil,
		{{Service: "postgres", Running: true, State: "running", Health: "healthy"}},
		{{Service: "postgres", Running: true, State: "running"}},
		{{Service: "postgres", Running: true, State: "running", Health: "unhealthy"}},
		{{Service: "postgres", State: "exited"}},
	} {
		result := remoteApplicationStatusObservation(resolved, "protected-project", services)
		if result.Ready || !result.HasUnverified() || result.State == "not_applied" || result.State == "stopped" {
			t.Fatal("partial provider observations qualified complete application state", result)
		}
		if len(services) == 1 && (services[0].Health == "unhealthy" || !services[0].Running) && !result.HasFailures() {
			t.Fatal("failed provider reported only unverified", result)
		}
	}
}
