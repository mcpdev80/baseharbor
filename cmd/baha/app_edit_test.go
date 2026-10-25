package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"path/filepath"
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

func TestAppEditCommandSQLRemovalPreviewDoesNotInheritPersistedPreferences(t *testing.T) {
	r := backendBindingFixture(t, capability.ProviderPostgreSQL)
	prefs := []application.ProviderPreference{{Requirement: capability.Requirement{Kind: capability.SQL, Name: "default"}}}
	if err := application.SaveDesiredProviderPreferencesAt(r.TargetStateRoot, r.Target.Name, r.Manifest, prefs); err != nil {
		t.Fatal(err)
	}
	authorities := defaultAppEditAuthorities(r.Store, nil)
	plan, err := authorities.Plan(context.Background(), r.Target.Name, nil, []application.ProviderPreference{})
	if err != nil || plan.PlanID == "" {
		t.Fatalf("SQL removal inherited its old selection: %v", err)
	}
	desired, err := appEditProjectRequirements(r.Manifest, nil)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := authorities.Preview(context.Background(), appEditDesired{Manifest: desired, Target: r.Target.Name, Preferences: []application.ProviderPreference{}})
	if err != nil || len(preview.Capabilities) != 0 {
		t.Fatalf("removed SQL returned to the preview: capabilities=%v err=%v", preview.Capabilities, err)
	}
	retained, err := application.DesiredProviderPreferencesAt(r.TargetStateRoot, r.Manifest)
	if err != nil || !reflect.DeepEqual(retained, prefs) {
		t.Fatalf("read-only preview changed persisted preferences: %v %v", retained, err)
	}
}

func TestAppEditCommandBoundProviderChoiceRemainsReadOnly(t *testing.T) {
	m := application.WithWorkloadComponents(providerChoiceManifest("dev", "primary"), "api")
	req := capability.Requirement{Kind: capability.SQL, Name: "primary"}
	snapshot := providerChoiceSnapshot("dev", providerChoiceShared("sql-old", "old"), providerChoiceShared("sql-new", "new"))
	resource, err := capability.Resolve(m.Name, req, capability.PostgreSQL)
	if err != nil {
		t.Fatal(err)
	}
	for _, environment := range []string{"dev", "prod"} {
		if err := snapshot.Registry.BindDeployment(resource, m.ApplicationID, environment, "sql-old"); err != nil {
			t.Fatal(err)
		}
	}
	bindings := append([]capability.ProviderBinding(nil), snapshot.Registry.Bindings...)
	view := GuidedReadContext{ApplicationID: m.ApplicationID, Name: m.Name, Environment: "dev", Target: "dev", SnapshotRevision: "fresh",
		Capabilities: []capability.Requirement{req}, Preferences: []application.ProviderPreference{{Requirement: req, InstanceID: "sql-old"}},
		Footprint: application.Footprint{Version: application.FootprintVersion, Target: "dev"}}
	a := appEditAuthorities{
		Read:    func(context.Context, string) (GuidedReadContext, error) { return view, nil },
		Choices: func(context.Context, string, []capability.Requirement) ([]appProviderChoice, error) { return nil, nil },
		Plan: func(context.Context, string, []capability.Requirement, []application.ProviderPreference) (application.ProviderImpactPlan, error) {
			return application.ProviderImpactPlan{}, nil
		},
		Preview: func(context.Context, appEditDesired) (GuidedReadContext, error) { return view, nil },
	}
	session, err := appEditStart(context.Background(), "dev", []byte(m.AuthoredYAML()), snapshot, a)
	if err != nil {
		t.Fatal(err)
	}
	changed, err := session.Controller.SelectProvider(context.Background(), req, "sql-new")
	if err != nil {
		t.Fatalf("bound application cannot propose a replacement: %v", err)
	}
	if changed.phase != appWizardNeeds || !reflect.DeepEqual(snapshot.Registry.Bindings, bindings) {
		t.Fatal("provider proposal changed actual bindings or bypassed preview")
	}
	otherEnvironment := false
	for _, binding := range changed.snapshot.Registry.Bindings {
		otherEnvironment = otherEnvironment || binding.Environment == "prod" && binding.ProviderInstanceID == "sql-old"
	}
	if !otherEnvironment {
		t.Fatal("provider proposal lost the other environment's binding")
	}
}

