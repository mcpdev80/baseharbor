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
				if err := runWithIOAsTestOperator(machineNoninteractiveContext(context.Background()), []string{"app", "init", "--quick", "--json"}, &output, &output); err != nil {
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
	target := configureTestTargetEmpty(t)
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
	baselineContainers, err := provider.ListRuntimeContainers(ctx)
	if err != nil {
		t.Fatal("inventory Coreless application runtime baseline", err)
	}
	baselineIDs := runtimeContainerIDSet(baselineContainers)
	baselineSnapshot := runtimeContainerSnapshotSet(baselineContainers)
	t.Chdir(t.TempDir())
	if err := os.WriteFile("compose.yaml", []byte(corelessHTTPWorkload), 0600); err != nil {
		t.Fatal(err)
	}
	writeApplicationBackendIntent(t, service, true)
	// Fresh app adoption allocates its canonical ID during initialization, so
	// there is no application UID yet to bind the local runtime grant to. Use
	// the explicit test operator only for this bootstrap boundary; subsequent
	// lifecycle operations retain the Coreless local-runtime grant path.
	if err := runWithIOAsTestOperator(ctx, []string{"app", "init", "--quick", "--json"}, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatalf("initialize application backend: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(testOperatorContext(machineNoninteractiveContext(context.Background())), 2*time.Minute)
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
		if containers, err := provider.ListRuntimeContainers(cleanupCtx); err != nil {
			t.Errorf("inspect Coreless application runtime cleanup: %v", err)
		} else if created, changed := runtimeContainerBaselineDiff(containers, baselineSnapshot); len(created) != 0 || len(changed) != 0 {
			t.Errorf("application backend cleanup did not preserve the Coreless runtime baseline: test-created=%v baseline-changed-or-removed=%v", created, changed)
		}
	})
	var selected []bhruntime.ProjectResource
	var originalIdentity [32]byte
	var appID string
	for phase, enabled := range []bool{true, false, true} {
		writeApplicationBackendIntent(t, service, enabled)
		applySelectiveBackendIntentWithLocalGrant(t, ctx)
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
		_, changed := runtimeContainerBaselineDiff(live, baselineSnapshot)
		if len(changed) != 0 {
			t.Fatalf("pre-existing runtime containers changed during application backend phase %d: %v", phase, changed)
		}
		for _, container := range runtimeContainersNotInBaseline(live, baselineIDs) {
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
			if phase == 0 && os.Getenv("BASEHARBOR_APP_STOP_ACCEPTANCE") == "1" {
				if err := stopSelectiveApplicationWithConsent(ctx, &bytes.Buffer{}); err != nil {
					t.Fatalf("acknowledged application backend stop: %v", err)
				}
				stopped, err := provider.InspectProjectResources(ctx, files.Project, selected)
				if err != nil {
					t.Fatal(err)
				}
				for _, resource := range stopped {
					if resource.Kind == "container" || resource.Kind == "network" {
						t.Fatal("application stop leaves selected runtime")
					}
				}
				for _, resource := range actual {
					if resource.Kind == "volume" && !application.ResourceNamedExists(stopped, resource) {
						t.Fatal("application stop lost observed owned persistent volume")
					}
				}
				applySelectiveBackendIntentWithLocalGrant(t, ctx)
				qualifyApplicationBackendRetainedData(t, ctx, files, service, retainedDataIntermediatePhase, &originalIdentity)
				t.Logf("application backend=%s acknowledged stop/resume: selected runtime withdrawn, observed data volumes preserved, original authenticated data and credentials work after resume", service)
			}
		}
		t.Logf("application backend=%s phase=%d needed=%t selected containers=%d retained volumes=%d", service, phase, enabled, containers, volumes)
	}
}

func applySelectiveBackendIntentWithLocalGrant(t *testing.T, ctx context.Context) {
	t.Helper()
	resolved, plan := waitForCompleteApplicationBackendPlan(t, ctx)
	applyCtx, candidateDigest := withConfirmCandidateDigestForResolved(ctx, resolved)
	if resolved.FromRepository && candidateDigest == "" {
		t.Fatal("repository candidate has no verifiable confirmation digest")
	}
	if impact := plan.ProviderPlan.Impact; impact != nil {
		applyCtx = withProviderChangeConsent(applyCtx, &application.ProviderChangeConsent{PlanID: impact.PlanID, First: true, Second: true})
	}
	var output bytes.Buffer
	if err := runWithIO(applyCtx, []string{"app", "apply", "--skip-memory-preflight"}, &output, &output); err != nil {
		t.Logf("Coreless local grant failed application backend plan: complete=%t changes=%+v", plan.ProviderPlan.Complete, plan.ProviderPlan.Changes)
		t.Fatalf("apply application backend with local runtime grant: %v\n%s", err, output.String())
	}
}

// A just-resumed native backend can briefly report a transitional health
// state even after its authenticated data probe succeeds. Do not mint typed
// provider consent from that transient, incomplete snapshot; wait for the
// canonical owner observation to settle, then bind consent to that plan.
func waitForCompleteApplicationBackendPlan(t *testing.T, ctx context.Context) (resolvedApplication, applicationLifecyclePlan) {
	t.Helper()
	deadline := time.Now().Add(45 * time.Second)
	for {
		resolved, err := resolveApplication(ctx, application.Store{}, nil, "apply")
		if err != nil {
			t.Fatal(err)
		}
		plan, err := buildResolvedApplicationPlan(ctx, resolved)
		if err != nil {
			t.Fatal(err)
		}
		if plan.ProviderPlan.Complete {
			return resolved, plan
		}
		if !time.Now().Before(deadline) {
			t.Fatalf("application backend provider plan did not reach a complete owner observation within 45s: changes=%+v", plan.ProviderPlan.Changes)
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(time.Second):
		}
	}
}
