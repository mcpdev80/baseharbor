package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/exposure"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func TestManagedHTTPExposureLifecycleInCI(t *testing.T) {
	if os.Getenv("CI") == "" {
		t.Skip("real managed exposure lifecycle runs in CI")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	compose, err := bhruntime.DetectCompose(ctx)
	if err != nil {
		t.Fatalf("detect compose: %v", err)
	}

	resolved, files, workload := managedExposureFixture(t, "exposure-ci", 8080, 8080)
	defer cleanupManagedExposureFixture(compose, resolved, files, workload)

	if err := startManagedExposureFixtureWorkload(ctx, compose, workload); err != nil {
		t.Fatalf("start fixture workload: %v", err)
	}
	prepared, err := prepareManagedExposure(ctx, compose, resolved)
	if err != nil {
		t.Fatalf("prepare managed exposure: %v", err)
	}
	if err := convergeManagedExposure(ctx, io.Discard, prepared); err != nil {
		t.Fatalf("converge managed exposure: %v", err)
	}

	firstState, providerFiles, err := exposure.Load(files)
	if err != nil {
		t.Fatal(err)
	}
	if len(firstState.Routes) != 1 || firstState.Routes[0].Visibility != "internal" {
		t.Fatalf("unexpected managed routes: %#v", firstState.Routes)
	}
	lines, err := inspectManagedExposure(ctx, compose, resolved.Manifest, files)
	if err != nil {
		t.Fatalf("inspect managed exposure: %v", err)
	}
	if len(lines) != 1 || !strings.Contains(lines[0], "HTTP 200") {
		t.Fatalf("managed exposure readiness = %#v", lines)
	}

	prepared, err = prepareManagedExposure(ctx, compose, resolved)
	if err != nil {
		t.Fatal(err)
	}
	if err := convergeManagedExposure(ctx, io.Discard, prepared); err != nil {
		t.Fatalf("repeat convergence: %v", err)
	}
	secondState, _, err := exposure.Load(files)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(firstState, secondState) {
		t.Fatalf("repeated convergence changed provider state: first=%#v second=%#v", firstState, secondState)
	}

	if err := stopManagedExposure(ctx, compose, resolved.Manifest, files); err != nil {
		t.Fatalf("stop managed exposure: %v", err)
	}
	running, err := compose.RunningServicesProject(ctx, secondState.Project, providerFiles.Compose, providerFiles.Env)
	if err != nil {
		t.Fatal(err)
	}
	if len(running) != 0 {
		t.Fatalf("managed exposure remains running after stop: %#v", running)
	}

	prepared, err = prepareManagedExposure(ctx, compose, resolved)
	if err != nil {
		t.Fatal(err)
	}
	if err := convergeManagedExposure(ctx, io.Discard, prepared); err != nil {
		t.Fatalf("restart managed exposure: %v", err)
	}
	thirdState, _, err := exposure.Load(files)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(secondState, thirdState) {
		t.Fatalf("stop/start changed provider state: before=%#v after=%#v", secondState, thirdState)
	}

	if err := destroyManagedExposure(ctx, compose, resolved.Manifest, files); err != nil {
		t.Fatalf("destroy managed exposure: %v", err)
	}
	if _, err := os.Stat(providerFiles.Dir); !os.IsNotExist(err) {
		t.Fatalf("managed exposure provider state remains after destroy: %v", err)
	}
}

