package main

import (
	"context"
	"errors"
	"os"
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
		{ID:"sql-a",Provider:capability.PostgreSQL,Scope:capability.ScopeShared,Ownership:capability.OwnershipBaseHarbor,SharingBoundary:"a"},
		{ID:"sql-b",Provider:capability.PostgreSQL,Scope:capability.ScopeShared,Ownership:capability.OwnershipBaseHarbor,SharingBoundary:"b"},
	}
	if err:=reg.Validate();err!=nil { t.Fatal(err) }
	return appWizardInput{
		Manifest:m,Snapshot:application.ResolutionSnapshot{Target:"dev",Registry:reg},
		Selection:appCapabilitySelectionInput{Current:[]capability.Requirement{
			{Kind:capability.SQL,Name:"primary"},{Kind:capability.SQL,Name:"analytics"},
		}},TTY:true,
	}
}

func appWizardAdvance(t *testing.T,w appWizardController,event appWizardEvent) appWizardController {
	t.Helper()
	n,err:=w.Step(context.Background(),event)
	if err!=nil {t.Fatalf("transition %s: %v",event,err)}
	return n
}

func TestAppWizardControllerRepeatedBackPreservesNamedResourcesAndProviders(t *testing.T) {
	input:=appWizardTestInput(t)
	original:=input.Manifest
	w,err:=newAppWizardController(context.Background(),input)
	if err!=nil {t.Fatal(err)}
	for i:=0;i<3;i++ {
		w=appWizardAdvance(t,w,appWizardNext)
		if w.phase!=appWizardProvider || len(w.choices)!=2 {t.Fatalf("provider choices lost: %+v",w)}
		for j,id:=range []string{"sql-a","sql-b"} {
			w,err=w.SelectProvider(context.Background(),w.choices[j].Requirement,id)
			if err!=nil {t.Fatal(err)}
		}
		w=appWizardAdvance(t,w,appWizardNext)
		if !strings.Contains(w.View(),"Read-only decision preview") {t.Fatal(w.View())}
		w=appWizardAdvance(t,w,appWizardNo)
		if w.phase!=appWizardProvider || w.choices[0].PreferredInstanceID!="sql-a" || w.choices[1].PreferredInstanceID!="sql-b" {t.Fatal("provider choices not retained")}
		w=appWizardAdvance(t,w,appWizardBack)
		if w.phase!=appWizardNeeds || !reflect.DeepEqual(w.selection.Requirements(),input.Selection.Current) {t.Fatal("named needs lost")}
	}
	w=appWizardAdvance(t,w,appWizardNext)
	w=appWizardAdvance(t,w,appWizardNext)
	w=appWizardAdvance(t,w,appWizardYes)
	d,ok:=w.Decision()
	if !ok || len(d.Preferences)!=2 || d.Preferences[0].InstanceID!="sql-a" || d.Preferences[1].InstanceID!="sql-b" ||
		!reflect.DeepEqual(d.Requirements,input.Selection.Current) {t.Fatalf("decision lost: %+v",d)}
	if !reflect.DeepEqual(input.Manifest,original) {t.Fatal("original manifest mutated")}
}

func TestAppWizardControllerChangedNeedsBackRetainsExistingComplexState(t *testing.T) {
	input:=appWizardTestInput(t)
	original:=input.Manifest
	w,err:=newAppWizardController(context.Background(),input)
	if err!=nil {t.Fatal(err)}
	w=appWizardAdvance(t,w,appWizardNext)
	w=appWizardAdvance(t,w,appWizardBack)
	w,err=w.SelectNeed(context.Background(),"space")
	if err!=nil {t.Fatal(err)}
	if len(w.selection.Requirements())!=0 {t.Fatal("deselection should omit both named SQL resources")}
	w,err=w.SelectNeed(context.Background(),"space")
	if err!=nil || !reflect.DeepEqual(w.selection.Requirements(),input.Selection.Current) {t.Fatal("reselection must restore exact named originals")}
	w=appWizardAdvance(t,w,appWizardNext)
	if len(w.choices)!=2 {t.Fatal("current requirements not restored")}
	if !reflect.DeepEqual(input.Manifest,original) {t.Fatal("complex original mutated")}
}

