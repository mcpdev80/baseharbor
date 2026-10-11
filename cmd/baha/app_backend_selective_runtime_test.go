package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func TestSelectiveBackendRuntimeAddRemoveReAdd(t *testing.T) {
	if os.Getenv("BASEHARBOR_BACKEND_LIFECYCLE_ACCEPTANCE") != "1" {
		t.Skip("requires isolated exact-SHA Docker/rootless Podman backend acceptance")
	}
	for _, service := range []string{"sql", "cache"} {
		if !t.Run(service, func(t *testing.T) { qualifySelectiveBackendLifecycle(t, service) }) {
			// Partial native cleanup failure must not contaminate or falsely
			// qualify a subsequent capability on the same isolated host.
			break
		}
	}
}

func qualifySelectiveBackendLifecycle(t *testing.T, service string) {
	ctx, cancel := context.WithTimeout(machineNoninteractiveContext(context.Background()), 10*time.Minute)
	defer cancel()
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	target := configureTestTarget(t)
	{
		cfg, err := deployment.LoadConfig()
		if err != nil {
			t.Fatal(err)
		}
		definition := cfg.Targets[target.Name]
		if os.Getenv("BASEHARBOR_TEST_RUNTIME") == "podman" {
			definition.Runtime.Provider = "podman"
		}
		delete(cfg.Targets, target.Name)
		cfg.DefaultTarget = "backend-" + service
		cfg.Targets[cfg.DefaultTarget] = definition
		if err := cfg.Save(); err != nil {
			t.Fatal(err)
		}
		target, err = cfg.ResolveTarget("", "")
		if err != nil {
			t.Fatal(err)
		}
	}
	provider, err := detectRuntimeForTarget(ctx, target)
	if err != nil {
		t.Fatal(err)
	}
	assertCorelessHost(t, ctx, provider, target)
	t.Chdir(t.TempDir())
	if err := os.WriteFile("compose.yaml", []byte(corelessHTTPWorkload), 0600); err != nil {
		t.Fatal(err)
	}
	writeIntent := func(enabled bool) {
		content := fmt.Sprintf("version: 1\napp:\n  name: selective-%s\n  environment: dev\nservices:\n  %s: %t\n", service, service, enabled)
		if err := os.WriteFile("baseharbor.yaml", []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	writeIntent(true)
	var output bytes.Buffer
	if err := runWithIO(ctx, []string{"app", "init", "--quick", "--json"}, &output, &output); err != nil {
		t.Fatalf("initialize selected backend: %v\n%s", err, output.String())
	}
	initialized, err := resolveApplication(ctx, application.Store{}, nil, "plan")
	if err != nil {
		t.Fatal(err)
	}
	var inventory application.SharedBackendInventory
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(machineNoninteractiveContext(context.Background()), 2*time.Minute)
		defer cleanupCancel()
		// Apply can fail after materializing a canonical provider but before
		// returning readiness. Inspect its existing owner instead of relying
		// on the successful-loop snapshot or guessing container names.
		partial, inspectErr := application.ExistingSharedBackendInventoryAt(initialized.TargetStateRoot, target.Name, "core")
		if inspectErr == nil {
			inventory = partial
		} else if !os.IsNotExist(inspectErr) {
			t.Errorf("partial backend cleanup ownership inspection: %v", inspectErr)
		}
		if err := runWithIO(cleanupCtx, []string{"app", "destroy", "--yes"}, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
			t.Errorf("isolated backend app cleanup: %v", err)
		}
		if inventory.Files.Project != "" {
			native, ok := provider.(bhruntime.RuntimeServiceRetirementProvider)
			if !ok {
				t.Error("isolated native backend retirement unsupported")
				return
			}
			selected := []bhruntime.ProjectResource{}
			for _, resource := range inventory.Resources {
				if resource.Kind == "container" {
					selected = append(selected, resource)
				}
			}
			if _, err := bhruntime.RetireOwnedServices(cleanupCtx, native, inventory.Files.Project, inventory.Resources, selected); err != nil {
				t.Errorf("isolated backend runtime cleanup: %v", err)
			}
		}
		if containers, err := provider.ListRuntimeContainers(cleanupCtx); err != nil || len(containers) != 0 {
			t.Errorf("isolated backend cleanup leaves containers=%d inspection=%v", len(containers), err)
		}
	})
	var originalEnv, originalConsumer [32]byte
	var volumes []bhruntime.ProjectResource
	for phase, enabled := range []bool{true, false, true} {
		writeIntent(enabled)
		applySelectiveIdentityIntent(t, ctx)
		resolved, err := resolveApplication(ctx, application.Store{}, nil, "status")
		if err != nil {
			t.Fatal(err)
		}
		status, _, err := collectResolvedApplicationStatus(ctx, resolved)
		if err != nil || !status.Ready {
			t.Fatalf("selected backend phase=%d readiness: %+v %v", phase, status, err)
		}
		if resolved.Manifest.ApplicationID != initialized.Manifest.ApplicationID {
			t.Fatal("backend change replaced stable application identity")
		}
		if resolved.DeploymentRecord == nil || !resolved.DeploymentRecord.Observed.Ready || resolved.DeploymentRecord.Applied.GeneratedState["previous_intent"] != "" {
			t.Fatal("successful backend apply lacks a verified converged deployment record")
		}
		inventory, err = application.ExistingSharedBackendInventoryAt(resolved.TargetStateRoot, target.Name, "core")
		if err != nil || inventory.CoreSQL != nil {
			t.Fatalf("selected backend unexpectedly consumes management Core SQL: %v", err)
		}
		data, err := os.ReadFile(inventory.Files.Env)
		if err != nil {
			t.Fatal(err)
		}
		actual, err := provider.InspectProjectResources(ctx, inventory.Files.Project, inventory.Resources)
		if err != nil {
			t.Fatal(err)
		}
		if phase == 0 {
			originalEnv = sha256.Sum256(data)
			for _, resource := range actual {
				if resource.Kind == "volume" {
					volumes = append(volumes, resource)
				}
			}
			if len(volumes) == 0 {
				t.Fatal("selected backend has no inspected data volume")
			}
		} else if sha256.Sum256(data) != originalEnv {
			t.Fatal("backend change replaced retained administrator credentials/configuration")
		}
		for _, volume := range volumes {
			if !application.ResourceNamedExists(actual, volume) {
				t.Fatal("backend removal lost retained owned data volume")
			}
		}
		if enabled {
			qualifySelectedBackendData(t, ctx, provider, resolved, inventory, service, phase, &originalConsumer)
		}
		assertSelectedBackendNativeInventory(t, ctx, provider, resolved, inventory, service, phase, enabled, len(volumes))
		if data, err := os.ReadFile("baseharbor.yaml"); err != nil || strings.Contains(string(data), initialized.Manifest.ApplicationID) {
			t.Fatal("backend lifecycle wrote a developer-supplied ID into authored intent")
		}
	}
}

