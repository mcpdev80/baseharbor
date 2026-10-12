package kubernetes

import (
	"context"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/artifact"
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

	plan, err := artifact.RuntimePlan("baseharbor-demo", "dev", "baseharbor-ci", model, map[string]string{"demo-app": "registry.example/baseharbor-demo@sha256:deadbeef"}, []Binding{
		{Name: "DATABASE_URL", SecretRef: "kubernetes://baseharbor-ci/demo-bindings/DATABASE_URL"},
		{Name: "REDIS_URL", SecretRef: "kubernetes://baseharbor-ci/demo-bindings/REDIS_URL"},
		{Name: "APP_SECRET", SecretRef: "kubernetes://baseharbor-ci/demo-bindings/APP_SECRET"},
		{Name: "S3_ENDPOINT", PublicValue: "http://s3.internal:9000"},
		{Name: "S3_BUCKET", PublicValue: "uploads"},
	})
	if err != nil {
		t.Fatal(err)
	}
	out, err := Render(plan)
	if err != nil {
		t.Fatal(err)
	}
	text := string(out)
	for _, want := range []string{
		"kind: Deployment",
		"kind: Service",
		"kind: ConfigMap",
		"secretKeyRef:",
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
	_, err := artifact.RuntimePlan("demo", "dev", "baseharbor-ci", workload.Model{Services: []workload.Service{{Name: "app", Build: &workload.Build{Context: "."}}}}, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "resolve it to an OCI image artifact") {
		t.Fatalf("expected artifact resolution failure, got %v", err)
	}
}

func TestRenderRejectsForeignOrCredentialBearingSecretReference(t *testing.T) {
	for _, reference := range []string{"kubernetes://foreign/secret/key", "kubernetes://user:credential@baseharbor-ci/secret/key", "kubernetes://baseharbor-ci/secret/key?token=private", "unsupported://baseharbor-ci/secret/key"} {
		plan := Plan{Application: "app", Environment: "dev", Target: runtimemodel.Target{Scope: "baseharbor-ci"}, Services: []runtimemodel.Service{{Name: "app", Image: "example/app:1"}}, Bindings: []Binding{{Name: "APP_SECRET", SecretRef: reference}}}
		if _, err := Render(plan); err == nil || strings.Contains(err.Error(), "credential") || strings.Contains(err.Error(), "private") {
			t.Fatal("unsafe secret reference accepted or leaked")
		}
	}
}

func TestApplyRejectsForeignSelectedScopeBeforeExecution(t *testing.T) {
	provider := Provider{namespace: "selected", kubectl: "must-not-run"}
	plan := Plan{Application: "app", Environment: "dev", Target: runtimemodel.Target{Scope: "other"}, Services: []runtimemodel.Service{{Name: "app", Image: "example/app:1"}}}
	if err := provider.Apply(context.Background(), plan); err == nil || !strings.Contains(err.Error(), "scope differs") {
		t.Fatal("Apply bypassed current semantic preflight")
	}
}
