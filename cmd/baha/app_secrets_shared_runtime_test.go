package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/applicationsecret"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/runtimebroker"
)

type sharedSecretsRuntimeApplication struct {
	root     string
	resolved resolvedApplication
	secret   [32]byte
}

func writeSharedSecretsRuntimeIntent(t *testing.T, root string, enabled bool) {
	t.Helper()
	content := fmt.Sprintf("version: 1\napp:\n  name: shared-secrets-sibling\n  environment: dev\nworkload:\n  components: [api]\nservices:\n  secrets: %t\n", enabled)
	if enabled {
		content += "secrets:\n  required:\n    - name: API_TOKEN\n      generate:\n        type: hex\n        bytes: 32\n"
	}
	if err := os.WriteFile(filepath.Join(root, "baseharbor.yaml"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func createSharedSecretsRuntimeApplication(t *testing.T, ctx context.Context, provider bhruntime.RuntimeProvider, providerRoot, firstID string) *sharedSecretsRuntimeApplication {
	t.Helper()
	sibling := &sharedSecretsRuntimeApplication{root: t.TempDir()}
	if err := os.WriteFile(filepath.Join(sibling.root, "compose.yaml"), []byte(corelessHTTPWorkload), 0600); err != nil {
		t.Fatal(err)
	}
	writeSharedSecretsRuntimeIntent(t, sibling.root, true)
	inSelectiveIdentityRepository(t, sibling.root, func() {
		var output bytes.Buffer
		if err := runWithIO(ctx, []string{"app", "init", "--quick", "--json"}, &output, &output); err != nil {
			t.Fatalf("initialize shared Secrets sibling: %v", err)
		}
		applySelectiveIdentityIntent(t, ctx)
		sibling.refresh(t, ctx)
	})
	if sibling.resolved.Manifest.ApplicationID == firstID {
		t.Fatal("two authored applications share the same system identity")
	}
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(machineNoninteractiveContext(context.Background()), 3*time.Minute)
		defer cancel()
		inSelectiveIdentityRepository(t, sibling.root, func() {
			files := application.RuntimeFilesFor(sibling.resolved.Store, sibling.resolved.Manifest)
			if _, err := runtimebroker.RetireContainer(cleanup, provider, sibling.resolved.Manifest, files); err != nil && !os.IsNotExist(err) {
				t.Errorf("isolated sibling broker cleanup: %v", err)
			}
			if err := runWithIO(cleanup, []string{"app", "destroy", "--yes"}, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
				t.Errorf("isolated sibling workload cleanup: %v", err)
			}
		})
	})
	sibling.secret = sibling.readSecret(t, ctx, provider, providerRoot)
	return sibling
}

func (s *sharedSecretsRuntimeApplication) refresh(t *testing.T, ctx context.Context) {
	t.Helper()
	current, err := resolveApplication(ctx, application.Store{}, nil, "status")
	if err != nil {
		t.Fatal(err)
	}
	status, _, err := collectResolvedApplicationStatus(ctx, current)
	if err != nil || !status.Ready || current.DeploymentRecord == nil || !current.DeploymentRecord.Observed.Ready || current.DeploymentRecord.Applied.GeneratedState["previous_intent"] != "" {
		t.Fatal("shared Secrets sibling has no verified converged deployment")
	}
	if s.resolved.Manifest.ApplicationID != "" && s.resolved.Manifest.ApplicationID != current.Manifest.ApplicationID {
		t.Fatal("shared Secrets sibling identity changed")
	}
	s.resolved = current
}

func (s *sharedSecretsRuntimeApplication) readSecret(t *testing.T, ctx context.Context, provider bhruntime.RuntimeProvider, root string) [32]byte {
	t.Helper()
	inventory, err := bhruntime.ExistingControlPlaneRuntimeInventory(root, bhruntime.SharedProjectName(s.resolved.Target.Name))
	if err != nil {
		t.Fatal(err)
	}
	files := application.RuntimeFilesFor(s.resolved.Store, s.resolved.Manifest)
	service := applicationsecret.NewForApplicationRuntime(s.resolved.Store, provider, inventory.Files, s.resolved.Manifest, files)
	value, err := service.Get(ctx, s.resolved.Manifest.Name, "API_TOKEN")
	if err != nil || len(value) == 0 {
		t.Fatal("shared Secrets sibling secret is not actually readable through its own protected scope")
	}
	digest := sha256.Sum256(value)
	clear(value)
	return digest
}

func (s *sharedSecretsRuntimeApplication) verifySecret(t *testing.T, ctx context.Context, provider bhruntime.RuntimeProvider, root string) {
	t.Helper()
	inSelectiveIdentityRepository(t, s.root, func() { s.refresh(t, ctx) })
	if s.readSecret(t, ctx, provider, root) != s.secret {
		t.Fatal("removing the other application changed the sibling secret")
	}
	t.Log("two apps: first Secrets removal preserves actual sibling readiness, owned broker and readable unchanged secret")
}

func (s *sharedSecretsRuntimeApplication) removeLastConsumer(t *testing.T, ctx context.Context, provider bhruntime.RuntimeProvider, root string, volumes []bhruntime.ProjectResource, originalEnv [32]byte) {
	t.Helper()
	writeSharedSecretsRuntimeIntent(t, s.root, false)
	inSelectiveIdentityRepository(t, s.root, func() {
		applySelectiveIdentityIntent(t, ctx)
		s.refresh(t, ctx)
	})
	inventory, err := bhruntime.ExistingControlPlaneRuntimeInventory(root, bhruntime.SharedProjectName(s.resolved.Target.Name))
	if err != nil {
		t.Fatal(err)
	}
	actual, err := provider.InspectProjectResources(ctx, inventory.Files.Project, inventory.Resources)
	if err != nil {
		t.Fatal(err)
	}
	for _, resource := range actual {
		if resource.Kind == "container" {
			t.Fatal("last Secrets consumer removal left unneeded provider runtime")
		}
	}
	for _, volume := range volumes {
		if !application.ResourceNamedExists(actual, volume) {
			t.Fatal("last Secrets consumer removal deleted retained data")
		}
	}
	data, err := os.ReadFile(inventory.Files.Env)
	if err != nil || sha256.Sum256(data) != originalEnv {
		t.Fatal("last Secrets consumer removal changed retained recovery/configuration material")
	}
	containers, err := provider.ListRuntimeContainers(ctx)
	if err != nil || len(containers) != 2 {
		t.Fatal("last Secrets removal must leave exactly the two requested workload containers")
	}
	t.Logf("two apps: last Secrets removal converged total runtime to two workloads; retained owned data volumes=%d", len(volumes))
}
