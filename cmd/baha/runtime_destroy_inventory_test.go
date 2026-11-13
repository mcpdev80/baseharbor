package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func TestFullDestroyInventoryMissingRuntimeExplainsSafeNextStep(t *testing.T) {
	err := fullDestroyInventoryError(deployment.ResolvedTarget{Name: "podman-test", RuntimeProvider: "podman"}, fmt.Errorf("detect provider: %w", bhruntime.ErrRuntimeNotFound))
	var detail *machine.Error
	if !errors.As(err, &detail) || detail.Code != machine.ErrorCapabilityMissing {
		t.Fatalf("missing runtime was not reported as a capability issue: %v", err)
	}
	if !strings.Contains(detail.Message, "podman-test") || !strings.Contains(detail.Next, "install the podman runtime") || !strings.Contains(detail.Next, "No resources were removed") {
		t.Fatalf("missing runtime error lacks a safe, actionable next step: %+v", detail)
	}
}

func TestFullDestroyInventoryKeepsReachableTargetsWhenPeerRuntimeIsUnavailable(t *testing.T) {
	targets := []deployment.ResolvedTarget{{Name: "local", RuntimeProvider: "docker"}, {Name: "podman-peer", RuntimeProvider: "podman"}}
	runtime := &inventoryTestRuntime{resources: map[string][]bhruntime.ProjectResource{"bh-local-shared": {{Kind: "container", Name: "owned"}}}}
	plans, err := collectFullDestroyInventoryUsing(t.Context(), targets, func(_ context.Context, target deployment.ResolvedTarget, _ bool) (targetDestroyInventory, error) {
		if target.Name == "podman-peer" {
			return targetDestroyInventory{Target: target}, fmt.Errorf("detect provider: %w", bhruntime.ErrRuntimeNotFound)
		}
		return collectTargetDestroyInventoryWithRuntime(t.Context(), target, runtime, false)
	})
	if err != nil {
		t.Fatalf("one unavailable Target aborted the independent inventory: %v", err)
	}
	if len(plans) != 2 || plans[0].InventoryError != "" || plans[0].Runtime == nil || plans[1].InventoryError == "" || plans[1].Runtime != nil {
		t.Fatalf("unexpected partial inventory: %+v", plans)
	}
	var out bytes.Buffer
	plans[0].render(&out)
	plans[1].render(&out)
	if !strings.Contains(out.String(), "owned resource: target=local") || !strings.Contains(out.String(), "PRESERVED target=podman-peer") {
		t.Fatalf("plan does not show reachable and preserved Targets: %s", out.String())
	}
	if len(runtime.deleted) != 0 {
		t.Fatal("inventory preview mutated reachable runtime")
	}
}

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

func TestFullDestroyRemovesAcknowledgedRecoveryFileWithoutReadingContents(t *testing.T) {
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
	if !report.Destroyed || report.Preview || !report.All || len(report.Preserved) != 0 {
		t.Fatalf("incorrect structured cleanup report: %#v", report)
	}
	if strings.Contains(out.String(), "private-recovery-material") || !strings.Contains(out.String(), "external recovery file") {
		t.Fatal("recovery secret leaked into cleanup inventory")
	}
	if _, err := os.Lstat(recovery); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("acknowledged recovery file was not removed: %v", err)
	}
}

func TestDestroyInventoryIncludesOwnedCoreIdentityWithoutRunningContainers(t *testing.T) {
	identityProject := bhruntime.SharedProjectName("local-core")
	coreProject := bhruntime.SharedProjectName("local")
	foreignProject := bhruntime.SharedProjectName("other-core")
	runtime := &inventoryTestRuntime{resources: map[string][]bhruntime.ProjectResource{
		identityProject: {{Kind: "container", Name: "keycloak"}, {Kind: "container", Name: "keycloak-access"}},
		coreProject:     {{Kind: "container", Name: "postgres"}},
		foreignProject:  {{Kind: "container", Name: "other-keycloak"}},
	}}
	for _, sharedOnly := range []bool{false, true} {
		t.Run(fmt.Sprintf("sharedOnly=%t", sharedOnly), func(t *testing.T) {
			plan, err := collectTargetDestroyInventoryWithRuntime(context.Background(),
				deployment.ResolvedTarget{Name: "local"}, runtime, sharedOnly)
			if err != nil {
				t.Fatal(err)
			}
			if len(plan.Projects[identityProject]) != 2 {
				t.Fatalf("stopped Keycloak project missing from destroy inventory: %+v", plan.Projects)
			}
			if _, foreign := plan.Projects[foreignProject]; foreign {
				t.Fatal("destroy inventory included foreign target identity")
			}
			var output bytes.Buffer
			plan.render(&output)
			if !strings.Contains(output.String(), "keycloak-access") || strings.Contains(output.String(), "other-keycloak") {
				t.Fatalf("destroy preview lacks Core identity or includes foreign identity: %s", output.String())
			}
		})
	}
	plan, err := collectTargetDestroyInventoryWithRuntime(context.Background(),
		deployment.ResolvedTarget{Name: "local"}, runtime, false)
	if err != nil {
		t.Fatal(err)
	}
	var results []fullDestroyResult
	plan.cleanup(context.Background(), &results)
	if countFullDestroyBlockers(results) != 0 || len(runtime.resources[identityProject]) != 0 {
		t.Fatalf("owned stopped Keycloak not destroyed safely: results=%+v remaining=%+v", results, runtime.resources[identityProject])
	}
	if len(runtime.resources[foreignProject]) != 1 {
		t.Fatal("cleanup affected foreign target identity resources")
	}
}
