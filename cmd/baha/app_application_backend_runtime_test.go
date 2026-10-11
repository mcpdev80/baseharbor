package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func TestSelectiveApplicationBackendRuntimeAddRemoveReAdd(t *testing.T) {
	if os.Getenv("BASEHARBOR_BACKEND_LIFECYCLE_ACCEPTANCE") != "1" {
		t.Skip("requires isolated exact-SHA Docker/rootless Podman backend acceptance")
	}
	for _, service := range []string{"messaging", "document_database"} {
		if !t.Run(service, func(t *testing.T) { qualifyApplicationBackendLifecycle(t, service) }) {
			break
		}
	}
}

func writeApplicationBackendIntent(t *testing.T, service string, enabled bool) {
	t.Helper()
	services := fmt.Sprintf("  document_database: %t\n", enabled)
	if service == "messaging" {
		services = fmt.Sprintf("  messaging_queue: %t\n  messaging_pubsub: %t\n  messaging_stream: %t\n", enabled, enabled, enabled)
	}
	content := "version: 1\napp:\n  name: selective-foundation\n  environment: dev\nservices:\n" + services
	if err := os.WriteFile("baseharbor.yaml", []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestSelectiveApplicationBackendIntentInitializesFromActualCLI(t *testing.T) {
	for _, service := range []string{"messaging", "document_database"} {
		t.Run(service, func(t *testing.T) {
			t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
			t.Setenv("BASEHARBOR_TEST_RUNTIME", "static")
			configureTestTarget(t)
			t.Chdir(t.TempDir())
			if err := os.WriteFile("compose.yaml", []byte(corelessHTTPWorkload), 0600); err != nil {
				t.Fatal(err)
			}
			for _, enabled := range []bool{true, false, true} {
				writeApplicationBackendIntent(t, service, enabled)
				var output bytes.Buffer
				if err := runWithIO(machineNoninteractiveContext(context.Background()), []string{"app", "init", "--quick", "--json"}, &output, &output); err != nil {
					t.Fatalf("actual CLI initialization for backend=%s enabled=%t: %v\n%s", service, enabled, err, output.String())
				}
			}
		})
	}
}

func qualifyApplicationBackendLifecycle(t *testing.T, service string) {
	ctx, cancel := context.WithTimeout(machineNoninteractiveContext(context.Background()), 10*time.Minute)
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
	writeApplicationBackendIntent(t, service, true)
	if err := runWithIO(ctx, []string{"app", "init", "--quick", "--json"}, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatalf("initialize application backend: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(machineNoninteractiveContext(context.Background()), 2*time.Minute)
		defer cleanupCancel()
		current, err := resolveApplication(cleanupCtx, application.Store{}, nil, "status")
		if err != nil {
			t.Errorf("application backend cleanup owner: %v", err)
			return
		}
		files := application.RuntimeFilesFor(current.Store, current.Manifest)
		if err := destroySelectiveApplicationWithConsent(cleanupCtx); err != nil {
			t.Errorf("application backend cleanup: %v", err)
		}
		if _, err := os.Lstat(files.Compose); err == nil {
			if err := provider.DestroyProject(cleanupCtx, files.Project, files.Compose, files.Env); err != nil {
				t.Errorf("isolated application backend native cleanup: %v", err)
			}
		}
		if containers, err := provider.ListRuntimeContainers(cleanupCtx); err != nil || len(containers) != 0 {
			t.Errorf("application backend cleanup leaves containers=%d inspection=%v", len(containers), err)
		}
	})
	var selected []bhruntime.ProjectResource
	var originalIdentity [32]byte
	var appID string
	for phase, enabled := range []bool{true, false, true} {
		writeApplicationBackendIntent(t, service, enabled)
		applySelectiveIdentityIntent(t, ctx)
		current, err := resolveApplication(ctx, application.Store{}, nil, "status")
		if err != nil {
			t.Fatal(err)
		}
		status, _, err := collectResolvedApplicationStatus(ctx, current)
		if err != nil || !status.Ready || current.DeploymentRecord == nil || !current.DeploymentRecord.Observed.Ready {
			t.Fatalf("application backend phase=%d readiness: %+v %v", phase, status, err)
		}
		files := application.RuntimeFilesFor(current.Store, current.Manifest)
		if phase == 0 {
			appID = current.Manifest.ApplicationID
			selected = application.ExpectedRuntimeResourcesForIdentity(current.Manifest, files.Project, files.ResourceProject)
		} else if current.Manifest.ApplicationID != appID {
			t.Fatal("application backend change replaced canonical identity")
		}
		actual, err := provider.InspectProjectResources(ctx, files.Project, selected)
		if err != nil {
			t.Fatal(err)
		}
		containers, volumes := 0, 0
		for _, resource := range actual {
			if resource.Kind == "container" {
				containers++
			} else if resource.Kind == "volume" {
				volumes++
			}
		}
		if (enabled && containers == 0) || (!enabled && containers != 0) || volumes == 0 {
			t.Fatalf("application backend=%s phase=%d containers=%d retained volumes=%d", service, phase, containers, volumes)
		}
		live, err := provider.ListRuntimeContainers(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, container := range live {
			workload := bhruntime.ProjectResource{Kind: "container", Name: files.Project + "-api-1"}
			matched := bhruntime.ContainerMatchesProjectResource(provider.Kind(), container, files.Project, workload)
			for _, resource := range selected {
				matched = matched || (resource.Kind == "container" && bhruntime.ContainerMatchesProjectResource(provider.Kind(), container, files.Project, resource))
			}
			if !matched {
				t.Fatal("unrequested Core or provider container materialized")
			}
		}
		if provider.Kind() == bhruntime.ProviderPodman {
			home, err := os.UserHomeDir()
			if err != nil {
				t.Fatal(err)
			}
			for _, resource := range selected {
				if resource.Kind != "container" {
					continue
				}
				name := strings.TrimSuffix(resource.Name, "-1")
				_, err := os.Lstat(filepath.Join(home, ".config", "containers", "systemd", name+".container"))
				if (enabled && err != nil) || (!enabled && !os.IsNotExist(err)) {
					t.Fatalf("application backend phase=%d native boot definition mismatch: %v", phase, err)
				}
			}
		}
		if enabled {
			qualifyApplicationBackendRetainedData(t, ctx, files, service, phase, &originalIdentity)
		}
		t.Logf("application backend=%s phase=%d needed=%t selected containers=%d retained volumes=%d", service, phase, enabled, containers, volumes)
	}
}
