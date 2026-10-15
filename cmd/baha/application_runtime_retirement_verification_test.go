package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

type coreSQLRetirementFixture struct {
	resolved                 resolvedApplication
	plan                     application.ProviderPlan
	facts                    map[string]application.ProviderFact
	registry                 capability.Registry
	core                     bhruntime.Files
	shared                   application.SharedBackendFiles
	logical, module, storage string
}

func newCoreSQLRetirementFixture(t *testing.T) *coreSQLRetirementFixture {
	t.Helper()
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv(application.ProviderScopeEnv(capability.ProviderPostgreSQL), string(capability.ScopeShared))
	target := configureTestTarget(t)
	root, err := targetDataRoot(target)
	if err != nil {
		t.Fatal(err)
	}
	runtimeRoot, err := targetRuntimeStateRoot(target)
	if err != nil {
		t.Fatal(err)
	}
	core, err := bhruntime.EnsureFilesForProjectAndResources(runtimeRoot, bhruntime.SharedProjectName(target.Name), bhruntime.SharedResourceProjectName(target.Name), bhruntime.Ports{Postgres: 15432, OpenBao: 18200}, false)
	if err != nil {
		t.Fatal(err)
	}
	m := application.New("retired-sql", "dev", true, false, false)
	if err := application.ReconcileReferenceProviderRegistryAt(root, m); err != nil {
		t.Fatal(err)
	}
	store := capability.RegistryStore{Path: filepath.Join(root, "provider-registry.json")}
	registry, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	f := &coreSQLRetirementFixture{resolved: resolvedApplication{Target: target, TargetStateRoot: root, Manifest: m}, core: core}
	for _, instance := range registry.Instances {
		if instance.Provider.Kind == capability.ProviderPostgreSQL && instance.Scope == capability.ScopeShared {
			f.logical = instance.ID
		}
	}
	if f.logical == "" || len(registry.Bindings) == 0 {
		t.Fatal("missing shared SQL fixture")
	}
	released := append([]capability.ProviderBinding{}, registry.Bindings...)
	if err := application.ReleaseApplicationProviderReferencesAt(root, m); err != nil {
		t.Fatal(err)
	}
	f.registry, err = store.Load()
	if err != nil {
		t.Fatal(err)
	}
	f.shared = application.SharedBackendFilesAt(root, target.Name, "core")
	if err := os.MkdirAll(f.shared.Dir, 0700); err != nil {
		t.Fatal(err)
	}
	state, err := json.Marshal(map[string]any{"version": 3, "environment": "core", "core_sql": core, "applications": map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	for path, data := range map[string][]byte{
		f.shared.State:   state,
		f.shared.Env:     []byte(""),
		f.shared.Compose: []byte("services: {}\nvolumes:\nnetworks:\n  shared-backend:\n    name: " + f.shared.Network + "\n    external: true\n"),
	} {
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := application.ExistingSharedBackendInventoryAt(root, target.Name, "core"); err != nil {
		t.Fatal(err)
	}
	f.module = application.SharedBackendRuntimeDependency(target.Name, capability.ProviderPostgreSQL).ID
	f.storage = application.RuntimeStorageDependencyID(target.Name)
	zero, count := int64(0), int64(3)
	f.facts = map[string]application.ProviderFact{
		f.storage: {Target: target.Name, State: application.ProviderRunning, Core: application.CoreRequired, Dependencies: []string{}, Ancillary: true, OwnerReference: runtimeRoot, Containers: application.Quantity{Value: &count, Classification: "measured"}},
		f.module:  {Target: target.Name, State: application.ProviderRunning, Core: application.CoreRequired, Dependencies: []string{f.storage}, Ancillary: true, OwnerReference: "runtime/" + f.shared.Project, Containers: application.Quantity{Value: &zero, Classification: "measured"}},
		f.logical: {Target: target.Name, State: application.ProviderRunning, Core: application.CoreRequired, Dependencies: []string{f.module}, Containers: application.Quantity{Value: &zero, Classification: "measured"}},
	}
	f.plan = application.ProviderPlan{Version: application.ProviderPlanVersion, Target: target.Name, Complete: true, Consumers: map[string]application.Footprint{"installation/current": {}}, ConsumerActivity: map[string]bool{"installation/current": true}, ReleasedBindings: released, Changes: []application.ProviderChange{
		{InstanceID: f.storage, Action: "reuse", Ownership: capability.OwnershipBaseHarbor, Consumers: []string{"installation/current"}},
		{InstanceID: f.module, Action: "stop", Ownership: capability.OwnershipBaseHarbor},
		{InstanceID: f.logical, Action: "stop", Ownership: capability.OwnershipBaseHarbor},
	}}
	return f
}

func (f *coreSQLRetirementFixture) changeFact(id string, mutate func(*application.ProviderFact)) {
	fact := f.facts[id]
	mutate(&fact)
	f.facts[id] = fact
}

func TestRuntimeRetirementVerifiesReleasedLogicalSQLWithoutStoppingRequiredCore(t *testing.T) {
	f := newCoreSQLRetirementFixture(t)
	before, err := os.ReadFile(f.core.Compose)
	if err != nil {
		t.Fatal(err)
	}
	if err := verifyApplicationRuntimeRetirement(f.resolved, f.plan, f.facts); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(f.core.Compose)
	if err != nil || string(before) != string(after) {
		t.Fatal("verification changed physical owner")
	}
	if f.facts[f.storage].State != application.ProviderRunning {
		t.Fatal("physical Core was retired")
	}
}

func TestRuntimeRetirementRejectsUnverifiedLogicalSQLRelease(t *testing.T) {
	cases := map[string]func(*testing.T, *coreSQLRetirementFixture){
		"unknown logical state": func(_ *testing.T, f *coreSQLRetirementFixture) {
			f.changeFact(f.logical, func(v *application.ProviderFact) { v.State = application.ProviderUnverifiable })
		},
		"foreign selected target": func(_ *testing.T, f *coreSQLRetirementFixture) {
			f.changeFact(f.logical, func(v *application.ProviderFact) { v.Target = "foreign" })
		},
		"missing container measurement": func(_ *testing.T, f *coreSQLRetirementFixture) {
			f.changeFact(f.logical, func(v *application.ProviderFact) { v.Containers.Value = nil })
		},
		"estimated zero": func(_ *testing.T, f *coreSQLRetirementFixture) {
			f.changeFact(f.logical, func(v *application.ProviderFact) { v.Containers.Classification = "estimated" })
		},
		"module has containers": func(_ *testing.T, f *coreSQLRetirementFixture) {
			f.changeFact(f.module, func(v *application.ProviderFact) { n := int64(1); v.Containers.Value = &n })
		},
		"foreign module owner": func(_ *testing.T, f *coreSQLRetirementFixture) {
			f.changeFact(f.module, func(v *application.ProviderFact) { v.OwnerReference = "foreign" })
		},
		"unknown module dependencies": func(_ *testing.T, f *coreSQLRetirementFixture) {
			f.changeFact(f.module, func(v *application.ProviderFact) { v.Dependencies = nil })
		},
		"foreign physical owner": func(_ *testing.T, f *coreSQLRetirementFixture) {
			f.changeFact(f.storage, func(v *application.ProviderFact) { v.OwnerReference = "foreign" })
		},
		"missing physical owner": func(_ *testing.T, f *coreSQLRetirementFixture) { delete(f.facts, f.storage) },
		"physical owner not running": func(_ *testing.T, f *coreSQLRetirementFixture) {
			f.changeFact(f.storage, func(v *application.ProviderFact) { v.State = application.ProviderStoppedOwned })
		},
		"no active physical consumer": func(_ *testing.T, f *coreSQLRetirementFixture) {
			f.plan.ConsumerActivity["installation/current"] = false
		},
		"unknown physical consumer":    func(_ *testing.T, f *coreSQLRetirementFixture) { delete(f.plan.Consumers, "installation/current") },
		"physical retirement selected": func(_ *testing.T, f *coreSQLRetirementFixture) { f.plan.Changes[0].Action = "stop" },
		"incomplete plan":              func(_ *testing.T, f *coreSQLRetirementFixture) { f.plan.Complete = false },
		"wrong plan target":            func(_ *testing.T, f *coreSQLRetirementFixture) { f.plan.Target = "foreign" },
		"no released binding":          func(_ *testing.T, f *coreSQLRetirementFixture) { f.plan.ReleasedBindings = nil },
		"foreign released owner":       func(_ *testing.T, f *coreSQLRetirementFixture) { f.plan.ReleasedBindings[0].ApplicationID = "foreign" },
		"remaining SQL reference": func(t *testing.T, f *coreSQLRetirementFixture) {
			f.registry.Bindings = append(f.registry.Bindings, f.plan.ReleasedBindings[0])
			if err := (capability.RegistryStore{Path: filepath.Join(f.resolved.TargetStateRoot, "provider-registry.json")}).Save(f.registry); err != nil {
				t.Fatal(err)
			}
		},
		"damaged physical owner": func(t *testing.T, f *coreSQLRetirementFixture) {
			file, err := os.OpenFile(f.core.Compose, os.O_APPEND|os.O_WRONLY, 0600)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := file.WriteString("# foreign\n"); err != nil {
				t.Fatal(err)
			}
			if err := file.Close(); err != nil {
				t.Fatal(err)
			}
		},
		"damaged shared owner": func(t *testing.T, f *coreSQLRetirementFixture) {
			if err := os.WriteFile(f.shared.Compose, []byte("services: {}\n"), 0600); err != nil {
				t.Fatal(err)
			}
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			f := newCoreSQLRetirementFixture(t)
			mutate(t, f)
			if err := verifyApplicationRuntimeRetirement(f.resolved, f.plan, f.facts); err == nil {
				t.Fatal("unverified retirement accepted")
			}
		})
	}
}

func TestRuntimeRetirementStillRequiresMeasuredPhysicalAbsence(t *testing.T) {
	f := newCoreSQLRetirementFixture(t)
	f.plan.Changes = []application.ProviderChange{{InstanceID: "physical", Action: "remove-runtime", Ownership: capability.OwnershipBaseHarbor}}
	zero := int64(0)
	fact := application.ProviderFact{Target: f.resolved.Target.Name, State: application.ProviderStoppedOwned, Containers: application.Quantity{Value: &zero, Classification: "measured"}}
	f.facts["physical"] = fact
	if err := verifyApplicationRuntimeRetirement(f.resolved, f.plan, f.facts); err != nil {
		t.Fatal(err)
	}
	fact.State = application.ProviderRunning
	f.facts["physical"] = fact
	if err := verifyApplicationRuntimeRetirement(f.resolved, f.plan, f.facts); err == nil {
		t.Fatal("arbitrary running+zero accepted")
	}
}
