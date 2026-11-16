package main

import (
	"os"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func TestReusedCoreSQLObservationRequiresExactPhysicalOwner(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	target := configureTestTarget(t)
	root, err := targetRuntimeStateRoot(target)
	if err != nil {
		t.Fatal(err)
	}
	files, err := bhruntime.EnsureFilesForProjectAndResources(root, bhruntime.SharedProjectName(target.Name), bhruntime.SharedResourceProjectName(target.Name), bhruntime.Ports{Postgres: 15432, OpenBao: 18200}, false)
	if err != nil {
		t.Fatal(err)
	}
	resolved := resolvedApplication{Target: target}
	count := int64(3)
	actual := application.ProviderFact{Target: target.Name, Ancillary: true, OwnerReference: root, Core: application.CoreRequired, State: application.ProviderRunning, Dependencies: []string{}, Containers: application.Quantity{Value: &count, Classification: "measured"}}
	id := application.RuntimeStorageDependencyID(target.Name)
	observe := func(selected *bhruntime.Files, fact application.ProviderFact) application.ProviderState {
		return observeReusedCoreSQLState(resolved, selected, map[string]application.ProviderFact{id: fact})
	}
	if got := observe(&files, actual); got != application.ProviderRunning {
		t.Fatalf("verified reused SQL falsely unavailable: %s", got)
	}
	stopped := actual
	stopped.State = application.ProviderStoppedOwned
	if got := observe(&files, stopped); got != application.ProviderStoppedOwned {
		t.Fatal("stopped physical SQL became running")
	}
	for _, mutate := range []func(*bhruntime.Files){
		func(f *bhruntime.Files) { f.Project = "foreign" },
		func(f *bhruntime.Files) { f.ResourceProject = "foreign" },
		func(f *bhruntime.Files) { f.Compose += ".foreign" },
		func(f *bhruntime.Files) { f.Env += ".foreign" },
		func(f *bhruntime.Files) { f.HA = !f.HA },
		func(f *bhruntime.Files) { f.DefaultNetworkExternal = !f.DefaultNetworkExternal },
	} {
		changed := files
		mutate(&changed)
		if observe(&changed, actual) != application.ProviderUnverifiable {
			t.Fatal("foreign or changed SQL reference adopted")
		}
	}
	for _, mutate := range []func(*application.ProviderFact){
		func(f *application.ProviderFact) { f.Target = "foreign" },
		func(f *application.ProviderFact) { f.Ancillary = false },
		func(f *application.ProviderFact) { f.OwnerReference += ".foreign" },
		func(f *application.ProviderFact) { f.Core = application.CoreNotRequired },
		func(f *application.ProviderFact) { f.Dependencies = nil },
		func(f *application.ProviderFact) { f.Dependencies = []string{"unknown"} },
		func(f *application.ProviderFact) { f.Containers.Value = nil },
		func(f *application.ProviderFact) { f.Containers.Classification = "estimated" },
		func(f *application.ProviderFact) { value := int64(0); f.Containers.Value = &value },
		func(f *application.ProviderFact) { value := int64(-1); f.Containers.Value = &value },
		func(f *application.ProviderFact) { f.State = application.ProviderUnverifiable },
		func(f *application.ProviderFact) { f.State = application.ProviderNewRequired },
	} {
		changed := actual
		mutate(&changed)
		if observe(&files, changed) != application.ProviderUnverifiable {
			t.Fatal("incomplete native authority became verified availability")
		}
	}
	if observeReusedCoreSQLState(resolved, &files, nil) != application.ProviderUnverifiable || observe(nil, actual) != application.ProviderUnverifiable {
		t.Fatal("missing physical evidence was invented")
	}
	before, err := os.ReadFile(files.Compose)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(files.Compose, append(before, []byte("# foreign\n")...), 0600); err != nil {
		t.Fatal(err)
	}
	if observe(&files, actual) != application.ProviderUnverifiable {
		t.Fatal("damaged canonical owner was adopted")
	}
	after, _ := os.ReadFile(files.Compose)
	if string(after) != string(before)+"# foreign\n" {
		t.Fatal("observation rewrote physical owner")
	}
}
