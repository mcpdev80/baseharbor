package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/machine"
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

func TestAppEditCommandPreviewUsesOneFreshSnapshotPerDecision(t *testing.T) {
	loaded := appEditTestLoaded(t, "dev", nil, nil)
	reads, plans := 0, 0
	view := GuidedReadContext{ApplicationID: loaded.ApplicationID, Name: "edit-safe", Environment: "dev", Target: "dev",
		Footprint: application.Footprint{Version: application.FootprintVersion, Target: "dev"}}
	authorities := appEditAuthorities{
		Read: func(context.Context, string) (GuidedReadContext, error) {
			reads++
			view.SnapshotRevision = fmt.Sprint(reads)
			return view, nil
		},
		Choices: func(context.Context, string, []capability.Requirement) ([]appProviderChoice, error) { return nil, nil },
		Plan: func(context.Context, string, []capability.Requirement, []application.ProviderPreference) (application.ProviderImpactPlan, error) {
			plans++
			return application.ProviderImpactPlan{Version: application.ProviderImpactVersion, Target: "dev", Revision: view.SnapshotRevision, PlanID: view.SnapshotRevision}, nil
		},
		Preview: func(context.Context, appEditDesired) (GuidedReadContext, error) { return view, nil },
	}
	session, err := appEditStart(context.Background(), "dev", loaded.Source,
		application.ResolutionSnapshot{Target: "dev", Registry: capability.NewRegistry()}, authorities)
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	session, err = driveAppEditWizard(context.Background(), session,
		bufio.NewReader(strings.NewReader("enter\nno\nenter\ncancel\n")), &output)
	if !errors.Is(err, errAppWizardAbort) || reads != 3 || plans != 2 {
		t.Fatalf("each preview needs one new read: reads=%d plans=%d err=%v\n%s", reads, plans, err, output.String())
	}
	if session.Controller.impact == nil || session.Controller.impact.PlanID != "3" {
		t.Fatal("second preview reused the first decision's snapshot")
	}
}

func TestAppEditCommandPreviewDiagnosticKeepsPublicCauseCodeWithoutDetails(t *testing.T) {
	for _, stage := range []string{"provider choices", "provider plan"} {
		t.Run(stage, func(t *testing.T) {
			cause := &machine.Error{Code: machine.ErrorValidationFailed, CauseCode: "invalid-impact-plan",
				Message: "private-message-value", Cause: errors.New("private-inner-value")}
			a := appEditAuthorities{
				Choices: func(context.Context, string, []capability.Requirement) ([]appProviderChoice, error) {
					if stage == "provider choices" {
						return nil, cause
					}
					return nil, nil
				},
				Plan: func(context.Context, string, []capability.Requirement, []application.ProviderPreference) (application.ProviderImpactPlan, error) {
					return application.ProviderImpactPlan{}, cause
				},
			}
			view := GuidedReadContext{Target: "dev", SnapshotRevision: "fresh", Footprint: application.Footprint{Target: "dev"}}
			_, err := appEditPropose(context.Background(), appMachineDesiredRequest{Target: "dev", SnapshotRevision: "fresh"}, view, a)
			var classified *appMachineError
			if !errors.As(err, &classified) || classified.Code != "denied" ||
				!strings.Contains(err.Error(), stage+" (invalid-impact-plan)") || strings.Contains(err.Error(), "private-") {
				t.Fatalf("unsafe or missing diagnostic: %v", err)
			}
		})
	}
}
