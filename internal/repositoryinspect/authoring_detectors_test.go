package repositoryinspect

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
)

func TestAuthoringDetectorsSatisfyHTTPAndSecrets(t *testing.T) {
	root := t.TempDir()
	manifest := application.Manifest{
		Version:       application.CurrentVersion,
		ApplicationID: application.MustNewApplicationID(),
		Name:          "demo",
		Environment:   "dev",
		Services:      application.Services{Secrets: true},
		Secrets: application.SecretRequirements{
			Required: []application.SecretRequirement{{Name: "APP_SECRET"}},
		},
		Workload: application.WorkloadConfig{Components: []string{"app"}},
		Exposures: []application.HTTPExposureRequirement{{
			Name: "web", Service: "app", Port: 8080, Protocol: "http", Visibility: "public",
		}},
	}
	if err := os.WriteFile(filepath.Join(root, application.RepositoryManifestName), []byte(manifest.YAML()), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".env.example"), []byte("APP_SECRET=\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\nimport \"net/http\"\nfunc main(){ http.ListenAndServe(\":8080\", http.NewServeMux()) }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "compose.yaml"), []byte("services:\n  app:\n    image: example/app\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := Inspect(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	assertSatisfied := func(capability string) {
		t.Helper()
		for _, item := range result.Reconciliation {
			if item.Capability == capability && item.State == ReconciliationSatisfied {
				return
			}
		}
		t.Fatalf("%s not satisfied: %#v", capability, result.Reconciliation)
	}
	assertSatisfied("exposure.http")
	assertSatisfied("secrets")
}
