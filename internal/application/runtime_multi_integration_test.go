package application

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func TestMultiInstanceComposeLifecycleInCI(t *testing.T) {
	if os.Getenv("CI") == "" {
		t.Skip("real multi-instance runtime verification runs in CI")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	compose, err := bhruntime.DetectCompose(ctx)
	if err != nil {
		t.Fatalf("detect compose: %v", err)
	}

	store := Store{Root: filepath.Join(t.TempDir(), "apps")}
	m := New("multi-ci", "dev", false, false, false)
	m = WithPostgresInstances(m, "primary", "analytics")
	m = WithRedisInstances(m, "cache", "sessions")
	files, err := EnsureRuntime(store, m)
	if err != nil {
		t.Fatal(err)
	}
	project := RuntimeProjectName(m)
	if err := compose.ConfigProject(ctx, project, files.Compose, files.Env); err != nil {
		t.Fatalf("validate generated multi-instance compose: %v", err)
	}
	if err := compose.UpProject(ctx, project, files.Compose, files.Env); err != nil {
		t.Fatalf("start multi-instance runtime: %v", err)
	}
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		if err := compose.DestroyProject(cleanupCtx, project, files.Compose, files.Env); err != nil {
			t.Errorf("destroy multi-instance runtime: %v", err)
		}
	}()

	network := bhruntime.ProjectResource{Kind: "network", Name: ApplicationBackendNetworkName(m)}
	exists, err := compose.InspectProjectResource(ctx, project, network)
	if err != nil {
		t.Fatalf("inspect application backend network: %v", err)
	}
	if !exists {
		t.Fatalf("application backend network %s was not created", network.Name)
	}

	deadline := time.Now().Add(45 * time.Second)
	for {
		err = VerifyPostgresRuntime(ctx, compose, m, files)
		if err == nil {
			err = VerifyValkeyRuntime(ctx, compose, m, files)
		}
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("multi-instance runtime never became ready: %v", err)
		}
		select {
		case <-ctx.Done():
			t.Fatalf("multi-instance runtime verification timed out: %v", ctx.Err())
		case <-time.After(time.Second):
		}
	}

	running, err := compose.RunningServicesProject(ctx, project, files.Compose, files.Env)
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(running)
	want := []string{"postgres-analytics", "postgres-primary", "valkey-cache", "valkey-sessions"}
	if len(running) != len(want) {
		t.Fatalf("unexpected running services: got %#v want %#v", running, want)
	}
	for i := range want {
		if running[i] != want[i] {
			t.Fatalf("unexpected running services: got %#v want %#v", running, want)
		}
	}
}