func TestManagedHTTPExposureFailedVerificationCleansProviderResourcesInCI(t *testing.T) {
	if os.Getenv("CI") == "" {
		t.Skip("real managed exposure failure cleanup runs in CI")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	compose, err := bhruntime.DetectCompose(ctx)
	if err != nil {
		t.Fatalf("detect compose: %v", err)
	}

	resolved, files, workload := managedExposureFixture(t, "exposure-fail-ci", 9099, 8080)
	defer cleanupManagedExposureFixture(compose, resolved, files, workload)

	if err := startManagedExposureFixtureWorkload(ctx, compose, workload); err != nil {
		t.Fatalf("start fixture workload: %v", err)
	}
	prepared, err := prepareManagedExposure(ctx, compose, resolved)
	if err != nil {
		t.Fatalf("prepare broken managed exposure: %v", err)
	}
	err = convergeManagedExposure(ctx, io.Discard, prepared)
	if err == nil {
		t.Fatal("broken managed exposure unexpectedly converged")
	}
	if !strings.Contains(err.Error(), "not ready") {
		t.Fatalf("unexpected convergence failure: %v", err)
	}

	providerFiles := exposure.FilesFor(files)
	if _, statErr := os.Stat(providerFiles.Dir); !os.IsNotExist(statErr) {
		t.Fatalf("failed exposure left provider state behind: %v", statErr)
	}
	running, err := compose.RunningServicesProjectFilesEnv(ctx, workload.Project, workload.RepositoryRoot, nil, workload.Compose, workload.Override)
	if err != nil {
		t.Fatalf("inspect fixture workload after provider failure: %v", err)
	}
	if len(running) != 1 || running[0] != "web" {
		t.Fatalf("provider failure mutated application-owned workload: running=%#v", running)
	}
}

func managedExposureFixture(t *testing.T, name string, targetPort, actualPort int) (resolvedApplication, application.RuntimeFiles, application.WorkloadFiles) {
	t.Helper()
	root := t.TempDir()
	m := application.Manifest{
		Version:     application.CurrentVersion,
		Name:        name,
		Environment: "dev",
		Workload: application.WorkloadConfig{
			Compose:  "compose.yaml",
			Services: []string{"web"},
		},
		Exposures: []application.HTTPExposureRequirement{{
			Name: "public", Service: "web", Port: targetPort, Protocol: "http", Visibility: "internal",
		}},
	}
	manifestPath := filepath.Join(root, "baseharbor.yaml")
	if err := os.WriteFile(manifestPath, []byte(m.YAML()), 0o644); err != nil {
		t.Fatal(err)
	}
	composeYAML := fmt.Sprintf(`services:
  web:
    image: alpine:3.22
    command:
      - sh
      - -ec
      - |
        mkdir -p /www
        printf 'baseharbor-exposure-ok\\n' >/www/index.html
        exec busybox httpd -f -p %d -h /www
    networks:
      - app-internal
networks:
  app-internal:
`, actualPort)
	if err := os.WriteFile(filepath.Join(root, "compose.yaml"), []byte(composeYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeRepositoryInitState(root, repositoryInitState{
		Hostname: "localhost", TLSMode: "local", RuntimeProvider: bhruntime.ProviderCompose,
	}); err != nil {
		t.Fatal(err)
	}
	store := application.Store{Root: filepath.Join(root, ".baseharbor", "apps")}
	if _, err := store.Sync(m); err != nil {
		t.Fatal(err)
	}
	files, err := application.EnsureRuntime(store, m)
	if err != nil {
		t.Fatal(err)
	}
	workload, found, err := application.MaterializeWorkload(root, m, files)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("fixture workload was not materialized")
	}
	return resolvedApplication{
		Manifest: m, ManifestPath: manifestPath, Store: store, FromRepository: true,
	}, files, workload
}

func startManagedExposureFixtureWorkload(ctx context.Context, compose bhruntime.Compose, workload application.WorkloadFiles) error {
	files := []string{workload.Compose, workload.Override}
	if err := compose.ConfigProjectFilesEnv(ctx, workload.Project, workload.RepositoryRoot, nil, files...); err != nil {
		return err
	}
	if err := compose.UpProjectFilesSelected(ctx, workload.Project, workload.RepositoryRoot, nil, workload.Services, files...); err != nil {
		return err
	}
	deadline, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	var lastRunning []string
	var lastErr error
	for deadline.Err() == nil {
		lastRunning, lastErr = compose.RunningServicesProjectFilesEnv(deadline, workload.Project, workload.RepositoryRoot, nil, files...)
		if lastErr == nil && len(lastRunning) == 1 && lastRunning[0] == "web" {
			return nil
		}
		select {
		case <-deadline.Done():
		case <-time.After(250 * time.Millisecond):
		}
	}
	return fmt.Errorf("fixture workload did not become ready: running=%v last_error=%v deadline=%w", lastRunning, lastErr, deadline.Err())
}

func cleanupManagedExposureFixture(compose bhruntime.Compose, resolved resolvedApplication, files application.RuntimeFiles, workload application.WorkloadFiles) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_ = destroyManagedExposure(ctx, compose, resolved.Manifest, files)
	_ = compose.DownProjectFilesEnv(ctx, workload.Project, workload.RepositoryRoot, nil, workload.Compose, workload.Override)
}