func TestAppWizardControllerCancellationAtEveryTransitionLeavesFilesAndInputs(t *testing.T) {
	for _,stage:=range []appWizardPhase{appWizardNeeds,appWizardProvider,appWizardPreview} {
		for _,event:=range []appWizardEvent{appWizardCancel,appWizardEOF} {
			t.Run(string(stage)+"/"+string(event),func(t *testing.T) {
				input:=appWizardTestInput(t)
				originalManifest:=input.Manifest
				originalRegistry:=input.Snapshot.Registry
				originalSelection:=append([]capability.Requirement(nil),input.Selection.Current...)
				dir:=t.TempDir()
				file:=filepath.Join(dir,"manifest.yaml")
				data:=[]byte("name: preserved\ncomplex: [one, two]\nsecrets: unchanged\n")
				if err:=os.WriteFile(file,data,0600);err!=nil {t.Fatal(err)}
				w,err:=newAppWizardController(context.Background(),input)
				if err!=nil {t.Fatal(err)}
				if stage!=appWizardNeeds {w=appWizardAdvance(t,w,appWizardNext)}
				if stage==appWizardPreview {w=appWizardAdvance(t,w,appWizardNext)}
				w,err=w.Step(context.Background(),event)
				if !errors.Is(err,errAppWizardAbort) || w.phase!=appWizardAborted {t.Fatalf("not aborted: %s %v",w.phase,err)}
				if _,ok:=w.Decision();ok {t.Fatal("abort produced decision")}
				after,err:=os.ReadFile(file)
				if err!=nil || !reflect.DeepEqual(after,data) {t.Fatal("abort wrote disk")}
				if !reflect.DeepEqual(input.Manifest,originalManifest) ||
					!reflect.DeepEqual(input.Snapshot.Registry,originalRegistry) ||
					!reflect.DeepEqual(input.Selection.Current,originalSelection) {t.Fatal("abort changed original state")}
			})
		}
	}
}

func TestAppWizardControllerContextAbortAtEveryTransition(t *testing.T) {
	for _,stage:=range []appWizardPhase{appWizardNeeds,appWizardProvider,appWizardPreview} {
		input:=appWizardTestInput(t)
		w,err:=newAppWizardController(context.Background(),input)
		if err!=nil {t.Fatal(err)}
		if stage!=appWizardNeeds {w=appWizardAdvance(t,w,appWizardNext)}
		if stage==appWizardPreview {w=appWizardAdvance(t,w,appWizardNext)}
		ctx,cancel:=context.WithCancel(context.Background())
		cancel()
		next,err:=w.Step(ctx,appWizardYes)
		if !errors.Is(err,context.Canceled) || next.phase!=appWizardAborted {t.Fatalf("%s: %v",stage,err)}
		if _,ok:=next.Decision();ok {t.Fatal("context abort authorized a decision")}
	}
}

func TestAppWizardControllerNonTTYNoInputAndInitiallyCancelled(t *testing.T) {
	for _,modify:=range []func(*appWizardInput){
		func(i *appWizardInput){i.TTY=false},
		func(i *appWizardInput){i.NoInput=true},
	} {
		input:=appWizardTestInput(t)
		modify(&input)
		if _,err:=newAppWizardController(context.Background(),input);!errors.Is(err,errAppWizardNonInteractive) {t.Fatalf("unguarded interactive wizard: %v",err)}
	}
	ctx,cancel:=context.WithCancel(context.Background())
	cancel()
	if _,err:=newAppWizardController(ctx,appWizardTestInput(t));!errors.Is(err,context.Canceled) {t.Fatal(err)}
}

func TestAppWizardControllerPreviewIsNotLifecycleAuthorization(t *testing.T) {
	input:=appWizardTestInput(t)
	plan:=application.ProviderImpactPlan{Target:"dev",Destructive:true,ConfirmationsRequired:2,Complete:false}
	input.Impact=&plan
	w,err:=newAppWizardController(context.Background(),input)
	if err!=nil {t.Fatal(err)}
	w=appWizardAdvance(t,w,appWizardNext)
	w=appWizardAdvance(t,w,appWizardNext)
	view:=w.View()
	if !strings.Contains(view,"Provider change preview") || !strings.Contains(view,"2 confirmation") {t.Fatal(view)}
	if _,ok:=w.Decision();ok {t.Fatal("preview granted decision")}
	w=appWizardAdvance(t,w,appWizardYes)
	if _,ok:=w.Decision();!ok {t.Fatal("expected intent decision")}
	if !strings.Contains(w.View(),"execution requires separate authorized lifecycle") {t.Fatal("controller claimed execution")}
}

func TestAppWizardControllerNoInputMutationOutsideCorrectPhase(t *testing.T) {
	w,err:=newAppWizardController(context.Background(),appWizardTestInput(t))
	if err!=nil {t.Fatal(err)}
	before:=w.selection.Requirements()
	if _,err:=w.SelectProvider(context.Background(),before[0],"sql-a");err==nil {t.Fatal("provider selection before provider phase")}
	w=appWizardAdvance(t,w,appWizardNext)
	if _,err:=w.SelectNeed(context.Background(),"space");err==nil {t.Fatal("needs changed outside needs phase")}
	if !reflect.DeepEqual(w.selection.Requirements(),before) {t.Fatal("unexpected mutation")}
}
