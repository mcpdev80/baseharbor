package main

import (
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
)

func footprintInt(v int64) *int64 { return &v }

func TestAppFootprintPresenterEmptyAndCoreless(t *testing.T) {
	fp := application.Footprint{Target: "dev", Core: application.CoreNotRequired, Complete: true}
	got := renderAppFootprint(fp, true, appFootprintPresenterOptions{})
	for _, want := range []string{"0 new", "0 reused/running", "Core: not required", "Resolution: complete", "No provider realizations"} {
		if !strings.Contains(got, want) { t.Errorf("missing %q: %s", want, got) }
	}
	if strings.Contains(got, "Core: required") { t.Fatal(got) }
}

func TestAppFootprintPresenterStatesAndDetails(t *testing.T) {
	fp := application.Footprint{Target: "test", Core: application.CoreRequired, Complete: true,
		Additional: []application.ProviderFootprint{{InstanceID: "new-db", State: application.ProviderNewRequired, Dependencies: []string{"broker"}}},
		Existing: []application.ProviderFootprint{
			{InstanceID: "running-broker", State: application.ProviderRunning, Dependencies: []string{}},
			{InstanceID: "stopped-cache", State: application.ProviderStoppedOwned, Dependencies: []string{}},
			{InstanceID: "unverified", State: application.ProviderUnverifiable},
		},
	}
	summary := renderAppFootprint(fp, true, appFootprintPresenterOptions{})
	for _, want := range []string{"1 new", "1 reused/running", "1 stopped/owned", "1 unverified", "Core: required"} {
		if !strings.Contains(summary, want) { t.Errorf("missing %q", want) }
	}
	if strings.Contains(summary, "new-db:") { t.Fatal("details leaked into summary") }
	detail := renderAppFootprint(fp, true, appFootprintPresenterOptions{Details:true})
	for _, want := range []string{"new-db: new", "running-broker: reused/running", "stopped-cache: stopped/owned (not running)", "unverified: unverified", "Dependencies: broker", "Dependencies: none verified", "Dependencies: unknown"} {
		if !strings.Contains(detail, want) { t.Errorf("missing %q: %s", want, detail) }
	}
}

func TestAppFootprintPresenterMeasuredEstimatedUnknown(t *testing.T) {
	fp := application.Footprint{Target:"dev", Core:application.CoreUnknown, Complete:false,
		Existing: []application.ProviderFootprint{
			{InstanceID:"measured", State:application.ProviderRunning, MemoryBytes:application.Quantity{Value:footprintInt(4096), Classification:"measured", Source:"runtime"}, Containers:application.Quantity{Value:footprintInt(2), Classification:"measured", Source:"runtime"}},
			{InstanceID:"estimated", State:application.ProviderStoppedOwned, MemoryBytes:application.Quantity{Value:footprintInt(8192), Classification:"estimated", Source:"plan"}, Containers:application.Quantity{Value:footprintInt(1), Classification:"estimated", Source:"plan"}},
			{InstanceID:"missing", State:application.ProviderUnverifiable, MemoryBytes:application.Quantity{Classification:"unknown"}, Containers:application.Quantity{Classification:"unknown"}},
		},
	}
	got := renderAppFootprint(fp, false, appFootprintPresenterOptions{Details:true})
	for _, want := range []string{"4096 bytes (measured)", "2 (measured)", "8192 bytes (estimated)", "1 (estimated)", "RAM unknown; containers unknown", "Core: unknown", "Resolution: incomplete"} {
		if !strings.Contains(got, want) { t.Errorf("missing %q: %s", want, got) }
	}
	if strings.Contains(got, "12288") || strings.Contains(got, "3 containers") { t.Fatalf("invented totals: %s", got) }
}

func TestAppFootprintPresenterTransitiveAndIncomplete(t *testing.T) {
	fp := application.Footprint{Target:"dev", Core:application.CoreUnknown, Complete:false,
		Existing:[]application.ProviderFootprint{{InstanceID:"api", State:application.ProviderRunning, Dependencies:[]string{"broker"}}, {InstanceID:"broker", State:application.ProviderRunning, Dependencies:[]string{"storage"}}, {InstanceID:"storage", State:application.ProviderUnverifiable}},
	}
	got := renderAppFootprint(fp, false, appFootprintPresenterOptions{Details:true})
	for _, want := range []string{"Dependencies: broker", "Dependencies: storage", "Dependencies: unknown", "Resolution: incomplete"} {
		if !strings.Contains(got,want) { t.Errorf("missing %q: %s",want,got) }
	}
}

func TestAppFootprintPresenterContextAuthorityAndNoSecrets(t *testing.T) {
	fp := application.Footprint{Target:"dev", Core:application.CoreNotRequired, Complete:true,
		Existing:[]application.ProviderFootprint{{InstanceID:"svc", State:application.ProviderRunning, Dependencies:[]string{}}}}
	ctx:=GuidedReadContext{Footprint:fp, Complete:false, SnapshotRevision:"secret-marker", PlanRevision:"private-plan", Name:"secret-name"}
	got:=presentAppFootprint(ctx,appFootprintPresenterOptions{Details:true})
	if !strings.Contains(got,"Resolution: incomplete") {t.Fatal(got)}
	for _, secret:=range []string{"secret-marker","private-plan","secret-name"} {
		if strings.Contains(got,secret) {t.Fatalf("leaked %q",secret)}
	}
}
