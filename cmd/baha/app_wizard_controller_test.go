package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
)

func appWizardTestInput(t *testing.T) appWizardInput {
	t.Helper()
	m := application.New("catalog", "dev", false, false, false)
	m.ApplicationID = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	m = application.WithSQLInstances(m, "primary", "analytics")
	reg := capability.NewRegistry()
	reg.Instances = []capability.ProviderInstance{
		{ID: "sql-a", Provider: capability.PostgreSQL, Scope: capability.ScopeShared, Ownership: capability.OwnershipBaseHarbor, SharingBoundary: "a"},
		{ID: "sql-b", Provider: capability.PostgreSQL, Scope: capability.ScopeShared, Ownership: capability.OwnershipBaseHarbor, SharingBoundary: "b"},
	}
	if err := reg.Validate(); err != nil {
		t.Fatal(err)
	}
	return appWizardInput{
		Manifest: m, Snapshot: application.ResolutionSnapshot{Target: "dev", Registry: reg},
		Selection: appCapabilitySelectionInput{Current: []capability.Requirement{
			{Kind: capability.SQL, Name: "primary"}, {Kind: capability.SQL, Name: "analytics"},
		}}, TTY: true,
		Refresh: func(ctx context.Context, d appWizardDecision) (*GuidedReadContext, *application.ProviderImpactPlan, error) {
			fp := &GuidedReadContext{}
			fp.Footprint.Target = d.Target
			fp.Footprint.Complete = true
			impact := &application.ProviderImpactPlan{Target: d.Target, Complete: true, PlanID: "current", Version: application.ProviderImpactVersion}
			for _, req := range d.Requirements { impact.Impacts = append(impact.Impacts, application.ProviderImpact{InstanceID: req.Name, Operation: "retain"}) }
			for _, pref := range d.Preferences { if pref.InstanceID != "" { impact.Impacts = append(impact.Impacts, application.ProviderImpact{InstanceID: pref.InstanceID, Operation: "select"}) } }
			return fp, impact, nil
		},
	}
}

func appWizardAdvance(t *testing.T, w appWizardController, event appWizardEvent) appWizardController {
	t.Helper()
	n, err := w.Step(context.Background(), event)
	if err != nil {
		t.Fatalf("transition %s: %v", event, err)
	}
	return n
}

func TestAppWizardControllerRepeatedBackPreservesNamedResourcesAndProviders(t *testing.T) {
	input := appWizardTestInput(t)
	original := input.Manifest
	w, err := newAppWizardController(context.Background(), input)
	if err != nil { t.Fatal(err) }
	for i := 0; i < 3; i++ {
		for j, id := range []string{"sql-a","sql-b"} {
			w, err = w.SelectProvider(context.Background(), input.Selection.Current[j], id)
			if err != nil { t.Fatal(err) }
		}
		w = appWizardAdvance(t,w,appWizardNext)
		if w.phase != appWizardPreview || !strings.Contains(w.View(),"sql-a") { t.Fatal(w.View()) }
		w = appWizardAdvance(t,w,appWizardNo)
		if w.phase != appWizardNeeds || !reflect.DeepEqual(w.selection.Requirements(),input.Selection.Current) { t.Fatal("No did not restore checklist") }
		if w.preferences[appWizardKey("dev",input.Selection.Current[0])] != "sql-a" { t.Fatal("preference lost") }
	}
	w = appWizardAdvance(t,w,appWizardNext)
	w = appWizardAdvance(t,w,appWizardYes)
	d,ok := w.Decision()
	if !ok || len(d.Preferences)!=2 || d.Preferences[0].InstanceID!="sql-a" || d.Preferences[1].InstanceID!="sql-b" { t.Fatalf("decision: %+v",d) }
	if !reflect.DeepEqual(input.Manifest,original) { t.Fatal("original mutated") }
}