func TestAppEditCommandBoundProviderProposalKeepsAppliedRegistry(t *testing.T) {
	r := backendBindingFixture(t, capability.ProviderPostgreSQL)
	store := capability.RegistryStore{Path: filepath.Join(r.TargetStateRoot, "provider-registry.json")}
	registry, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	projected, err := application.WithBackendPlacementProjection(r.Manifest, map[capability.ProviderKind]capability.ProviderPlacement{
		capability.ProviderPostgreSQL: {Scope: capability.ScopeApplication, Ownership: capability.OwnershipBaseHarbor},
	})
	if err != nil {
		t.Fatal(err)
	}
	offers, err := application.PreviewReferenceProviderRegistryAt(r.TargetStateRoot, projected)
	if err != nil {
		t.Fatal(err)
	}
	var destination capability.ProviderInstance
	for _, offer := range offers.Instances {
		if offer.Provider.Kind == capability.ProviderPostgreSQL && offer.Scope == capability.ScopeApplication && offer.OwnerApplicationID == r.Manifest.ApplicationID {
			destination = offer
		}
	}
	if destination.ID == "" {
		t.Fatal("missing canonical destination")
	}
	if err := registry.Register(destination); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(registry); err != nil {
		t.Fatal(err)
	}
	req := capability.Requirement{Kind: capability.SQL, Name: "default"}
	a := defaultAppEditAuthorities(r.Store, nil)
	choices, err := a.Choices(context.Background(), r.Target.Name, []capability.Requirement{req})
	if err != nil || len(choices) != 1 {
		t.Fatalf("choices: %v %v", choices, err)
	}
	if _, err := choices[0].Select(destination.ID); err != nil {
		t.Fatalf("existing binding blocked proposed replacement: %v", err)
	}
	prefs := []application.ProviderPreference{{Requirement: req, InstanceID: destination.ID, ProviderKind: capability.ProviderPostgreSQL}}
	plan, err := a.Plan(context.Background(), r.Target.Name, []capability.Requirement{req}, prefs)
	if err != nil || plan.PlanID == "" {
		t.Fatalf("replacement plan: %v", err)
	}
	preview, err := a.Preview(context.Background(), appEditDesired{Target: r.Target.Name, Manifest: r.Manifest, Preferences: prefs})
	if err != nil || len(preview.Footprint.Requirements) != 1 || preview.Footprint.Requirements[0].InstanceID != destination.ID {
		t.Fatalf("replacement preview: %+v %v", preview.Footprint, err)
	}
	after, err := store.Load()
	if err != nil || !reflect.DeepEqual(after, registry) {
		t.Fatal("read-only replacement changed applied registry", err)
	}
}

func TestAppEditCommandPreviewSamplingKeepsAuthorityAndDisplay(t *testing.T) {
	for _, drift := range []string{"sampling", "owner", "state", "expiry", "outcome", "confirmations", "plan", "snapshot"} {
		t.Run(drift, func(t *testing.T) {
			input := appWizardTestInput(t)
			refresh := input.Refresh
			calls := 0
			var observed *GuidedReadContext
			var observedPlan *application.ProviderImpactPlan
			input.Refresh = func(ctx context.Context, d appWizardDecision) (*GuidedReadContext, *application.ProviderImpactPlan, error) {
				fp, plan, err := refresh(ctx, d)
				calls++
				quantity, remaining := int64(calls*1024), int64(120-calls)
				fp.SnapshotRevision = "stable-structure"
				fp.Footprint.Existing = []application.ProviderFootprint{{InstanceID: "sql-a", MemoryBytes: application.Quantity{Value: &quantity}, Containers: application.Quantity{Value: &quantity}}}
				fp.Footprint.Additional = []application.ProviderFootprint{{InstanceID: "sql-b", MemoryBytes: application.Quantity{Value: &quantity}}}
				plan.Impacts[0].Functions = []application.FunctionConsequence{{Consumer: "app", Outcome: "remains", ValidUntil: "2030-01-01T00:00:00Z", RemainingValiditySeconds: &remaining}}
				plan.Impacts[0].Materials = []application.MaterialConsequence{{Before: application.ProviderMaterial{ID: "certificate", ExpiresAt: "2030-01-01T00:00:00Z"}, RemainingValiditySeconds: &remaining}}
				if calls > 1 {
					switch drift {
					case "owner":
						fp.Footprint.Existing[0].Ownership = capability.OwnershipBaseHarbor
					case "state":
						fp.Footprint.Existing[0].State = application.ProviderState("changed")
					case "expiry":
						plan.Impacts[0].Materials[0].Before.ExpiresAt = "2029-01-01T00:00:00Z"
					case "outcome":
						plan.Impacts[0].Functions[0].Outcome = "unavailable"
					case "confirmations":
						plan.ConfirmationsRequired++
					case "plan":
						plan.PlanID = "other"
					case "snapshot":
						fp.SnapshotRevision = "changed-structure"
					}
				}
				observed, observedPlan = fp, plan
				return fp, plan, err
			}
			w, err := newAppWizardController(context.Background(), input)
			if err != nil {
				t.Fatal(err)
			}
			w = appWizardAdvance(t, w, appWizardNext)
			if *observed.Footprint.Existing[0].MemoryBytes.Value != 1024 || observedPlan.Impacts[0].Functions[0].RemainingValiditySeconds == nil || w.impact.Impacts[0].Materials[0].RemainingValiditySeconds == nil {
				t.Fatal("comparison changed displayed authority data")
			}
			next, err := w.Step(context.Background(), appWizardYes)
			_, decided := next.Decision()
			if drift == "sampling" {
				if err != nil || !decided {
					t.Fatalf("sample-only refresh rejected: %v", err)
				}
			} else if err == nil || decided {
				t.Fatalf("authority drift %s accepted", drift)
			}
			if *w.footprint.Footprint.Existing[0].MemoryBytes.Value != 1024 {
				t.Fatal("detached preview was changed by refresh")
			}
		})
	}
}

