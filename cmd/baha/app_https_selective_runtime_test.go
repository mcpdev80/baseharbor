package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/devgateway"
	"github.com/mcpdev80/baseharbor/internal/exposure"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func TestSelectiveHTTPSRuntimeAddRemoveReAdd(t *testing.T) {
	if os.Getenv("BASEHARBOR_HTTPS_LIFECYCLE_ACCEPTANCE") != "1" {
		t.Skip("requires isolated exact-SHA Docker/rootless Podman HTTPS acceptance")
	}
	ctx, cancel := context.WithTimeout(machineNoninteractiveContext(context.Background()), 8*time.Minute)
	defer cancel()
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	target := configureTestTarget(t)
	if os.Getenv("BASEHARBOR_TEST_RUNTIME") == "podman" {
		cfg, err := deployment.LoadConfig()
		if err != nil {
			t.Fatal(err)
		}
		definition := cfg.Targets[target.Name]
		definition.Runtime.Provider = "podman"
		cfg.Targets[target.Name] = definition
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
		content := "version: 1\napp:\n  name: selective-https\n  environment: dev\nworkload:\n  components:\n    - api\n"
		if enabled {
			content += "exposure:\n  http:\n    - name: public\n      service: api\n      port: 8080\n      protocol: https\n      visibility: public\n"
		}
		if err := os.WriteFile("baseharbor.yaml", []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	writeIntent(true)
	var output bytes.Buffer
	if err := runWithIO(ctx, []string{"app", "init", "--input", "tls_mode=local", "--yes"}, &output, &output); err != nil {
		t.Fatalf("initialize HTTPS-only workload: %v\n%s", err, output.String())
	}
	initialized, err := resolveApplication(ctx, application.Store{}, nil, "plan")
	if err != nil {
		t.Fatal(err)
	}
	files := application.RuntimeFilesFor(initialized.Store, initialized.Manifest)
	var originalCA [32]byte
	gateway, err := devgateway.FilesFor(target.Name)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(machineNoninteractiveContext(context.Background()), 2*time.Minute)
		defer stop()
		if err := removeApplicationGatewayOwners(cleanup, provider, initialized, "app/"+initialized.Manifest.Name+"/"+initialized.Manifest.Environment); err != nil {
			t.Errorf("isolated HTTPS gateway cleanup: %v", err)
		}
		if _, err := exposure.RetireRuntime(cleanup, provider, initialized.Manifest, files); err != nil && !os.IsNotExist(err) {
			t.Errorf("isolated HTTPS provider cleanup: %v", err)
		}
		if err := runWithIO(cleanup, []string{"app", "destroy", "--yes"}, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
			t.Errorf("isolated HTTPS workload cleanup: %v", err)
		}
		if containers, err := provider.ListRuntimeContainers(cleanup); err != nil || len(containers) != 0 {
			t.Errorf("isolated HTTPS cleanup leaves containers=%d inspection=%v", len(containers), err)
		}
	})
	for phase, enabled := range []bool{true, false, true} {
		writeIntent(enabled)
		applySelectiveIdentityIntent(t, ctx)
		current, err := resolveApplication(ctx, application.Store{}, nil, "status")
		if err != nil {
			t.Fatal(err)
		}
		status, _, err := collectResolvedApplicationStatus(ctx, current)
		if err != nil || !status.Ready || current.Manifest.ApplicationID != initialized.Manifest.ApplicationID {
			t.Fatalf("HTTPS phase=%d readiness/identity: %+v %v", phase, status, err)
		}
		if current.DeploymentRecord == nil || !current.DeploymentRecord.Observed.Ready || current.DeploymentRecord.Applied.GeneratedState["previous_intent"] != "" {
			t.Fatal("HTTPS apply lacks verified converged deployment record")
		}
		inventory, err := exposure.ExistingRuntimeInventory(current.Manifest, files)
		if err != nil {
			t.Fatal(err)
		}
		containers, err := provider.ListRuntimeContainers(ctx)
		if err != nil {
			t.Fatal(err)
		}
		selected := 0
		for _, container := range containers {
			matched := false
			for _, resource := range inventory.Resources {
				if bhruntime.ContainerMatchesProjectResource(provider.Kind(), container, inventory.State.Project, resource) {
					matched = true
					selected++
				}
			}
			workload := bhruntime.ProjectResource{Kind: "container", Name: files.Project + "-api-1"}
			gatewayResource := bhruntime.ProjectResource{Kind: "container", Name: gateway.Project + "-dev-gateway-1"}
			if !matched && !bhruntime.ContainerMatchesProjectResource(provider.Kind(), container, files.Project, workload) && !(enabled && bhruntime.ContainerMatchesProjectResource(provider.Kind(), container, gateway.Project, gatewayResource)) {
				t.Fatalf("HTTPS-only materialized unrequested project=%s service=%s", container.Project, container.Service)
			}
		}
		if (enabled && selected == 0) || (!enabled && selected != 0) {
			t.Fatalf("HTTPS phase=%d need=%t actual selected=%d", phase, enabled, selected)
		}
		if enabled {
			if err := devgateway.Verify(ctx, target.Name); err != nil {
				t.Fatalf("actual verified browser TLS/workload route: %v", err)
			}
			data, err := os.ReadFile(gateway.CA)
			if err != nil {
				t.Fatal(err)
			}
			if phase == 0 {
				originalCA = sha256.Sum256(data)
			} else if originalCA != sha256.Sum256(data) {
				t.Fatal("HTTPS re-add replaced retained public CA")
			}
		}
		if provider.Kind() == bhruntime.ProviderPodman {
			home, err := os.UserHomeDir()
			if err != nil {
				t.Fatal(err)
			}
			for _, route := range inventory.State.Routes {
				_, err := os.Lstat(filepath.Join(home, ".config", "containers", "systemd", inventory.State.Project+"-baseharbor-internal-exposure-"+route.Name+".container"))
				if (enabled && err != nil) || (!enabled && !os.IsNotExist(err)) {
					t.Fatalf("HTTPS phase=%d actual boot definition differs from selected need: %v", phase, err)
				}
			}
		}
		if data, err := os.ReadFile("baseharbor.yaml"); err != nil || strings.Contains(string(data), initialized.Manifest.ApplicationID) {
			t.Fatal("HTTPS lifecycle wrote manual ID into authored manifest")
		}
		assertCorelessHost(t, ctx, provider, target)
		t.Logf("runtime=%s HTTPS phase=%d need=%t actual selected=%d total=%d; public CA retained, verified deployment and system identity unchanged", provider.Kind(), phase, enabled, selected, len(containers))
	}
}
