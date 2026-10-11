package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/applicationsecret"
	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/runtimebroker"
)

func TestSelectiveSecretsRuntimeAddRemoveReAdd(t *testing.T) {
	if os.Getenv("BASEHARBOR_SECRETS_LIFECYCLE_ACCEPTANCE") != "1" {
		t.Skip("requires isolated exact-SHA Docker/rootless Podman Secrets acceptance with matching runtime image")
	}
	ctx, cancel := context.WithTimeout(machineNoninteractiveContext(context.Background()), 15*time.Minute)
	defer cancel()
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	target := configureTestTarget(t)
	cfg, err := deployment.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	definition := cfg.Targets[target.Name]
	if os.Getenv("BASEHARBOR_TEST_RUNTIME") == "podman" {
		definition.Runtime.Provider = "podman"
	}
	delete(cfg.Targets, target.Name)
	cfg.DefaultTarget = "selected-secrets"
	cfg.Targets[cfg.DefaultTarget] = definition
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	target, err = cfg.ResolveTarget("", "")
	if err != nil {
		t.Fatal(err)
	}
	provider, err := detectRuntimeForTarget(ctx, target)
	if err != nil {
		t.Fatal(err)
	}
	assertCorelessHost(t, ctx, provider, target)
	root, err := targetRuntimeStateRoot(target)
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(t.TempDir())
	if err := os.WriteFile("compose.yaml", []byte(corelessHTTPWorkload), 0600); err != nil {
		t.Fatal(err)
	}
	writeIntent := func(enabled bool) {
		content := fmt.Sprintf("version: 1\napp:\n  name: selective-secrets\n  environment: dev\nworkload:\n  components:\n    - api\nservices:\n  secrets: %t\n", enabled)
		if enabled {
			content += "secrets:\n  required:\n    - name: API_TOKEN\n      generate:\n        type: hex\n        bytes: 32\n"
		}
		if err := os.WriteFile("baseharbor.yaml", []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	writeIntent(true)
	var output bytes.Buffer
	if err := runWithIO(ctx, []string{"app", "init", "--quick", "--json"}, &output, &output); err != nil {
		t.Fatalf("initialize Secrets-only workload: %v\n%s", err, output.String())
	}
	initialized, err := resolveApplication(ctx, application.Store{}, nil, "plan")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(machineNoninteractiveContext(context.Background()), 3*time.Minute)
		defer stop()
		inventory, inspectErr := bhruntime.ExistingControlPlaneRuntimeInventory(root, bhruntime.SharedProjectName(target.Name))
		if inspectErr != nil && !os.IsNotExist(inspectErr) {
			t.Errorf("partial Secrets owner inspection: %v", inspectErr)
		}
		if current, err := resolveApplication(cleanup, application.Store{}, nil, "status"); err == nil {
			appFiles := application.RuntimeFilesFor(current.Store, current.Manifest)
			if _, err := runtimebroker.RetireContainer(cleanup, provider, current.Manifest, appFiles); err != nil && !os.IsNotExist(err) {
				t.Errorf("isolated Secrets broker retirement: %v", err)
			}
		}
		if err := runWithIO(cleanup, []string{"app", "destroy", "--yes"}, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
			t.Errorf("isolated Secrets workload cleanup: %v", err)
		}
		if inspectErr == nil {
			native, ok := provider.(bhruntime.RuntimeServiceRetirementProvider)
			if !ok {
				t.Error("native Secrets retirement unsupported")
				return
			}
			selected := []bhruntime.ProjectResource{}
			for _, resource := range inventory.Resources {
				if resource.Kind == "container" {
					selected = append(selected, resource)
				}
			}
			if _, err := bhruntime.RetireOwnedServices(cleanup, native, inventory.Files.Project, inventory.Resources, selected); err != nil {
				t.Errorf("isolated Secrets module cleanup: %v", err)
			}
		}
		if containers, err := provider.ListRuntimeContainers(cleanup); err != nil || len(containers) != 0 {
			t.Errorf("isolated Secrets cleanup leaves containers=%d inspection=%v", len(containers), err)
		}
	})
	var originalSecret, originalEnv [32]byte
	var volumes []bhruntime.ProjectResource
	for phase, enabled := range []bool{true, false, true} {
		writeIntent(enabled)
		applySelectiveIdentityIntent(t, ctx)
		current, err := resolveApplication(ctx, application.Store{}, nil, "status")
		if err != nil {
			t.Fatal(err)
		}
		status, _, err := collectResolvedApplicationStatus(ctx, current)
		if err != nil || !status.Ready || current.Manifest.ApplicationID != initialized.Manifest.ApplicationID {
			t.Fatalf("Secrets phase=%d readiness/identity: %+v %v", phase, status, err)
		}
		if current.DeploymentRecord == nil || !current.DeploymentRecord.Observed.Ready || current.DeploymentRecord.Applied.GeneratedState["previous_intent"] != "" {
			t.Fatal("Secrets apply lacks verified converged deployment record")
		}
		// The first apply allocates the canonical deployment owner. Never use
		// an earlier read's unpersisted deployment proposal for secret access,
		// native inventory or cleanup.
		appFiles := application.RuntimeFilesFor(current.Store, current.Manifest)
		if _, err := coreinstallation.Load(root); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("Secrets capability allocated independent management installation: %v", err)
		}
		inventory, err := bhruntime.ExistingControlPlaneRuntimeInventory(root, bhruntime.SharedProjectName(target.Name))
		if err != nil {
			t.Fatal(err)
		}
		actual, err := provider.InspectProjectResources(ctx, inventory.Files.Project, inventory.Resources)
		if err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(inventory.Files.Env)
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
				t.Fatal("selected Secrets dependency has no actual owned data volume")
			}
		} else if originalEnv != sha256.Sum256(data) {
			t.Fatal("Secrets change replaced protected provider credential/configuration material")
		}
		for _, volume := range volumes {
			if !application.ResourceNamedExists(actual, volume) {
				t.Fatal("Secrets removal lost retained owned data volume")
			}
		}
		if enabled {
			service := applicationsecret.NewForApplicationRuntime(current.Store, provider, inventory.Files, current.Manifest, appFiles)
			value, err := service.Get(ctx, current.Manifest.Name, "API_TOKEN")
			if err != nil || len(value) == 0 {
				t.Fatal("generated application secret is not actually readable through the owned application scope")
			}
			if phase == 0 {
				originalSecret = sha256.Sum256(value)
			} else if originalSecret != sha256.Sum256(value) {
				t.Fatal("Secrets re-add lost or regenerated retained application secret")
			}
			clear(value)
		}
		containers, err := provider.ListRuntimeContainers(ctx)
		if err != nil {
			t.Fatal(err)
		}
		providerCount, brokerCount := 0, 0
		for _, container := range containers {
			matched := false
			for _, resource := range inventory.Resources {
				if resource.Kind == "container" && bhruntime.ContainerMatchesProjectResource(provider.Kind(), container, inventory.Files.Project, resource) {
					providerCount++
					matched = true
				}
			}
			broker := bhruntime.ProjectResource{Kind: "container", Name: runtimebroker.ProjectNameForRuntime(current.Manifest, appFiles) + "-broker-1"}
			if bhruntime.ContainerMatchesProjectResource(provider.Kind(), container, runtimebroker.ProjectNameForRuntime(current.Manifest, appFiles), broker) {
				brokerCount++
				matched = true
			}
			workload := bhruntime.ProjectResource{Kind: "container", Name: appFiles.Project + "-api-1"}
			if !matched && !bhruntime.ContainerMatchesProjectResource(provider.Kind(), container, appFiles.Project, workload) {
				t.Fatalf("Secrets-only materialized unrequested project=%s service=%s", container.Project, container.Service)
			}
		}
		if (enabled && (providerCount == 0 || brokerCount != 1)) || (!enabled && (providerCount != 0 || brokerCount != 0)) {
			t.Fatalf("Secrets phase=%d need=%t actual provider=%d broker=%d", phase, enabled, providerCount, brokerCount)
		}
		if provider.Kind() == bhruntime.ProviderPodman {
			home, err := os.UserHomeDir()
			if err != nil {
				t.Fatal(err)
			}
			for _, resource := range append(append([]bhruntime.ProjectResource{}, inventory.Resources...), bhruntime.ProjectResource{Kind: "container", Name: runtimebroker.ProjectNameForRuntime(current.Manifest, appFiles) + "-broker-1"}) {
				if resource.Kind != "container" {
					continue
				}
				_, err := os.Lstat(filepath.Join(home, ".config", "containers", "systemd", strings.TrimSuffix(resource.Name, "-1")+".container"))
				if (enabled && err != nil) || (!enabled && !os.IsNotExist(err)) {
					t.Fatalf("Secrets phase=%d actual boot definition differs from selected need: %v", phase, err)
				}
			}
		}
		t.Logf("runtime=%s Secrets phase=%d need=%t actual provider=%d broker=%d total=%d retained owned data volumes=%d; stable system ID and verified deployment", provider.Kind(), phase, enabled, providerCount, brokerCount, len(containers), len(volumes))
	}
	t.Log("generated app secret actually read before removal and after re-add: protected retained value unchanged, no plaintext output")
}
