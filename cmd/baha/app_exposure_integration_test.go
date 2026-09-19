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
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	compose, err := bhruntime.DetectCompose(ctx)
	if err != nil {
		t.Fatalf("detect compose: %v", err)
	}

	resolved, files := managedExposureFixture(t, "exposure-ci", 8080, 8080)
	defer func() {
		_ = destroyManagedExposure(context.Background(), compose, resolved.Manifest, files)
		_, _ = stopRepositoryWorkload(context.Background(), compose, resolved, files)
	}()

	prepared, err := prepareManagedExposure(ctx, compose, resolved)
	if err != nil {
		t.Fatalf("prepare managed exposure: %v", err)
	}
	if _, err := applyRepositoryWorkload(ctx, io.Discard, compose, resolved, files); err != nil {
		t.Fatalf("apply repository workload: %v", err)
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
		t.Fatalf("repeated convergence changed provider state:\nfirst=%#v\nsecond=%#v", firstState, secondState)
	}

	if err := stopManagedExposure(ctx, compose, resolved.Manifest, files); err != nil {
		t.Fatalf("stop managed exposure: %v", err)
	}
	if _, err := stopRepositoryWorkload(ctx, compose, resolved, files); err != nil {
		t.Fatalf("stop repository workload: %v", err)
	}
	running, err := compose.RunningServicesProject(ctx, secondState.Project, providerFiles.Compose, providerFiles.Env)
	if err != nil {
		t.Fatal(err)
	}
	if len(running) != 0 {
		t.Fatalf("managed exposure remains running after stop: %#v", running)
	}

	if _, err := applyRepositoryWorkload(ctx, io.Discard, compose, resolved, files); err != nil {
		t.Fatalf("restart repository workload: %v", err)
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
		t.Fatalf("stop/start changed provider state:\nbefore=%#v\nafter=%#v", secondState, thirdState)
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
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	compose, err := bhruntime.DetectCompose(ctx)
	if err != nil {
		t.Fatalf("detect compose: %v", err)
	}

	resolved, files := managedExposureFixture(t, "exposure-fail-ci", 9099, 8080)
	defer func() {
		_ = destroyManagedExposure(context.Background(), compose, resolved.Manifest, files)
		_, _ = stopRepositoryWorkload(context.Background(), compose, resolved, files)
	}()

	prepared, err := prepareManagedExposure(ctx, compose, resolved)
	if err != nil {
		t.Fatalf("prepare broken managed exposure: %v", err)
	}
	if _, err := applyRepositoryWorkload(ctx, io.Discard, compose, resolved, files); err != nil {
		t.Fatalf("apply repository workload: %v", err)
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
	_, running, found, inspectErr := inspectRepositoryWorkload(ctx, compose, resolved, files)
	if inspectErr != nil {
		t.Fatalf("inspect repository workload after provider failure: %v", inspectErr)
	}
	if !found || len(running) == 0 {
		t.Fatalf("provider failure mutated application-owned workload: found=%v running=%#v", found, running)
	}
}

func managedExposureFixture(t *testing.T, name string, targetPort, actualPort int) (resolvedApplication, application.RuntimeFiles) {
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
	composeYAML := "services:\n  web:\n    image: alpine:3.22\n    command: [\"sh\", \"-ec\", \"exec busybox httpd -f -p " +
		fmt.Sprint(actualPort) + "\"]\n    networks:\n      - app-internal\nnetworks:\n  app-internal:\n"
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
	return resolvedApplication{
		Manifest: m, ManifestPath: manifestPath, Store: store, FromRepository: true,
	}, files
}
