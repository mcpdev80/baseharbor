package kubernetes

import (
	"strings"
	"testing"

	runtimemodel "github.com/mcpdev80/baseharbor/internal/runtime/model"
	"github.com/mcpdev80/baseharbor/internal/workload"
)

func TestRenderReferenceDemoUsesArtifactOverrideAndNamespacedResources(t *testing.T) {
	model := workload.Model{Services: []workload.Service{{
		Name:  "demo-app",
		Build: &workload.Build{Context: "./demo-app"},
		Environment: map[string]string{
			"PORT":         "8080",
			"DATABASE_URL": "${DATABASE_URL:-postgres://standalone}",
			"REDIS_URL":    "${REDIS_URL:-redis://standalone}",
			"S3_ENDPOINT":  "${S3_ENDPOINT:-http://standalone:9000}",
			"S3_BUCKET":    "${S3_BUCKET:-uploads}",
			"APP_SECRET":   "${APP_SECRET:-standalone-secret}",
		},
		Ports: []workload.Port{{Container: 8080, Protocol: "tcp"}},
	}}}

	out, err := Render(Plan{
		Application: "baseharbor-demo",
		Environment: "dev",
		Target:      runtimemodel.Target{Scope: "baseharbor-ci"},
		Workload:    model,
		Images: map[string]string{
			"demo-app": "registry.example/baseharbor-demo@sha256:deadbeef",
		},
		Bindings: map[string]Binding{
			"DATABASE_URL": {Value: "postgresql://managed", Sensitive: true},
			"REDIS_URL":    {Value: "redis://managed", Sensitive: true},
			"S3_ENDPOINT":  {Value: "http://s3.internal:9000"},
			"S3_BUCKET":    {Value: "uploads"},
			"APP_SECRET":   {Value: "managed-secret", Sensitive: true},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	text := string(out)
	for _, want := range []string{
		"kind: Deployment",
		"kind: Service",
		"kind: ConfigMap",
		"kind: Secret",
		"namespace: baseharbor-ci",
		"app.kubernetes.io/managed-by: baseharbor",
		"baseharbor.io/application: baseharbor-demo",
		"baseharbor.io/environment: dev",
		"image: registry.example/baseharbor-demo@sha256:deadbeef",
		"containerPort: 8080",
		"port: 8080",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("rendered Kubernetes resources missing %q:\n%s", want, text)
		}
	}
	for _, forbidden := range []string{
		"kind: Namespace",
		"kind: ClusterRole",
		"kind: ClusterRoleBinding",
		"kind: CustomResourceDefinition",
		"kind: StorageClass",
		"kind: GatewayClass",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("rendered forbidden cluster-scoped object %q:\n%s", forbidden, text)
		}
	}
	if strings.Contains(text, "standalone-secret") ||
		strings.Contains(text, "postgres://standalone") ||
		strings.Contains(text, "redis://standalone") {
		t.Fatalf("standalone Compose fallback leaked over managed bindings:\n%s", text)
	}
}

func TestRenderBuildBackedServiceRequiresResolvedArtifact(t *testing.T) {
	_, err := Render(Plan{
		Application: "demo",
		Environment: "dev",
		Target:      runtimemodel.Target{Scope: "baseharbor-ci"},
		Workload: workload.Model{Services: []workload.Service{{
			Name:  "app",
			Build: &workload.Build{Context: "."},
		}}},
	})
	if err == nil || !strings.Contains(err.Error(), "resolve it to an OCI image artifact") {
		t.Fatalf("expected artifact resolution failure, got %v", err)
	}
}
