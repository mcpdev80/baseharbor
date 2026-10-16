package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
)

func footprintShowFixture() GuidedReadContext {
	ram := int64(4096)
	count := int64(2)
	return GuidedReadContext{
		Target: "dev", Complete: false, SnapshotRevision: "private-revision",
		Footprint: application.Footprint{
			Version: application.FootprintVersion, Target: "dev",
			Core: application.CoreUnknown, Complete: false,
			Existing: []application.ProviderFootprint{{
				InstanceID: "database", State: application.ProviderRunning,
				Dependencies: []string{"storage"},
				MemoryBytes: application.Quantity{Value: &ram, Classification: "measured", Source: "observer"},
				Containers: application.Quantity{Value: &count, Classification: "estimated", Source: "provider-plan"},
			}, {InstanceID: "storage", State: application.ProviderUnverifiable}},
		},
	}
}

func TestAppFootprintShowSameAuthorityHumanAndJSON(t *testing.T) {
	view := footprintShowFixture()
	snapshot := view.Footprint
	readerCalls := 0
	read := func(context.Context) (GuidedReadContext, error) {
		readerCalls++
		return view, nil
	}
	var human, machine bytes.Buffer
	for _, test := range []struct {
		buffer *bytes.Buffer
		json bool
	}{{&human, false}, {&machine, true}} {
		err := appFootprintShow(context.Background(), test.buffer, read,
			appFootprintShowOptions{Target: "dev", JSON: test.json, Details: true})
		if err != nil {
			t.Fatal(err)
		}
	}
	var decoded application.Footprint
	if err := json.Unmarshal(machine.Bytes(), &decoded); err != nil {
			t.Fatal(err)
		}
	if decoded.Version != snapshot.Version || decoded.Core != snapshot.Core || decoded.Complete != snapshot.Complete ||
		len(decoded.Existing) != len(snapshot.Existing) || decoded.Existing[0].MemoryBytes.Classification != "measured" ||
		decoded.Existing[0].Containers.Classification != "estimated" { t.Fatalf("JSON lost authoritative facts: %+v", decoded) }
	for _, fact := range []string{"database", "storage", "4096 bytes (measured)", "2 (estimated)",
		"Resolution: incomplete", "Core: unknown", "Dependencies: storage"} {
		if !strings.Contains(human.String(), fact) {
			t.Errorf("human missing %q", fact)
		}
	}
	if readerCalls != 2 {
			t.Fatalf("read calls: %d", readerCalls)
		}
	if view.Footprint.Existing[0].InstanceID != snapshot.Existing[0].InstanceID {
			t.Fatal("mutated footprint")
		}
	for _, output := range []string{human.String(), machine.String()} {
		if strings.Contains(output, "private-revision") {
			t.Fatal("leaked context metadata")
		}
	}
}

func TestAppFootprintShowEmptyAndPartial(t *testing.T) {
	for _, complete := range []bool{true, false} {
		view := GuidedReadContext{Target: "dev", Complete: complete,
			Footprint: application.Footprint{Version: application.FootprintVersion,
				Target: "dev", Core: application.CoreNotRequired, Complete: complete}}
		var h, j bytes.Buffer
		if err := appFootprintShowContext(&h, view, appFootprintShowOptions{Target: "dev"}); err != nil { t.Fatal(err) }
		if err := appFootprintShowContext(&j, view, appFootprintShowOptions{Target: "dev", JSON: true}); err != nil { t.Fatal(err) }
		if !strings.Contains(h.String(), "No provider realizations selected.") {
			t.Fatal(h.String())
		}
		if complete && !strings.Contains(h.String(), "Resolution: complete") {
			t.Fatal(h.String())
		}
		if !complete && !strings.Contains(h.String(), "Resolution: incomplete") {
			t.Fatal(h.String())
		}
		var actual application.Footprint
		if err := json.Unmarshal(j.Bytes(), &actual); err != nil {
			t.Fatal(err)
		}
		if actual.Complete != complete || actual.Core != application.CoreNotRequired {
			t.Fatalf("%+v", actual)
		}
	}
}

func TestAppFootprintShowWrongTargetDoesNotWrite(t *testing.T) {
	for _, target := range []string{"prod", ""} {
		var out bytes.Buffer
		err := appFootprintShowContext(&out, footprintShowFixture(), appFootprintShowOptions{Target: target, JSON: true})
		if err == nil || !strings.Contains(err.Error(), "wrong_target") || out.Len() != 0 {
			t.Fatalf("err=%v bytes=%q", err, out.String())
		}
	}
}

func TestAppFootprintShowDeniedAndSourceMissingRedacted(t *testing.T) {
	for _, kind := range []string{"denied", "source_missing", "wrong_target", "unverifiable", "unexpected"} {
		calls := 0
		var out bytes.Buffer
		err := appFootprintShow(context.Background(), &out,
			func(context.Context) (GuidedReadContext, error) {
				calls++
				return GuidedReadContext{}, &GuidedReadError{Kind: kind, Cause: errors.New("/secret/path?token=private")}
			}, appFootprintShowOptions{Target: "dev", JSON: true})
		if err == nil || calls != 1 || out.Len() != 0 || strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "private") {
			t.Fatalf("unsafe error: %v, output=%q", err, out.String())
		}
		if kind == "unexpected" && err.Error() != "footprint: unavailable" {
			t.Fatal(err)
		}
	}
}

func TestAppFootprintShowReadOnlyAndWriterError(t *testing.T) {
	view := footprintShowFixture()
	original, err := json.Marshal(view)
	if err != nil {
			t.Fatal(err)
		}
	var out bytes.Buffer
	if err := appFootprintShowContext(&out, view, appFootprintShowOptions{Target: "dev", Details: true}); err != nil { t.Fatal(err) }
	after, err := json.Marshal(view)
	if err != nil {
			t.Fatal(err)
		}
	if !bytes.Equal(original, after) {
			t.Fatal("mutated read context")
		}
}
