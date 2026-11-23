package main

import (
	"context"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
)

// TestReview918SpaceToggleAcknowledgesOnScreen confirms the needs screen echoes
// a one-line status for a space toggle so the operator sees the new selected
// state, for both the select and deselect direction.
func TestReview918SpaceToggleAcknowledgesOnScreen(t *testing.T) {
	req := capability.Requirement{Kind: capability.SQL, Name: "primary"}
	loaded := appEditTestLoaded(t, "dev", []capability.Requirement{req}, nil)
	manifest := mustAppEditManifest(t, loaded)
	plan := application.ProviderImpactPlan{Version: application.ProviderImpactVersion, Target: "dev", Revision: "r", PlanID: "p", Complete: true}
	build := func() appEditSession {
		controller, err := newAppWizardController(context.Background(), appWizardInput{
			Manifest: manifest, Snapshot: application.ResolutionSnapshot{Target: "dev", Registry: capability.NewRegistry()},
			Selection: appCapabilitySelectionInput{Current: []capability.Requirement{req}}, TTY: true,
			Refresh: func(context.Context, appWizardDecision) (*GuidedReadContext, *application.ProviderImpactPlan, error) {
				view := &GuidedReadContext{ApplicationID: manifest.ApplicationID, Target: "dev", Footprint: application.Footprint{Version: application.FootprintVersion, Target: "dev"}}
				copyPlan := plan
				return view, &copyPlan, nil
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		return appEditSession{Loaded: loaded, Controller: controller}
	}

	// SQL is a detected/selected suggestion initially, so the first space
	// deselects it and must be acknowledged on screen.
	var output strings.Builder
	_, _ = driveAppEditWizard(context.Background(), build(), bufioReader(" \n\nyes\n"), &output)
	if !strings.Contains(output.String(), "database.sql deselected") {
		t.Fatalf("space deselect was not acknowledged on screen: %q", output.String())
	}

	// A fresh session without the need starts unselected; the first space
	// selects it and must be acknowledged on screen.
	freshLoaded := appEditTestLoaded(t, "dev", nil, nil)
	freshManifest := mustAppEditManifest(t, freshLoaded)
	fresh, err := newAppWizardController(context.Background(), appWizardInput{
		Manifest: freshManifest, Snapshot: application.ResolutionSnapshot{Target: "dev", Registry: capability.NewRegistry()},
		Selection: appCapabilitySelectionInput{Current: []capability.Requirement{}}, TTY: true,
		Refresh: func(context.Context, appWizardDecision) (*GuidedReadContext, *application.ProviderImpactPlan, error) {
			view := &GuidedReadContext{ApplicationID: freshManifest.ApplicationID, Target: "dev", Footprint: application.Footprint{Version: application.FootprintVersion, Target: "dev"}}
			copyPlan := plan
			return view, &copyPlan, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var freshOutput strings.Builder
	_, _ = driveAppEditWizard(context.Background(), appEditSession{Loaded: freshLoaded, Controller: fresh}, bufioReader(" \n\nyes\n"), &freshOutput)
	if !strings.Contains(freshOutput.String(), "database.sql selected") {
		t.Fatalf("space select was not acknowledged on screen: %q", freshOutput.String())
	}
}
