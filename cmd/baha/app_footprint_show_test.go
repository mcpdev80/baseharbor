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
	database := application.ProviderFootprint{
		InstanceID: "database",
		State:      application.ProviderRunning,
		Dependencies: []string{"storage"},
		MemoryBytes: application.Quantity{
			Value: &ram, Classification: "measured", Source: "observer",
		},
		Containers: application.Quantity{
			Value: &count, Classification: "estimated", Source: "provider-plan",
		},
	}
	storage := application.ProviderFootprint{
		InstanceID: "storage",
		State:      application.ProviderUnverifiable,
	}
	return GuidedReadContext{
		Target:           "dev",
		Complete:         false,
		SnapshotRevision: "private-revision",
		Footprint: application.Footprint{
			Version:  application.FootprintVersion,
			Target:   "dev",
			Core:     application.CoreUnknown,
			Complete: false,
			Existing: []application.ProviderFootprint{database, storage},
		},
	}
}

func TestAppFootprintShowSameAuthorityHumanAndJSON(t *testing.T) {
	view := footprintShowFixture()
	readerCalls := 0
	read := func(context.Context) (GuidedReadContext, error) {
		readerCalls++
		return view, nil
	}
	var human, machine bytes.Buffer
	if err := appFootprintShow(context.Background(), &human, read, appFootprintShowOptions{
		Target: "dev", Details: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := appFootprintShow(context.Background(), &machine, read, appFootprintShowOptions{
		Target: "dev", JSON: true,
	}); err != nil {
		t.Fatal(err)
	}
	var decoded application.Footprint
	if err := json.Unmarshal(machine.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Version != view.Footprint.Version ||
		decoded.Core != view.Footprint.Core ||
		decoded.Complete != view.Footprint.Complete ||
		len(decoded.Existing) != 2 ||
		decoded.Existing[0].MemoryBytes.Classification != "measured" ||
		decoded.Existing[0].Containers.Classification != "estimated" {
		t.Fatalf("JSON lost footprint evidence: %+v", decoded)
	}
	for _, fact := range []string{
		"database", "storage", "4096 bytes (measured)",
		"2 (estimated)", "Resolution: incomplete", "Core: unknown",
		"Dependencies: storage",
	} {
		if !strings.Contains(human.String(), fact) {
			t.Errorf("human output missing %q", fact)
		}
	}
	if readerCalls != 2 {
		t.Fatalf("expected one read per handler, got %d", readerCalls)
	}
	for _, output := range []string{human.String(), machine.String()} {
		if strings.Contains(output, "private-revision") {
			t.Fatal("leaked GuidedReadContext metadata")
		}
	}
}

func TestAppFootprintShowEmptyAndPartial(t *testing.T) {
	for _, complete := range []bool{true, false} {
		view := GuidedReadContext{
			Target: "dev", Complete: complete,
			Footprint: application.Footprint{
				Version:  application.FootprintVersion,
				Target:   "dev",
				Core:     application.CoreNotRequired,
				Complete: complete,
			},
		}
		var human, machine bytes.Buffer
		if err := appFootprintShowContext(&human, view, appFootprintShowOptions{Target: "dev"}); err != nil {
			t.Fatal(err)
		}
		if err := appFootprintShowContext(&machine, view, appFootprintShowOptions{Target: "dev", JSON: true}); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(human.String(), "No provider realizations selected.") {
			t.Fatal(human.String())
		}
		want := "Resolution: incomplete"
		if complete {
			want = "Resolution: complete"
		}
		if !strings.Contains(human.String(), want) {
			t.Fatal(human.String())
		}
		var actual application.Footprint
		if err := json.Unmarshal(machine.Bytes(), &actual); err != nil {
			t.Fatal(err)
		}
		if actual.Complete != complete || actual.Core != application.CoreNotRequired {
			t.Fatalf("unexpected JSON: %+v", actual)
		}
	}
}

func TestAppFootprintShowWrongTargetDoesNotWrite(t *testing.T) {
	for _, target := range []string{"prod", ""} {
		var out bytes.Buffer
		err := appFootprintShowContext(&out, footprintShowFixture(), appFootprintShowOptions{
			Target: target, JSON: true,
		})
		if err == nil || !strings.Contains(err.Error(), "wrong_target") || out.Len() != 0 {
			t.Fatalf("err=%v bytes=%q", err, out.String())
		}
	}
}

func TestAppFootprintShowDeniedAndSourceMissingRedacted(t *testing.T) {
	for _, kind := range []string{"denied", "source_missing", "wrong_target", "unverifiable", "unexpected"} {
		calls := 0
		var out bytes.Buffer
		read := func(context.Context) (GuidedReadContext, error) {
			calls++
			return GuidedReadContext{}, &GuidedReadError{
				Kind: kind, Cause: errors.New("/secret/path?token=private"),
			}
		}
		err := appFootprintShow(context.Background(), &out, read, appFootprintShowOptions{
			Target: "dev", JSON: true,
		})
		if err == nil || calls != 1 || out.Len() != 0 ||
			strings.Contains(err.Error(), "secret") ||
			strings.Contains(err.Error(), "private") {
			t.Fatalf("unsafe error: %v, output=%q", err, out.String())
		}
		if kind == "unexpected" && err.Error() != "footprint: unavailable" {
			t.Fatal(err)
		}
	}
}

func TestAppFootprintShowReadOnly(t *testing.T) {
	view := footprintShowFixture()
	original, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := appFootprintShowContext(&out, view, appFootprintShowOptions{
		Target: "dev", Details: true,
	}); err != nil {
		t.Fatal(err)
	}
	after, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(original, after) {
		t.Fatal("read-only handler mutated context")
	}
}
