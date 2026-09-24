package kubernetes

import (
	"bytes"
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

	rendered, err := Render(Plan{
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
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	name := app + "-app"
	cleanup := func() {
		for _, args := range [][]string{
			{"delete", "deployment", name, "-n", namespace, "--ignore-not-found", "--wait=true"},
			{"delete", "service", name, "-n", namespace, "--ignore-not-found"},
			{"delete", "secret", name + "-bindings", "-n", namespace, "--ignore-not-found"},
			{"delete", "configmap", name + "-config", "-n", namespace, "--ignore-not-found"},
		} {
			_ = exec.CommandContext(context.Background(), "kubectl", args...).Run()
		}
	}
	cleanup()
	t.Cleanup(cleanup)

	cmd := exec.CommandContext(ctx, "kubectl", "apply", "-f", "-")
	cmd.Stdin = bytes.NewReader(rendered)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("kubectl apply failed: %v\n%s\n%s", err, output, rendered)
	}

	if output, err := exec.CommandContext(
		ctx, "kubectl", "rollout", "status", "deployment/"+name,
		"-n", namespace, "--timeout=90s",
	).CombinedOutput(); err != nil {
		t.Fatalf("rollout failed: %v\n%s", err, output)
	}

	output, err := exec.CommandContext(
		ctx, "kubectl", "get", "deployment", name,
		"-n", namespace,
		"-o", "jsonpath={.metadata.labels.app\\.kubernetes\\.io/managed-by}",
	).CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(output)) != "baseharbor" {
		t.Fatalf("managed-by label = %q", output)
	}

	denied, err := exec.CommandContext(ctx, "kubectl", "auth", "can-i", "create", "namespaces").CombinedOutput()
	fields := strings.Fields(string(denied))
	if err == nil || len(fields) == 0 || fields[len(fields)-1] != "no" {
		t.Fatalf("expected namespace creation to remain denied, output=%q err=%v", denied, err)
	}
}
