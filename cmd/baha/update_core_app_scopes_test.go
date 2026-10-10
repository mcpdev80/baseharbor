package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/openbao"
)

func TestVerifyApplicationSecretScopeRecordsUsesProtectedDeploymentCredentials(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	id, err := deployment.NewDeploymentIdentity("dev", "11111111-1111-4111-8111-111111111111", "payment", "dev")
	if err != nil {
		t.Fatal(err)
	}
	root, err := deployment.DeploymentRoot(id)
	if err != nil {
		t.Fatal(err)
	}
	runtimeDir := filepath.Join(root, "state", "payment", "runtime")
	if err := os.MkdirAll(runtimeDir, 0700); err != nil {
		t.Fatal(err)
	}
	creds := openbao.ApplicationCredentialsPath(runtimeDir)
	if err := os.WriteFile(creds, []byte("protected-app-credentials"), 0600); err != nil {
		t.Fatal(err)
	}
	record := deployment.DeploymentRecord{Identity: id, Applied: deployment.AppliedDeployment{
		Intent:         json.RawMessage(`{"name":"payment","environment":"dev","services":{"secrets":true}}`),
		GeneratedState: map[string]string{"runtime": runtimeDir},
	}}
	checked := 0
	probe := func(_ context.Context, identity openbao.ApplicationIdentity, path string) error {
		checked++
		if identity.Name != "payment" || identity.Environment != "dev" || path != creds {
			t.Fatalf("incorrect app secret scope: %v %s", identity, path)
		}
		return nil
	}
	if err := verifyApplicationSecretScopeRecords([]deployment.DeploymentRecord{record}, probe, context.Background()); err != nil {
		t.Fatal(err)
	}
	if checked != 1 {
		t.Fatalf("application scope checks=%d, want 1", checked)
	}
	if err := os.Chmod(creds, 0644); err != nil {
		t.Fatal(err)
	}
	if err := verifyApplicationSecretScopeRecords([]deployment.DeploymentRecord{record}, probe, context.Background()); err == nil {
		t.Fatal("world-readable app credentials accepted")
	}
	if err := os.Chmod(creds, 0600); err != nil {
		t.Fatal(err)
	}
	record.Applied.GeneratedState["runtime"] = t.TempDir()
	if err := verifyApplicationSecretScopeRecords([]deployment.DeploymentRecord{record}, probe, context.Background()); err == nil {
		t.Fatal("foreign application runtime accepted")
	}
	record.Applied.GeneratedState["runtime"] = runtimeDir
	record.Applied.Intent = json.RawMessage(`{"name":"other","environment":"dev","services":{"secrets":true}}`)
	if err := verifyApplicationSecretScopeRecords([]deployment.DeploymentRecord{record}, probe, context.Background()); err == nil {
		t.Fatal("mismatched application identity accepted")
	}
}

func TestVerifyApplicationSecretScopesSkipsExplicitlySecretlessApps(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	id, err := deployment.NewDeploymentIdentity("dev", "11111111-1111-4111-8111-111111111111", "public", "dev")
	if err != nil {
		t.Fatal(err)
	}
	record := deployment.DeploymentRecord{Identity: id, Applied: deployment.AppliedDeployment{Intent: json.RawMessage(`{"name":"public","environment":"dev","services":{"secrets":false}}`)}}
	calls := 0
	probe := func(context.Context, openbao.ApplicationIdentity, string) error { calls++; return nil }
	if err := verifyApplicationSecretScopeRecords([]deployment.DeploymentRecord{record}, probe, context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Fatal("secretless app unexpectedly required OpenBao credentials")
	}
}
