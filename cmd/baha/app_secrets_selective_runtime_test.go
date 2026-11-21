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
	"github.com/mcpdev80/baseharbor/internal/applicationsecret"
	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/runtimebroker"
)

func TestSelectiveSecretsRuntimeAddRemoveReAdd(t *testing.T) {
	runSelectiveSecretsRuntimeLifecycle(t, false)
}

func TestSharedSecretsRuntimePreservesSiblingAndRetiresLastConsumer(t *testing.T) {
	runSelectiveSecretsRuntimeLifecycle(t, true)
}

func runSelectiveSecretsRuntimeLifecycle(t *testing.T, shared bool) {
	if os.Getenv("BASEHARBOR_SECRETS_LIFECYCLE_ACCEPTANCE") != "1" {
		t.Skip("requires isolated exact-SHA Docker/rootless Podman Secrets acceptance with matching runtime image")
	}
	ctx, cancel := context.WithTimeout(machineNoninteractiveContext(context.Background()), 15*time.Minute)
	defer cancel()
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	// Secrets uses the managed provider registry, which requires a durable
	// installation owner. Seed a synthetic owner for the exact selected runtime,
	// but run no Core services on the host so this exercises the Coreless
	// workload path.
	target := configureTestTargetEmpty(t)
	cfg, err := deployment.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	definition := cfg.Targets[target.Name]
	if os.Getenv("BASEHARBOR_TEST_RUNTIME") == "podman" {
		definition.Runtime.Provider = "podman"
	}
	// Keep the configured Target name so the installation owner remains bound
	// to the Target used by the managed provider registry.
	cfg.Targets[target.Name] = definition
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	target, err = cfg.ResolveTarget(target.Name, "")
	if err != nil {
		t.Fatal(err)
	}
	root := seedProviderRuntimeOwner(t, ctx, target.Name, target.RuntimeProvider, false)
	provider, err := detectRuntimeForTarget(ctx, target)
	if err != nil {
		t.Fatal(err)
	}
	initialOwner, err := coreinstallation.Load(root)
	if err != nil || !initialOwner.Ready || initialOwner.Spec.Target != target.Name {
		t.Fatalf("load synthetic provider-registry owner: %+v %v", initialOwner, err)
	}
	baselineContainers, err := provider.ListRuntimeContainers(ctx)
	if err != nil {
		t.Fatal("inventory isolated Secrets runtime baseline", err)
	}
	for _, container := range baselineContainers {
		if container.Project == targetRuntimeProjectName(target) || container.Project == bhruntime.SharedProjectName(target.Name+"-core") {
			t.Fatalf("unexpected Core runtime container before Secrets lifecycle: %s (%s)", container.ID, container.Service)
		}
	}
	baselineIDs := runtimeContainerIDSet(baselineContainers)
	baselineSnapshot := runtimeContainerSnapshotSet(baselineContainers)
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
	if err := runWithIOAsTestOperator(ctx, []string{"app", "init", "--quick", "--json"}, &output, &output); err != nil {
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
		if err := destroySelectiveApplicationWithConsent(cleanup); err != nil {
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
		if containers, err := provider.ListRuntimeContainers(cleanup); err != nil {
			t.Errorf("inspect isolated Secrets cleanup: %v", err)
		} else if created, changed := runtimeContainerBaselineDiff(containers, baselineSnapshot); len(created) != 0 || len(changed) != 0 {
			t.Errorf("isolated Secrets cleanup did not preserve the runtime baseline: test-created=%v baseline-changed-or-removed=%v", created, changed)
		}
	})
	var originalSecret, originalEnv [32]byte
	var volumes []bhruntime.ProjectResource
	var sibling *sharedSecretsRuntimeApplication
	for phase, enabled := range []bool{true, false, true} {
		if shared && phase == 1 {
			sibling = createSharedSecretsRuntimeApplication(t, ctx, provider, root, initialized.Manifest.ApplicationID)
		}
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
		owner, err := coreinstallation.Load(root)
		if err != nil || owner.ID != initialOwner.ID || owner.Spec != initialOwner.Spec {
			t.Fatalf("Secrets capability changed its authoritative installation owner: before=%+v after=%+v err=%v", initialOwner, owner, err)
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
		_, changed := runtimeContainerBaselineDiff(containers, baselineSnapshot)
		if len(changed) != 0 {
			t.Fatalf("pre-existing runtime containers changed during Secrets phase %d: %v", phase, changed)
		}
		providerCount, brokerCount := 0, 0
		siblingBrokerCount, siblingWorkloadCount := 0, 0
		for _, container := range runtimeContainersNotInBaseline(containers, baselineIDs) {
			matched := false
			if sibling != nil {
				files := application.RuntimeFilesFor(sibling.resolved.Store, sibling.resolved.Manifest)
				project := runtimebroker.ProjectNameForRuntime(sibling.resolved.Manifest, files)
				if bhruntime.ContainerMatchesProjectResource(provider.Kind(), container, project, bhruntime.ProjectResource{Kind: "container", Name: project + "-broker-1"}) {
					siblingBrokerCount++
					matched = true
				}
				if bhruntime.ContainerMatchesProjectResource(provider.Kind(), container, files.Project, bhruntime.ProjectResource{Kind: "container", Name: files.Project + "-api-1"}) {
					siblingWorkloadCount++
					matched = true
				}
			}
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
		needed := enabled || (sibling != nil && phase == 1)
		if (needed && providerCount == 0) || (!needed && providerCount != 0) || (enabled && brokerCount != 1) || (!enabled && brokerCount != 0) {
			t.Fatalf("Secrets phase=%d need=%t actual provider=%d broker=%d", phase, enabled, providerCount, brokerCount)
		}
		if sibling != nil && (siblingWorkloadCount != 1 || (phase == 1 && siblingBrokerCount != 1) || (phase == 2 && siblingBrokerCount != 0)) {
			t.Fatal("shared Secrets change did not preserve the sibling workload and requested broker lifecycle")
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
				resourceNeeded := needed
				if resource.Name == runtimebroker.ProjectNameForRuntime(current.Manifest, appFiles)+"-broker-1" {
					resourceNeeded = enabled
				}
				if (resourceNeeded && err != nil) || (!resourceNeeded && !os.IsNotExist(err)) {
					t.Fatalf("Secrets phase=%d actual boot definition differs from selected need: %v", phase, err)
				}
			}
		}
		if sibling != nil && phase == 1 {
			sibling.verifySecret(t, ctx, provider, root)
			sibling.removeLastConsumer(t, ctx, provider, root, volumes, originalEnv)
		}
		t.Logf("runtime=%s Secrets phase=%d need=%t actual provider=%d broker=%d total=%d retained owned data volumes=%d; stable system ID and verified deployment", provider.Kind(), phase, enabled, providerCount, brokerCount, len(containers), len(volumes))
	}
	current, err := resolveApplication(ctx, application.Store{}, nil, "status")
	if err != nil {
		t.Fatal(err)
	}
	qualifySecretsDefaultRuntimeRetirement(t, ctx, provider, root, current, originalSecret, originalEnv, volumes)
	t.Log("generated app secret actually read before removal and after re-add: protected retained value unchanged, no plaintext output")
}