func TestAppEditCommandSnapshotRevisionIgnoresOnlyUsageSamples(t *testing.T) {
	input := appWizardTestInput(t)
	value := int64(1024)
	input.Snapshot.Facts = map[string]application.ProviderFact{"sql-a": {Target: "dev", OwnerReference: "app", MemoryBytes: application.Quantity{Value: &value}, Containers: application.Quantity{Value: &value}}}
	original, err := guidedSnapshotRevision(input.Manifest, input.Snapshot, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, drift := range []string{"sampling", "state", "owner", "target", "dependencies", "manifest", "preferences", "binding"} {
		t.Run(drift, func(t *testing.T) {
			snapshot := input.Snapshot
			snapshot.Facts = map[string]application.ProviderFact{"sql-a": input.Snapshot.Facts["sql-a"]}
			fact := snapshot.Facts["sql-a"]
			manifest := input.Manifest
			var preferences []application.ProviderPreference
			switch drift {
			case "sampling":
				sample := int64(2048)
				fact.MemoryBytes.Value = &sample
				fact.Containers.Value = &sample
			case "state":
				fact.State = application.ProviderState("changed")
			case "owner":
				fact.OwnerReference = "other"
			case "target":
				snapshot.Target = "other"
			case "dependencies":
				fact.Dependencies = []string{"other"}
			case "manifest":
				manifest.Name = "other"
			case "preferences":
				preferences = []application.ProviderPreference{{InstanceID: "sql-b"}}
			case "binding":
				snapshot.Registry.Bindings = append([]capability.ProviderBinding(nil), snapshot.Registry.Bindings...)
				snapshot.Registry.Bindings = append(snapshot.Registry.Bindings, capability.ProviderBinding{ProviderInstanceID: "sql-a"})
			}
			snapshot.Facts["sql-a"] = fact
			revision, err := guidedSnapshotRevision(manifest, snapshot, preferences)
			if err != nil {
				t.Fatal(err)
			}
			if (revision == original) != (drift == "sampling") {
				t.Fatalf("incorrect revision equality for %s", drift)
			}
		})
	}
	if *input.Snapshot.Facts["sql-a"].MemoryBytes.Value != 1024 {
		t.Fatal("revision changed original facts")
	}
}

func TestAppEditApplyConsentCountdownBindsAbsoluteExpiry(t *testing.T) {
	seconds := int64(120)
	plan := application.ProviderImpactPlan{PlanID: "approved", Impacts: []application.ProviderImpact{{Functions: []application.FunctionConsequence{{ValidUntil: "2030-01-01T00:00:00Z", RemainingValiditySeconds: &seconds}}, Materials: []application.MaterialConsequence{{Before: application.ProviderMaterial{ID: "cert", ExpiresAt: "2030-01-01T00:00:00Z"}, RemainingValiditySeconds: &seconds}}}}}
	original, err := appEditAuthorizationSurface(plan)
	if err != nil {
		t.Fatal(err)
	}
	seconds--
	refreshed, err := appEditAuthorizationSurface(plan)
	if err != nil || !bytes.Equal(original, refreshed) {
		t.Fatal("countdown invalidated consent")
	}
	plan.Impacts[0].Materials[0].Before.ExpiresAt = "2029-01-01T00:00:00Z"
	changed, err := appEditAuthorizationSurface(plan)
	if err != nil || bytes.Equal(original, changed) {
		t.Fatal("changed absolute expiry inherited consent")
	}
}

func TestAppEditApplyRebuildKeepsRuntimeOutput(t *testing.T) {
	resolved := appEditResultResolved("dev")
	resolved.Manifest = application.WithWorkloadComponents(application.New("edit-output", "dev", false, false, false), "api")
	var out, errOut bytes.Buffer
	execution, err := rebuildApplicationApplyExecution(context.Background(), resolved, &out, &errOut)
	if err != nil {
		t.Fatal(err)
	}
	if out.Len() != 0 || errOut.Len() != 0 {
		t.Fatal("rebuild repeated initial header")
	}
	execution.term.Info("applying", "visible-progress")
	fmt.Fprintln(execution.out, "runtime-output")
	fmt.Fprintln(execution.errOut, "runtime-diagnostic")
	execution.term.Fail("verification", "visible-failure")
	if !strings.Contains(out.String(), "visible-progress") || !strings.Contains(out.String(), "runtime-output") || !strings.Contains(out.String(), "visible-failure") || !strings.Contains(errOut.String(), "runtime-diagnostic") {
		t.Fatalf("rebuilt execution swallowed output: %q / %q", out.String(), errOut.String())
	}
}

func TestAppEditApplyFreshMachinePlanCountdownAndAuthority(t *testing.T) {
	for _, drift := range []string{"countdown", "expiry", "verification", "outcome", "plan"} {
		t.Run(drift, func(t *testing.T) {
			before := []capability.Requirement{{Kind: capability.SQL, Name: "primary"}}
			after := append(append([]capability.Requirement(nil), before...), capability.Requirement{Kind: capability.KeyValue, Name: "cache"})
			loaded := appEditTestLoaded(t, "dev", before, nil)
			seconds, changedSeconds := int64(120), int64(119)
			plan := application.ProviderImpactPlan{Version: application.ProviderImpactVersion, Target: "dev", Revision: "r", PlanID: "p", Complete: true, ConfirmationsRequired: 1, Impacts: []application.ProviderImpact{{InstanceID: "cache", Operation: "add", Functions: []application.FunctionConsequence{{ValidUntil: "2030-01-01T00:00:00Z", RemainingValiditySeconds: &seconds, Outcome: "remains"}}, Materials: []application.MaterialConsequence{{Before: application.ProviderMaterial{ID: "cert", ExpiresAt: "2030-01-01T00:00:00Z", Verified: true}, RemainingValiditySeconds: &seconds}}}}}
			fresh := stableAppProviderImpact(plan)
			fresh.Impacts[0].Functions[0].RemainingValiditySeconds = &changedSeconds
			fresh.Impacts[0].Materials[0].RemainingValiditySeconds = &changedSeconds
			switch drift {
			case "expiry":
				fresh.Impacts[0].Materials[0].Before.ExpiresAt = "2029-01-01T00:00:00Z"
			case "verification":
				fresh.Impacts[0].Materials[0].Before.Verified = false
			case "outcome":
				fresh.Impacts[0].Functions[0].Outcome = "unavailable"
			case "plan":
				fresh.PlanID = "other"
			}
			executed := 0
			authorities := appEditTestAuthorities("dev", loaded.ApplicationID, before, nil, []application.ProviderImpactPlan{plan, fresh}, &executed)
			_, err := appEditApplyConfirmed(context.Background(), appEditTestController("dev", after, nil, plan), loaded, application.ProviderChangeConsent{PlanID: plan.PlanID, First: true}, authorities)
			if drift == "countdown" {
				if err != nil || executed != 1 {
					t.Fatalf("countdown blocked execution: %v (%d)", err, executed)
				}
			} else if err == nil || executed != 0 {
				t.Fatalf("authority drift executed: %s %v (%d)", drift, err, executed)
			}
		})
	}
}
