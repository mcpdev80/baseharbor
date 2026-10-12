package kubernetes_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/artifact"
	kubernetes "github.com/mcpdev80/baseharbor/internal/providers/runtime/kubernetes"
	runtimemodel "github.com/mcpdev80/baseharbor/internal/runtime/model"
)

func TestKubernetesReferenceDemoArtifactLifecycleOnCI(t *testing.T) {
	if os.Getenv("BASEHARBOR_KUBERNETES_DEMO_INTEGRATION") != "1" {
		t.Skip("set BASEHARBOR_KUBERNETES_DEMO_INTEGRATION=1 for real reference-demo acceptance")
	}

	root := strings.TrimSpace(os.Getenv("BASEHARBOR_DEMO_ROOT"))
	image := strings.TrimSpace(os.Getenv("BASEHARBOR_DEMO_IMAGE"))
	if root == "" || image == "" {
		t.Fatal("BASEHARBOR_DEMO_ROOT and BASEHARBOR_DEMO_IMAGE are required")
	}
	if _, err := os.Stat(filepath.Join(root, "compose.yaml")); err != nil {
		t.Fatalf("reference demo Compose source unavailable: %v", err)
	}

	manifest := application.Manifest{
		Version:     application.CurrentVersion,
		Name:        "baseharbor-demo",
		Environment: "dev",
		Services: application.Services{
			Postgres:      true,
			Redis:         true,
			ObjectStorage: true,
			Secrets:       true,
		},
		Workload: application.WorkloadConfig{
			Compose:  "compose.yaml",
			Services: []string{"demo-app"},
		},
	}

	model, found, err := application.ResolveRepositoryWorkloadModel(root, manifest)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("reference demo workload not found")
	}

	resolution, err := artifact.ResolveWorkload(root, model)
	if err != nil {
		t.Fatal(err)
	}
	if resolution.Complete() || len(resolution.Builds) != 1 {
		t.Fatalf("expected one source-backed demo artifact request, got %#v", resolution)
	}
	if resolution.Builds[0].Service != "demo-app" {
		t.Fatalf("build request = %#v", resolution.Builds[0])
	}

	provider, err := kubernetes.Detect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	namespace := provider.Namespace()
	if namespace == "" {
		t.Fatal("Kubernetes target namespace is empty")
	}

	plan := runtimemodel.WorkloadPlan{
		Application: manifest.Name,
		Environment: manifest.Environment,
		Target:      runtimemodel.Target{Scope: namespace},
		Workload:    model,
		Images: map[string]string{
			"demo-app": image,
		},
		Bindings: map[string]runtimemodel.Binding{
			"APP_SECRET": {
				Value:     "kubernetes-demo-ci",
				Sensitive: true,
			},
			"DATABASE_URL": {
				Value:     "postgresql://demo:demo@unavailable.invalid:5432/demo",
				Sensitive: true,
			},
			"REDIS_URL": {
				Value:     "redis://unavailable.invalid:6379/0",
				Sensitive: true,
			},
			"S3_ENDPOINT": {
				Value: "http://unavailable.invalid:9000",
			},
			"S3_BUCKET": {
				Value: "uploads",
			},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	_ = provider.Destroy(context.Background(), manifest.Name, manifest.Environment, namespace)
	t.Cleanup(func() {
		_ = provider.Destroy(context.Background(), manifest.Name, manifest.Environment, namespace)
	})

	if err := provider.Apply(ctx, plan); err != nil {
		t.Fatal(err)
	}
	if err := provider.WaitReady(ctx, plan, 2*time.Minute); err != nil {
		t.Fatal(err)
	}

	observation, err := provider.Observe(ctx, manifest.Name, manifest.Environment, namespace)
	if err != nil {
		t.Fatal(err)
	}
	if !observation.Ready() {
		t.Fatalf("reference demo is not ready: %#v", observation)
	}
	if len(observation.Services) != 1 || observation.Services[0].Service != "demo-app" {
		t.Fatalf("reference demo observation lost logical workload identity: %#v", observation)
	}

	health, err := provider.Exec(
		ctx,
		manifest.Name,
		manifest.Environment,
		namespace,
		"demo-app",
		"wget", "-qO-", "http://127.0.0.1:8080/healthz",
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(health, `"status":"ok"`) ||
		!strings.Contains(health, `"service":"baseharbor-demo"`) {
		t.Fatalf("unexpected reference demo health response: %s", health)
	}

	if err := provider.Destroy(ctx, manifest.Name, manifest.Environment, namespace); err != nil {
		t.Fatal(err)
	}
	after, err := provider.Observe(ctx, manifest.Name, manifest.Environment, namespace)
	if err != nil {
		t.Fatal(err)
	}
	if after.Found {
		t.Fatalf("reference demo remains after destroy: %#v", after)
	}
}
