package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/deployment"
)

// versionMismatchDeployment writes a version-1 deployment.json (written by an
// older shipped BaseHarbor) under an owned target so the production
// deployment-owner binding passes but the version gate refuses the record. It
// returns the identity so callers can address the record by name.
func versionMismatchDeployment(t *testing.T, target deployment.ResolvedTarget, name, applicationID string) deployment.DeploymentIdentity {
	t.Helper()
	identity := deployment.DeploymentIdentity{
		DeploymentID:  "33333333-3333-4333-8333-333333333333",
		ApplicationID: applicationID,
		Target:        target.Name,
		Application:   name,
		Environment:   "dev",
	}
	if err := deployment.ValidateDeploymentID(identity.DeploymentID); err != nil {
		t.Fatal(err)
	}
	root, err := deployment.DeploymentRoot(identity)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	record := deployment.DeploymentRecord{
		Version:  1,
		Identity: identity,
		Source:   deployment.DeploymentSource{Kind: "repository", Repository: "/nonexistent"},
		Applied:  deployment.AppliedDeployment{RuntimeProvider: target.RuntimeProvider},
	}
	data, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "deployment.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	return identity
}

// TestAppStatusReportsVersionMismatchNotNotRegistered proves that a managed
// deployment whose deployment.json was written by an older shipped BaseHarbor
// (record version 1) surfaces the version reason on the human surface instead
// of the false "no registered deployment" the CLI previously printed. Fail-closed
// (nothing loaded/migrated) is preserved; only the message is repaired.
func TestAppStatusReportsVersionMismatchNotNotRegistered(t *testing.T) {
	configureTestTarget(t)
	target := deployment.ResolvedTarget{Name: "docker-dev", RuntimeProvider: "docker", AccessProvider: "", Scope: "local"}
	versionMismatchDeployment(t, target, "oldstatus", "app-oldstatus-0001")

	var out, errOut bytes.Buffer
	err := runWithIO(context.Background(), []string{"app", "status", "oldstatus"}, &out, &errOut)
	if err == nil {
		t.Fatalf("status of a version-gated record must fail closed")
	}
	msg := err.Error()
	if strings.Contains(msg, "no registered deployment") {
		t.Fatalf("status must not print the masked 'no registered deployment' lie: %q", msg)
	}
	if !strings.Contains(msg, "version 1") || !strings.Contains(msg, "version 2") {
		t.Fatalf("status must name the found and supported versions: %q", msg)
	}
}

// TestAppDestroyReportsVersionMismatchNotNotRegistered proves the destroy
// surface surfaces the version reason instead of "no registered deployment".
func TestAppDestroyReportsVersionMismatchNotNotRegistered(t *testing.T) {
	configureTestTarget(t)
	target := deployment.ResolvedTarget{Name: "docker-dev", RuntimeProvider: "docker", AccessProvider: "", Scope: "local"}
	versionMismatchDeployment(t, target, "olddestroy", "app-olddestroy-0001")

	var out, errOut bytes.Buffer
	err := runWithIO(context.Background(), []string{"app", "destroy", "olddestroy", "--yes"}, &out, &errOut)
	if err == nil {
		t.Fatalf("destroy of a version-gated record must fail closed")
	}
	msg := err.Error()
	if strings.Contains(msg, "no registered deployment") {
		t.Fatalf("destroy must not print the masked 'no registered deployment' lie: %q", msg)
	}
	if !strings.Contains(msg, "version 1") || !strings.Contains(msg, "version 2") {
		t.Fatalf("destroy must name the found and supported versions: %q", msg)
	}
}