func TestAppWizardControllerChangedNeedsBackRetainsExistingComplexState(t *testing.T) {
	input := appWizardTestInput(t)
	w,err := newAppWizardController(context.Background(),input)
	if err != nil { t.Fatal(err) }
	w = appWizardAdvance(t,w,appWizardNext)
	w = appWizardAdvance(t,w,appWizardNo)
	w,err = w.SelectNeed(context.Background(),"space")
	if err != nil || len(w.selection.Requirements())!=0 { t.Fatal("deselection failed") }
	w = appWizardAdvance(t,w,appWizardNext)
	if strings.Contains(w.View(),"primary: retain") { t.Fatal("stale preview") }
	w = appWizardAdvance(t,w,appWizardNo)
	w,err = w.SelectNeed(context.Background(),"space")
	if err != nil || !reflect.DeepEqual(w.selection.Requirements(),input.Selection.Current) { t.Fatal("names lost") }
	w = appWizardAdvance(t,w,appWizardNext)
	if !strings.Contains(w.View(),"primary: retain") { t.Fatal("fresh consequences missing") }
}

func TestAppWizardControllerCancellationAtEveryTransitionLeavesFilesAndInputs(t *testing.T) {
	for _,stage := range []appWizardPhase{appWizardNeeds,appWizardPreview} {
		for _,event := range []appWizardEvent{appWizardCancel,appWizardEOF} {
			t.Run(string(stage)+"/"+string(event),func(t *testing.T) {
				input := appWizardTestInput(t)
				manifest := input.Manifest
				registry := input.Snapshot.Registry
				dir := t.TempDir()
				path := filepath.Join(dir,"manifest.yaml")
				data := []byte("name: preserved\\ncomplex: [one, two]\\nsecrets: unchanged\\n")
				if err := os.WriteFile(path,data,0600);err != nil {t.Fatal(err)}
				w,err := newAppWizardController(context.Background(),input)
				if err != nil {t.Fatal(err)}
				if stage==appWizardPreview { w=appWizardAdvance(t,w,appWizardNext) }
				w,err=w.Step(context.Background(),event)
				if !errors.Is(err,errAppWizardAbort)||w.phase!=appWizardAborted {t.Fatal(err)}
				if _,ok:=w.Decision();ok {t.Fatal("unexpected decision")}
				after,err:=os.ReadFile(path)
				if err!=nil||!reflect.DeepEqual(after,data)||!reflect.DeepEqual(manifest,input.Manifest)||!reflect.DeepEqual(registry,input.Snapshot.Registry) {t.Fatal("state or file mutated")}
			})
		}
	}
}

func TestAppWizardControllerContextAbortAtEveryTransition(t *testing.T) {
	for _,stage := range []appWizardPhase{appWizardNeeds,appWizardPreview} {
		w,err:=newAppWizardController(context.Background(),appWizardTestInput(t))
		if err!=nil {t.Fatal(err)}
		if stage==appWizardPreview {w=appWizardAdvance(t,w,appWizardNext)}
		ctx,cancel:=context.WithCancel(context.Background())
		cancel()
		next,err:=w.Step(ctx,appWizardYes)
		if !errors.Is(err,context.Canceled)||next.phase!=appWizardAborted {t.Fatal(err)}
	}
}

func TestAppWizardControllerStaleAndWrongTargetPreview(t *testing.T) {
	for _,kind := range []string{"missing","wrong-target","stale"} {
		t.Run(kind,func(t *testing.T) {
			input:=appWizardTestInput(t)
			calls:=0
			original:=input.Refresh
			input.Refresh=func(ctx context.Context,d appWizardDecision)(*GuidedReadContext,*application.ProviderImpactPlan,error){
				calls++
				if kind=="missing" {return nil,nil,nil}
				fp,impact,err:=original(ctx,d)
				if kind=="wrong-target" {impact.Target="prod"}
				if kind=="stale"&&calls>1 {impact.PlanID="changed"}
				return fp,impact,err
			}
			w,err:=newAppWizardController(context.Background(),input)
			if err!=nil {t.Fatal(err)}
			next,err:=w.Step(context.Background(),appWizardNext)
			if kind!="stale" {
				if err==nil||next.phase!=appWizardNeeds {t.Fatal("accepted missing or wrong target")}
				return
			}
			if err!=nil {t.Fatal(err)}
			if _,err=next.Step(context.Background(),appWizardYes);err==nil {t.Fatal("accepted stale preview")}
		})
	}
}