func assertSelectedBackendNativeInventory(t *testing.T, ctx context.Context, provider bhruntime.RuntimeProvider, resolved resolvedApplication, inventory application.SharedBackendInventory, service string, phase int, enabled bool, retainedVolumes int) {
	t.Helper()
	kind := capability.ProviderPostgreSQL
	if service == "cache" {
		kind = capability.ProviderValkey
	}
	for other, services := range inventory.Services {
		if other != kind && len(services) > 0 {
			t.Fatalf("single backend materialized another provider kind=%s", other)
		}
	}
	containers, err := provider.ListRuntimeContainers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	appFiles := application.RuntimeFilesFor(resolved.Store, resolved.Manifest)
	running := 0
	for _, container := range containers {
		matched := false
		for _, resource := range inventory.Resources {
			if resource.Kind == "container" && bhruntime.ContainerMatchesProjectResource(provider.Kind(), container, inventory.Files.Project, resource) {
				matched = true
				if container.Running {
					running++
				}
				break
			}
		}
		workload := bhruntime.ProjectResource{Kind: "container", Name: appFiles.Project + "-api-1"}
		if !matched && !bhruntime.ContainerMatchesProjectResource(provider.Kind(), container, appFiles.Project, workload) {
			t.Fatalf("unrequested runtime realization: project=%s service=%s", container.Project, container.Service)
		}
	}
	if (enabled && running == 0) || (!enabled && running != 0) {
		t.Fatalf("backend=%s phase=%d required=%t running=%d", service, phase, enabled, running)
	}
	if provider.Kind() == bhruntime.ProviderPodman {
		home, err := os.UserHomeDir()
		if err != nil {
			t.Fatal(err)
		}
		for _, selected := range inventory.Services[kind] {
			// Native rootless Quadlet uses the real user's unit directory,
			// independently of the isolated BaseHarbor config/Target state.
			path := filepath.Join(home, ".config", "containers", "systemd", inventory.Files.Project+"-"+selected+".container")
			_, err := os.Lstat(path)
			if (enabled && err != nil) || (!enabled && !os.IsNotExist(err)) {
				t.Fatalf("backend=%s phase=%d boot definition does not match selected need: %v", service, phase, err)
			}
		}
		t.Logf("rootless Podman backend=%s phase=%d actual container boot definitions match selected need=%t", service, phase, enabled)
	}
	t.Logf("runtime=%s backend=%s phase=%d required=%t selected running=%d actual total=%d retained owned data volumes=%d", provider.Kind(), service, phase, enabled, running, len(containers), retainedVolumes)
}
