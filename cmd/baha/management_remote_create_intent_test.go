package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
)

func TestRemoteCreateIntentUsesMatchingRepositoryContract(t *testing.T) {
	root := t.TempDir()
	manifest := `version: 1
app:
  name: demo
  environment: dev
services:
  sql:
    enabled: true
  cache:
    enabled: true
runtime:
  permissions:
    - capability: object-storage.s3/v1
      services:
        - web
      operations:
        - runtime.create
workload:
  components:
    - web
metrics:
  sources:
    - name: app
      service: web
      port: 8080
      path: /metrics
exposure:
  http:
    - name: web
      service: web
      port: 8080
      protocol: https
      visibility: public
`
	if err := os.WriteFile(filepath.Join(root, application.RepositoryManifestName), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	fallback := application.New("demo", "dev", true, false, false)
	fallback.ApplicationID = "client-id-must-not-be-used"

	got, matched, err := remoteCreateIntentFromRepository([]string{"demo"}, fallback)
	if err != nil {
		t.Fatal(err)
	}
	if !matched {
		t.Fatal("matching repository contract was not selected")
	}
	if got.ApplicationID != "" || !got.Services.SQL || !got.Services.Cache || len(got.Runtime.Permissions) != 1 ||
		len(got.Workload.Components) != 1 || len(got.Metrics.Sources) != 1 || len(got.Exposures) != 1 {
		t.Fatalf("remote create did not preserve the full repository intent without client identity: %+v", got)
	}
}

func TestRemoteCreateIntentRejectsServiceOverridesAndEnvironmentMismatch(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, application.RepositoryManifestName), []byte("version: 1\napp:\n  name: demo\n  environment: dev\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	fallback := application.New("demo", "dev", true, false, false)
	for _, args := range [][]string{{"demo", "--sql"}, {"demo", "--cache"}} {
		if _, _, err := remoteCreateIntentFromRepository(args, fallback); err == nil ||
			!strings.Contains(err.Error(), "service options cannot override") {
			t.Fatalf("args %v: expected conflicting service flags to be rejected, got %v", args, err)
		}
	}
	if _, _, err := remoteCreateIntentFromRepository([]string{"demo", "--environment", "prod"}, fallback); err == nil ||
		!strings.Contains(err.Error(), "environment does not match") {
		t.Fatalf("expected repository environment mismatch to fail, got %v", err)
	}
}

func TestRemoteCreateIntentFallsBackOutsideMatchingRepository(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, application.RepositoryManifestName), []byte("version: 1\napp:\n  name: another\n  environment: dev\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	fallback := application.New("demo", "dev", true, false, false)
	got, matched, err := remoteCreateIntentFromRepository([]string{"demo", "--cache"}, fallback)
	if err != nil {
		t.Fatal(err)
	}
	if matched || got.Name != fallback.Name || !got.Services.SQL {
		t.Fatalf("unrelated repository changed target-managed create intent: matched=%v intent=%+v", matched, got)
	}
}

func TestPersistRemoteRepositoryIdentityUsesOnlyCoreAssignedIdentity(t *testing.T) {
	root := t.TempDir()
	intent := application.New("demo", "dev", true, true, false)
	coreID := application.MustNewApplicationID()
	path, err := persistRemoteRepositoryIdentity(root, intent, coreID)
	if err != nil {
		t.Fatal(err)
	}
	stored, _, err := (application.Store{Root: filepath.Join(root, ".baseharbor", "apps")}).Load("demo")
	if err != nil {
		t.Fatal(err)
	}
	if stored.ApplicationID != coreID {
		t.Fatalf("stored ID = %q, want Core ID %q", stored.ApplicationID, coreID)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("protected identity mode = %v, error = %v", info, err)
	}
	if _, err := persistRemoteRepositoryIdentity(root, intent, application.MustNewApplicationID()); err == nil {
		t.Fatal("different Core identity replaced the repository's existing owner")
	}
}