func TestAppWizardControllerProviderChangeRefreshesConsequences(t *testing.T) {
	w,err:=newAppWizardController(context.Background(),appWizardTestInput(t))
	if err!=nil {t.Fatal(err)}
	w=appWizardAdvance(t,w,appWizardNext)
	if strings.Contains(w.View(),"sql-b: select") {t.Fatal("invented explicit choice")}
	w=appWizardAdvance(t,w,appWizardNo)
	w,err=w.SelectProvider(context.Background(),capability.Requirement{Kind:capability.SQL,Name:"analytics"},"sql-b")
	if err!=nil {t.Fatal(err)}
	w=appWizardAdvance(t,w,appWizardNext)
	if !strings.Contains(w.View(),"sql-b: select") {t.Fatal("preview not refreshed")}
}
func TestAppWizardControllerNonTTYNoInputAndInitiallyCancelled(t *testing.T) {
	for _, modify := range []func(*appWizardInput){
		func(i *appWizardInput) { i.TTY = false },
		func(i *appWizardInput) { i.NoInput = true },
	} {
		input := appWizardTestInput(t)
		modify(&input)
		if _, err := newAppWizardController(context.Background(), input); !errors.Is(err, errAppWizardNonInteractive) {
			t.Fatalf("unguarded interactive wizard: %v", err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := newAppWizardController(ctx, appWizardTestInput(t)); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestAppWizardControllerPreviewIsNotLifecycleAuthorization(t *testing.T) {
	input := appWizardTestInput(t)
	plan := application.ProviderImpactPlan{Target: "dev", Destructive: true, ConfirmationsRequired: 2, Complete: false}
	input.Refresh = func(ctx context.Context, d appWizardDecision)(*GuidedReadContext,*application.ProviderImpactPlan,error){fp:=&GuidedReadContext{};fp.Footprint.Target=d.Target;plan.Target=d.Target;return fp,&plan,nil}
	w, err := newAppWizardController(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	w = appWizardAdvance(t, w, appWizardNext)
	view := w.View()
	if !strings.Contains(view, "Provider change preview") || !strings.Contains(view, "2 confirmation") {
		t.Fatal(view)
	}
	if _, ok := w.Decision(); ok {
		t.Fatal("preview granted decision")
	}
	w = appWizardAdvance(t, w, appWizardYes)
	if _, ok := w.Decision(); !ok {
		t.Fatal("expected intent decision")
	}
	if !strings.Contains(w.View(), "execution requires separate authorized lifecycle") {
		t.Fatal("controller claimed execution")
	}
}

func TestAppWizardControllerNoInputMutationOutsideCorrectPhase(t *testing.T) {
	w, err := newAppWizardController(context.Background(), appWizardTestInput(t))
	if err != nil {
		t.Fatal(err)
	}
	before := w.selection.Requirements()
	if _,err := w.SelectProvider(context.Background(),capability.Requirement{Kind:capability.SQL,Name:"other"},"sql-a");err==nil {t.Fatal("nonselected provider allowed")}
	w = appWizardAdvance(t, w, appWizardNext)
	if _, err := w.SelectNeed(context.Background(), "space"); err == nil {
		t.Fatal("needs changed outside needs phase")
	}
	if !reflect.DeepEqual(w.selection.Requirements(), before) {
		t.Fatal("unexpected mutation")
	}
}

func TestAppWizardControllerFormatting(t *testing.T) {
	for _, name := range []string{"app_wizard_controller.go", "app_wizard_controller_test.go"} {
		diff, err := exec.Command("gofmt", "-d", name).CombinedOutput()
		if err != nil || len(diff) != 0 {
			t.Errorf("gofmt mismatch in %s: %s (%v)", name, diff, err)
		}
	}
}
