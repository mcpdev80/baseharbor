package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/deployment"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

type inventoryTestRuntime struct {
	bhruntime.RuntimeProvider
	resources  map[string][]bhruntime.ProjectResource
	containers []bhruntime.RuntimeContainer
	fail       bool
	retain     bool
	deleted    []string
	destroyErr error
}

func (r *inventoryTestRuntime) ListRuntimeContainers(context.Context) ([]bhruntime.RuntimeContainer, error) {
	return r.containers, nil
}
func (r *inventoryTestRuntime) ListOwnedProjectResources(_ context.Context, project string) ([]bhruntime.ProjectResource, error) {
	if r.fail {
		return nil, errors.New("ownership inventory unavailable")
	}
	return append([]bhruntime.ProjectResource{}, r.resources[project]...), nil
}
func (r *inventoryTestRuntime) DestroyOwnedProjectResources(_ context.Context, project string, _ []bhruntime.ProjectResource) error {
	r.deleted = append(r.deleted, project)
	if r.destroyErr != nil {
		return r.destroyErr
	}
	if !r.retain {
		r.resources[project] = nil
	}
	return nil
}

func TestDestroyInventoryResourceMissingErrorCannotHideRemainingResources(t *testing.T) {
	for _, message := range []string{
		"remove owned container vanished: no such container",
		"remove owned volume vanished: no such volume",
		"remove owned network vanished: no such network",
		"runtime storage database not found",
		"unable to find runtime command",
	} {
		t.Run(message, func(t *testing.T) {
			remaining := bhruntime.ProjectResource{Kind: "volume", Name: "retained-data"}
			planned := []bhruntime.ProjectResource{
				{Kind: "container", Name: "vanished"}, remaining,
			}
			runtime := &inventoryTestRuntime{
				resources:  map[string][]bhruntime.ProjectResource{"owned-project": {remaining}},
				destroyErr: errors.New(message),
			}
			plan := targetDestroyInventory{
				Target: deployment.ResolvedTarget{Name: "local"}, Runtime: runtime,
				Projects: map[string][]bhruntime.ProjectResource{"owned-project": planned},
			}
			var results []fullDestroyResult
			plan.cleanup(context.Background(), &results)
			if countFullDestroyBlockers(results) == 0 {
				t.Fatalf("unverified resource removal did not block completion: %+v", results)
			}
			for _, result := range results {
				if result.Status != "FAILED" || !strings.Contains(result.Detail, message) {
					t.Fatalf("runtime failure was replaced by successful absence evidence: %+v", results)
				}
			}
			if got := runtime.resources["owned-project"]; len(got) != 1 || got[0] != remaining {
				t.Fatal("failed destroy lost retained owned data")
			}
		})
	}
}

func TestDestroyInventoryAcceptsAbsenceVerifiedByRuntimeOwner(t *testing.T) {
	runtime := &inventoryTestRuntime{resources: map[string][]bhruntime.ProjectResource{}}
	plan := targetDestroyInventory{
		Target: deployment.ResolvedTarget{Name: "local"}, Runtime: runtime,
		Projects: map[string][]bhruntime.ProjectResource{"owned-project": {
			{Kind: "container", Name: "already-absent"},
			{Kind: "volume", Name: "already-absent-data"},
		}},
	}
	var results []fullDestroyResult
	plan.cleanup(context.Background(), &results)
	if countFullDestroyBlockers(results) != 0 || len(results) != 2 {
		t.Fatalf("runtime-verified absence did not complete: %+v", results)
	}
	for _, result := range results {
		if result.Status != "REMOVED" {
			t.Fatalf("unexpected runtime-verified absence result: %+v", result)
		}
	}
}
func TestDestroyInventoryPreviewsOlderOwnedVolumeAndExcludesForeignTarget(t *testing.T) {
	runtime := &inventoryTestRuntime{resources: map[string][]bhruntime.ProjectResource{
		"bh-local-shared": {{Kind: "volume", Name: "bh-local-shared_runtime-resource-state"}, {Kind: "container", Name: "provider"}},
		"bh-other-shared": {{Kind: "container", Name: "foreign"}},
	}, containers: []bhruntime.RuntimeContainer{{Project: "bh-local-shared", Name: "provider"}, {Project: "bh-other-shared", Name: "foreign"}}}
	plan, err := collectTargetDestroyInventoryWithRuntime(context.Background(), deployment.ResolvedTarget{Name: "local"}, runtime, false)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	plan.render(&out)
	if !strings.Contains(out.String(), "bh-local-shared_runtime-resource-state") || strings.Contains(out.String(), "foreign") {
		t.Fatalf("incomplete or foreign preview: %s", out.String())
	}
	if len(runtime.deleted) != 0 {
		t.Fatal("preview mutated runtime")
	}
	results := []fullDestroyResult{}
	plan.cleanup(context.Background(), &results)
	if countFullDestroyBlockers(results) != 0 || len(results) != 2 || len(runtime.resources["bh-other-shared"]) != 1 {
		t.Fatalf("cleanup crossed ownership or lost concrete evidence: %v", results)
	}
}
func TestDestroyInventoryFailureAndResidualResourcesBlockCompletion(t *testing.T) {
	runtime := &inventoryTestRuntime{fail: true}
	if _, err := collectTargetDestroyInventoryWithRuntime(context.Background(), deployment.ResolvedTarget{Name: "local"}, runtime, false); err == nil {
		t.Fatal("unavailable ownership accepted")
	}
	if len(runtime.deleted) != 0 {
		t.Fatal("inventory failure mutated resources")
	}
	runtime = &inventoryTestRuntime{retain: true, resources: map[string][]bhruntime.ProjectResource{"bh-local-shared": {{Kind: "volume", Name: "residual"}}}}
	plan, err := collectTargetDestroyInventoryWithRuntime(context.Background(), deployment.ResolvedTarget{Name: "local"}, runtime, true)
	if err != nil {
		t.Fatal(err)
	}
	results := []fullDestroyResult{}
	plan.cleanup(context.Background(), &results)
	if countFullDestroyBlockers(results) == 0 {
		t.Fatal("residual resource reported removed")
	}
}

func TestFullDestroyJSONReportsPreservedRecoveryWithoutReadingContents(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	root, err := deployment.TargetStateRoot("orphan")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	recovery, err := defaultTargetRecoveryFile("orphan")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(recovery), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(recovery, []byte("private-recovery-material"), 0600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := runtimeDestroyCommand(currentInstallationDestroyConsent(t), []string{"--all", "--yes", "-o", "json"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	var report destroyReport
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if !report.Destroyed || report.Preview || !report.All || len(report.Preserved) != 1 || report.Preserved[0].Status != "PRESERVED" {
		t.Fatalf("incorrect structured cleanup report: %#v", report)
	}
	if strings.Contains(out.String(), "private-recovery-material") {
		t.Fatal("recovery secret leaked into cleanup inventory")
	}
	if raw, err := os.ReadFile(recovery); err != nil || string(raw) != "private-recovery-material" {
		t.Fatal("external recovery file was changed")
	}
}
