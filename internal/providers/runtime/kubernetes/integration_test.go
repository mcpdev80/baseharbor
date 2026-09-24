package kubernetes

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/workload"
)

func TestKubernetesRenderedWorkloadLifecycleOnCI(t *testing.T) {
	if os.Getenv("BASEHARBOR_KUBERNETES_INTEGRATION") != "1" {
		t.Skip("set BASEHARBOR_KUBERNETES_INTEGRATION=1 for real Kubernetes acceptance")
	}
	if _, err := exec.LookPath("kubectl"); err != nil {
		t.Skipf("kubectl unavailable: %v", err)
	}

	namespace := os.Getenv("BASEHARBOR_K8S_NAMESPACE")
	if namespace == "" {
		namespace = "baseharbor-ci"
	}
	const app = "baha-render-acceptance"

	provider, err := Detect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if provider.Context() == "" {
		t.Fatal("detected Kubernetes provider has no context")
	}

	plan := Plan{
		Application: app,
		Environment: "dev",
		Namespace:   namespace,
		Workload: workload.Model{Services: []workload.Service{{
			Name:        "app",
			Image:       "registry.k8s.io/pause:3.10",
			Environment: map[string]string{"PORT": "8080"},
			Ports:       []workload.Port{{Container: 8080, Protocol: "tcp"}},
		}}},
		Bindings: map[string]Binding{
			"APP_SECRET": {Value: "ci-only", Sensitive: true},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	_ = provider.Destroy(context.Background(), app, "dev", namespace)
	t.Cleanup(func() {
		_ = provider.Destroy(context.Background(), app, "dev", namespace)
	})

	if err := provider.Apply(ctx, plan); err != nil {
		t.Fatal(err)
	}
	if err := provider.WaitReady(ctx, plan, 90*time.Second); err != nil {
		t.Fatal(err)
	}

	observation, err := provider.Observe(ctx, app, "dev", namespace)
	if err != nil {
		t.Fatal(err)
	}
	if !observation.Found || !observation.Ready() {
		t.Fatalf("workload observation not ready: %#v", observation)
	}
	if len(observation.Services) != 1 || observation.Services[0].Service != "app" {
		t.Fatalf("unexpected workload observation: %#v", observation)
	}

	denied, err := exec.CommandContext(ctx, "kubectl", "auth", "can-i", "create", "namespaces").CombinedOutput()
	fields := strings.Fields(string(denied))
	if err == nil || len(fields) == 0 || fields[len(fields)-1] != "no" {
		t.Fatalf("expected namespace creation to remain denied, output=%q err=%v", denied, err)
	}

	if err := provider.Destroy(ctx, app, "dev", namespace); err != nil {
		t.Fatal(err)
	}
	after, err := provider.Observe(ctx, app, "dev", namespace)
	if err != nil {
		t.Fatal(err)
	}
	if after.Found {
		t.Fatalf("workload remains after destroy: %#v", after)
	}
}
