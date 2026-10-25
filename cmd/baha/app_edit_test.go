package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
)

func TestAppEditCommandHydratesExactSourceAndNoCancelWrites(t *testing.T) {
	requirement := capability.Requirement{Kind: capability.SQL, Name: "primary"}
	manifest := application.WithSQLInstances(application.New("edit-safe", "dev", false, false, false), "primary")
	applicationID := manifest.ApplicationID
	source := []byte("# keep this comment exactly\n" + manifest.AuthoredYAML())
	read := func(context.Context, string) (GuidedReadContext, error) {
		return GuidedReadContext{
			ApplicationID:    applicationID,
			Name:             "edit-safe",
			Environment:      "dev",
			Target:           "dev",
			Capabilities:     []capability.Requirement{requirement},
			Footprint:        application.Footprint{Version: application.FootprintVersion, Target: "dev"},
			SnapshotRevision: "snapshot-1",
			Complete:         true,
		}, nil
	}
	mutations := 0
	authorities := appEditAuthorities{
		Read: read,
		Choices: func(context.Context, string, []capability.Requirement) ([]appProviderChoice, error) {
			return nil, nil
		},
		Plan: func(context.Context, string, []capability.Requirement, []application.ProviderPreference) (application.ProviderImpactPlan, error) {
			return application.ProviderImpactPlan{Version: application.ProviderImpactVersion, Target: "dev", Revision: "r", PlanID: "p", ConfirmationsRequired: 1}, nil
		},
		Preview: func(context.Context, appEditDesired) (GuidedReadContext, error) {
			return GuidedReadContext{ApplicationID: applicationID, Target: "dev", Footprint: application.Footprint{Version: application.FootprintVersion, Target: "dev"}}, nil
		},
		Authorize: func(context.Context, string, string) error { return nil },
		Execute: func(context.Context, appEditDesired, application.ProviderImpactPlan, application.ProviderChangeConsent) ([]application.ProviderExecutionOutcome, error) {
			mutations++
			return nil, nil
		},
	}
	session, err := appEditStart(context.Background(), "dev", source, application.ResolutionSnapshot{Target: "dev", Registry: capability.NewRegistry()}, authorities)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(session.Loaded.Source, source) || !reflect.DeepEqual(session.Loaded.Capabilities, []capability.Requirement{requirement}) {
		t.Fatalf("hydration changed source or named requirements: %+v", session.Loaded)
	}
	cancelled, err := session.Controller.Step(context.Background(), appWizardCancel)
	if !errors.Is(err, errAppWizardAbort) {
		t.Fatalf("cancel error = %v", err)
	}
	session.Controller = cancelled
	result, err := runAppEditCommand(context.Background(), session, nil, nil, authorities)
	if !errors.Is(err, errAppWizardAbort) || mutations != 0 || !bytes.Equal(result.Source, source) {
		t.Fatalf("cancel wrote state: err=%v calls=%d result=%+v", err, mutations, result)
	}
}

func TestAppEditCommandRepeatedNoRequiresFreshPreviewBeforeConsent(t *testing.T) {
	requirement := capability.Requirement{Kind: capability.SQL, Name: "primary"}
	loaded := appEditTestLoaded(t, "dev", []capability.Requirement{requirement}, nil)
	plan := application.ProviderImpactPlan{Version: application.ProviderImpactVersion, Target: "dev", Revision: "r", PlanID: "plan-current", ConfirmationsRequired: 1}
	refreshes := 0
	controller, err := newAppWizardController(context.Background(), appWizardInput{
		Manifest:  mustAppEditManifest(t, loaded),
		Snapshot:  application.ResolutionSnapshot{Target: "dev", Registry: capability.NewRegistry()},
		Selection: appCapabilitySelectionInput{Current: []capability.Requirement{requirement}},
		TTY:       true,
		Refresh: func(context.Context, appWizardDecision) (*GuidedReadContext, *application.ProviderImpactPlan, error) {
			refreshes++
			view := &GuidedReadContext{ApplicationID: loaded.ApplicationID, Target: "dev", Footprint: application.Footprint{Version: application.FootprintVersion, Target: "dev"}}
			copyPlan := plan
			return view, &copyPlan, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	controller, err = controller.Step(context.Background(), appWizardNext)
	if err != nil {
		t.Fatal(err)
	}
	controller, err = controller.Step(context.Background(), appWizardNo)
	if err != nil {
		t.Fatal(err)
	}
	controller, err = controller.Step(context.Background(), appWizardNext)
	if err != nil {
		t.Fatal(err)
	}
	controller, err = controller.Step(context.Background(), appWizardNo)
	if err != nil {
		t.Fatal(err)
	}
	controller, err = controller.Step(context.Background(), appWizardNext)
	if err != nil {
		t.Fatal(err)
	}
	controller, err = controller.Step(context.Background(), appWizardYes)
	if err != nil {
		t.Fatal(err)
	}
	if refreshes < 4 {
		t.Fatalf("repeated No did not force fresh previews: %d", refreshes)
	}
}

func mustAppEditManifest(t *testing.T, loaded appEditLoaded) application.Manifest {
	t.Helper()
	manifest, err := appEditParseSource(loaded)
	if err != nil {
		t.Fatal(err)
	}
	return manifest
}

func TestAppEditCommandPreviewErrorPreservesCauseWithoutDecision(t *testing.T) {
	for _, cause := range []error{errors.New("preview authority unavailable"), context.DeadlineExceeded} {
		for _, key := range []string{"", "next", "enter"} {
			t.Run(cause.Error()+"/"+key, func(t *testing.T) {
				input := appWizardTestInput(t)
				calls := 0
				input.Refresh = func(context.Context, appWizardDecision) (*GuidedReadContext, *application.ProviderImpactPlan, error) {
					calls++
					return nil, nil, cause
				}
				controller, err := newAppWizardController(context.Background(), input)
				if err != nil {
					t.Fatal(err)
				}
				var output bytes.Buffer
				session, err := driveAppEditWizard(context.Background(), appEditSession{Controller: controller},
					bufio.NewReader(strings.NewReader(key+"\nyes\n")), &output)
				if !errors.Is(err, cause) || calls != 1 {
					t.Fatalf("preview cause lost or repeated: err=%v calls=%d", err, calls)
				}
				if _, confirmed := session.Controller.Decision(); confirmed || session.Controller.phase != appWizardNeeds {
					t.Fatal("failed preview created a decision or changed navigation")
				}
				if !reflect.DeepEqual(session.Controller.manifest, controller.manifest) ||
					strings.Contains(output.String(), "Read-only decision preview (not execution)") {
					t.Fatal("failed preview changed intent or presented an approvable preview")
				}
			})
		}
	}
}
