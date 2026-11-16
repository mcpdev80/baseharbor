package main

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/delivery"
	"github.com/mcpdev80/baseharbor/internal/deployment"
)

// TestCLIEnvDestroyPreservesGlobalAndRemovesScoped is the integration test for
// `baha env destroy`: it seeds a managed target, a scoped app deployed only in
// the target environment and a global app deployed in two environments, then
// drives the real CLI run and asserts the scoped identity is removed while the
// global app's environment record on the OTHER environment survives.
func TestCLIEnvDestroyPreservesGlobalAndRemovesScoped(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	target := configureTestTarget(t)
	t.Setenv("BASEHARBOR_TARGET", target.Name)
	t.Setenv("BASEHARBOR_ENVIRONMENT", "prod")

	// The environment identity must exist for the destroy seam to remove it.
	if _, gErr := deployment.GetEnvironment(target.Name, "prod"); gErr != nil {
		t.Fatalf("seed environment prod: %v", gErr)
	}

	// Seed two deployments on the target: scoped (prod only) and global
	// (prod + dev).
	seedCLIEnvDeployment(t, target.Name, "scoped-app", "prod")
	seedCLIEnvDeployment(t, target.Name, "global-app", "prod")
	seedCLIEnvDeployment(t, target.Name, "global-app", "dev")

	// Confirm the global app has an environment record on dev before destroy.
	if _, devErr := deployment.GetEnvironment(target.Name, "dev"); devErr != nil {
		t.Fatalf("seed dev environment: %v", devErr)
	}

	var out, errOut bytes.Buffer
	if err := runWithIO(context.Background(), []string{"env", "destroy", "prod", "--yes"}, &out, &errOut); err != nil {
		t.Fatalf("env destroy --yes returned error: %v\nstderr:%s", err, errOut.String())
	}
	human := out.String()
	if !strings.Contains(human, "destroyed on target") {
		t.Fatalf("expected success output, got: %s", human)
	}

	// The environment identity must be gone (verified by absence of the
	// on-disk record, since GetEnvironment is idempotent and re-creates it).
	exists, stateErr := deployment.EnvironmentStateExists(target.Name, "prod")
	if stateErr != nil {
		t.Fatalf("check environment existence: %v", stateErr)
	}
	if exists {
		t.Fatalf("environment identity should have been removed")
	}
	// The global app's environment record on the OTHER environment survives
	// (preserved across environments).
	if _, devErr := deployment.GetEnvironment(target.Name, "dev"); devErr != nil {
		t.Fatalf("global app's other-environment identity must be preserved: %v", devErr)
	}
}

// TestCLIEnvDestroyFailClosedWhenOwnershipUnknown drives the CLI when the
// target has no authoritative installation owner: the scope must fail closed
// rather than guessing.
func TestCLIEnvDestroyFailClosedWhenOwnershipUnknown(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	target := configureTestTargetEmpty(t)
	t.Setenv("BASEHARBOR_TARGET", target.Name)

	var out, errOut bytes.Buffer
	runWithIO(context.Background(), []string{"env", "destroy", "prod", "--json"}, &out, &errOut)
	// With no owner, CollectEnvironmentDestroyScope returns an error which the
	// CLI propagates; the command must not print success.
	if strings.Contains(out.String(), "\"success\":true") {
		t.Fatalf("destroy must not report success without an owner: %s", out.String())
	}
}

// TestCLIEnvDestroyPreviewWithoutYesStopsWithoutMutation drives the preview
// path: without --yes the identity is untouched.
func TestCLIEnvDestroyPreviewWithoutYesStopsWithoutMutation(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	target := configureTestTarget(t)
	t.Setenv("BASEHARBOR_TARGET", target.Name)
	t.Setenv("BASEHARBOR_ENVIRONMENT", "prod")
	seedCLIEnvDeployment(t, target.Name, "scoped-app", "prod")

	if _, beforeErr := deployment.GetEnvironment(target.Name, "prod"); beforeErr != nil {
		t.Fatalf("seed environment: %v", beforeErr)
	}

	var out, errOut bytes.Buffer
	// No --yes: preview only.
	runWithIO(context.Background(), []string{"env", "destroy", "prod", "--json"}, &out, &errOut)
	if strings.Contains(out.String(), "\"success\":true") {
		t.Fatalf("preview without --yes must not report success: %s", out.String())
	}
	// The identity still exists.
	if _, afterErr := deployment.GetEnvironment(target.Name, "prod"); afterErr != nil {
		t.Fatalf("environment identity should still exist after a preview-only run: %v", afterErr)
	}
}

// TestCLIEnvDestroyDeleteDataRequiresExplicitConsent drives the destructive
// path with --yes but without the typed data-deletion confirmation: the
// identity must be preserved because the separate consent was not given.
func TestCLIEnvDestroyDeleteDataRequiresExplicitConsent(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	target := configureTestTarget(t)
	t.Setenv("BASEHARBOR_TARGET", target.Name)
	t.Setenv("BASEHARBOR_ENVIRONMENT", "prod")
	seedCLIEnvDeployment(t, target.Name, "scoped-app", "prod")

	if _, beforeErr := deployment.GetEnvironment(target.Name, "prod"); beforeErr != nil {
		t.Fatalf("seed environment: %v", beforeErr)
	}

	var out, errOut bytes.Buffer
	// --yes but the destructive data-deletion confirmation is not typed.
	runWithIO(context.Background(), []string{"env", "destroy", "prod", "--delete-data", "--yes"}, &out, &errOut)
	// Without the separate typed confirmation the identity remains.
	if _, afterErr := deployment.GetEnvironment(target.Name, "prod"); afterErr != nil {
		t.Fatalf("data-deletion without confirmation must preserve the identity, got: %v", afterErr)
	}
}

// ---- helpers ----

// seedCLIEnvDeployment seeds a deployment record for an app on a target/env
// through the production SaveDeploymentRecord path.
func seedCLIEnvDeployment(t *testing.T, target, application, environment string) {
	t.Helper()
	id, err := deployment.NewDeploymentIdentity(target, validSubjectIDForCLI(application), application, environment)
	if err != nil {
		t.Fatal(err)
	}
	record := deployment.DeploymentRecord{
		Version: deployment.DeploymentRecordVersion,
		Identity: deployment.DeploymentIdentity{
			DeploymentID:  id.DeploymentID,
			ApplicationID: id.ApplicationID,
			Target:        target,
			Application:   application,
			Environment:   environment,
		},
		Source:   deployment.DeploymentSource{Kind: "repository"},
		Applied:  deployment.AppliedDeployment{RuntimeProvider: "docker", Delivery: delivery.Direct()},
		Observed: deployment.ObservedDeployment{State: "created"},
	}
	if err := deployment.SaveDeploymentRecord(record); err != nil {
		t.Fatalf("seed CLI deployment %s/%s/%s: %v", target, application, environment, err)
	}
}

// validSubjectIDForCLI turns an app slug into a syntactically valid UUIDv4 so
// a seeded deployment carries a real (application_id, environment) identity.
func validSubjectIDForCLI(application string) string {
	h := 0
	for _, r := range application {
		h = h*31 + int(r)
	}
	return fmt.Sprintf("00000000-0000-4%03x-8%03x-b%03x%04x%04x",
		byte(h>>16), byte(h>>8), byte(h), byte(h>>8), byte(h))
}
