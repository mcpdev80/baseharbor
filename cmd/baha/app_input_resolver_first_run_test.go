package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/identity"
	"github.com/mcpdev80/baseharbor/internal/operatorauth"
)

func TestRepositoryRuntimeInitRecordsPendingDeploymentBeforeStateMutation(t *testing.T) {
	target := configureTestTarget(t)
	repo := t.TempDir()
	m := application.Manifest{
		Version:       application.CurrentVersion,
		ApplicationID: application.MustNewApplicationID(),
		Name:          "demo",
		Environment:   "test",
		Workload:      application.WorkloadConfig{Components: []string{"app"}},
	}
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(repo, application.RepositoryManifestName)
	if err := os.WriteFile(manifestPath, []byte(m.YAML()), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "compose.yaml"), []byte("services:\n  app:\n    image: alpine:3.20\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWD)
	if err := os.Chdir(repo); err != nil {
		t.Fatal(err)
	}

	resolved, err := resolveApplication(context.Background(), application.DefaultStore(), nil, "up")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.DeploymentRecord != nil {
		t.Fatal("fresh repository unexpectedly has a deployment record")
	}

	var out bytes.Buffer
	if err := runRepositoryRuntimeInitResolved(operatorauth.WithVerifiedPrincipal(context.Background(), &identity.Principal{Subject: "authenticated-fixture", Issuer: "https://issuer.example"}), resolved, repo, repositoryInitOptions{Yes: true}, &out); err != nil {
		t.Fatalf("initialize repository deployment inputs: %v\n%s", err, out.String())
	}

	record, found, err := deployment.FindDeployment(target.Name, m.ApplicationID, m.Environment)
	if err != nil {
		t.Fatalf("find deployment after input initialization: %v", err)
	}
	if !found {
		t.Fatal("deployment record was not created before deployment-local state")
	}
	if record.Identity.Application != m.Name || record.Identity.Environment != m.Environment {
		t.Fatalf("deployment identity = %#v", record.Identity)
	}
	root, err := deployment.DeploymentRoot(record.Identity)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "deployment.json")); err != nil {
		t.Fatalf("deployment record missing: %v", err)
	}
	if _, err := os.Stat(repositoryInitEnvPathFromStateRoot(root)); err != nil {
		t.Fatalf("deployment input state missing: %v", err)
	}

	if _, err := resolveApplication(context.Background(), application.DefaultStore(), nil, "up"); err != nil {
		t.Fatalf("repository resolve after first-run input initialization: %v", err)
	}
}
