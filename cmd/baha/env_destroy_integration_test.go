package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/delivery"
	"github.com/mcpdev80/baseharbor/internal/deployment"
)

// TestEnvironmentDestroyNativeLifecycle proves KEEP retires the selected
// environment's real workload while retaining its deployment owner and source.
// CI runs it once on each isolated Docker and Podman Coreless lifecycle runner.
func TestEnvironmentDestroyNativeLifecycle(t *testing.T) {
	if os.Getenv("BASEHARBOR_ENV_DESTROY_ACCEPTANCE") != "1" {
		t.Skip("requires isolated native environment-destroy acceptance")
	}
	ctx, cancel := context.WithTimeout(machineNoninteractiveContext(context.Background()), 8*time.Minute)
	defer cancel()
	ctx = testOperatorContext(ctx)
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	target := configureTestTargetEmpty(t)
	t.Setenv("BASEHARBOR_TARGET", target.Name)
	t.Setenv("BASEHARBOR_ENVIRONMENT", "prod")
	if os.Getenv("BASEHARBOR_TEST_RUNTIME") == "podman" {
		cfg, err := deployment.LoadConfig()
		if err != nil {
			t.Fatal(err)
		}
		entry := cfg.Targets[target.Name]
		entry.Runtime.Provider = "podman"
		cfg.Targets[target.Name] = entry
		if err := cfg.Save(); err != nil {
			t.Fatal(err)
		}
		target, err = cfg.ResolveTarget("", "")
		if err != nil {
			t.Fatal(err)
		}
	}
	runtime, err := detectRuntimeForTarget(ctx, target)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	t.Chdir(root)
	authored := "version: 1\napp:\n  name: env-destroy-native\n  environment: prod\n"
	if err := os.WriteFile(filepath.Join(root, "baseharbor.yaml"), []byte(authored), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "compose.yaml"), []byte(corelessHTTPWorkload), 0o600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := runWithIOAsTestOperator(ctx, []string{"app", "init", "--quick", "--json"}, &output, &output); err != nil {
		t.Fatalf("initialize native environment workload: %v\n%s", err, output.String())
	}
	destroyed := false
	t.Cleanup(func() {
		if destroyed {
			return
		}
		cleanupCtx, cleanupCancel := context.WithTimeout(machineNoninteractiveContext(context.Background()), 2*time.Minute)
		defer cleanupCancel()
		if err := destroySelectiveApplicationWithConsent(testOperatorContext(cleanupCtx)); err != nil {
			t.Errorf("native environment-destroy fixture cleanup: %v", err)
		}
	})
	if _, err := deployment.GetEnvironment(target.Name, "prod"); err != nil {
		t.Fatalf("create prod environment identity: %v", err)
	}
	if err := ensureRepositoryDeploymentInputsForUp(ctx, strings.NewReader(""), io.Discard, runtimeUpOptions{Yes: true}); err != nil {
		t.Fatalf("prepare native workload deployment inputs: %v", err)
	}
	resolved := applySelectiveIdentityIntent(t, ctx)
	status, _, err := collectResolvedApplicationStatus(ctx, resolved)
	if err != nil || !status.Ready {
		t.Fatalf("native prod workload did not become ready: %+v %v", status, err)
	}
	files, err := application.ExistingRuntimeFiles(resolved.Store, resolved.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	workload, found, err := application.MaterializeWorkload(root, resolved.Manifest, files)
	if err != nil || !found {
		t.Fatalf("materialize native workload inventory: found=%t err=%v", found, err)
	}
	containers, err := runtime.ListRuntimeContainers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	workloadRunning := false
	for _, container := range containers {
		if container.Project == workload.Project {
			workloadRunning = true
		}
	}
	if !workloadRunning {
		t.Fatal("native prod workload had no owned runtime container before destroy")
	}
	stopCtx := withApplicationInput(withTargetOverride(ctx, target.Name), runtimeInput)
	stopCtx = context.WithValue(stopCtx, applicationStopModeKey{}, true)
	stopPlan, err := buildApplicationStopPlan(stopCtx, resolved)
	if err != nil {
		t.Fatalf("build exact application stop consent: %v", err)
	}
	consent, err := json.Marshal(application.ProviderChangeConsent{PlanID: stopPlan.Scope.PlanID, First: true, Second: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(providerChangeConsentEnv, string(consent))
	output.Reset()
	if err := runWithIOAsTestOperator(ctx, []string{"env", "destroy", "prod", "--keep-data", "--yes"}, &output, &output); err != nil {
		t.Fatalf("native env destroy: %v\n%s", err, output.String())
	}
	destroyed = true
	if exists, err := deployment.EnvironmentStateExists(target.Name, "prod"); err != nil || exists {
		t.Fatalf("native environment identity remains after success: exists=%t err=%v", exists, err)
	}
	record, found, err := deployment.FindDeployment(target.Name, resolved.Manifest.ApplicationID, "prod")
	if err != nil || !found || record.Observed.State != "stopped" || record.Observed.Ready {
		t.Fatalf("KEEP must retain the stopped deployment owner: found=%t state=%q ready=%t err=%v", found, record.Observed.State, record.Observed.Ready, err)
	}
	containers, err = runtime.ListRuntimeContainers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, container := range containers {
		if container.Project == workload.Project {
			t.Fatalf("native environment destroy left selected prod container %s running", container.ID)
		}
	}
	if source, err := os.ReadFile(filepath.Join(root, "baseharbor.yaml")); err != nil || string(source) != authored {
		t.Fatalf("KEEP changed authored source: err=%v", err)
	}
	t.Log("native env destroy retired the prod workload and removed its environment identity while retaining its deployment owner, recovery state and authored source")
}

// TestCLIEnvDestroyFailsClosedWhenAppliedIntentIsMissing seeds scoped and
// shared deployments without the durable intent required by the application
// lifecycle. The CLI must retain the environment identity for repair/retry and
// must not mutate the deployment registry or sibling environment.
func TestCLIEnvDestroyFailsClosedWhenAppliedIntentIsMissing(t *testing.T) {
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
	if err := runWithIO(context.Background(), []string{"env", "destroy", "prod", "--yes"}, &out, &errOut); err == nil || !strings.Contains(err.Error(), "no applied intent snapshot") {
		t.Fatalf("expected fail-closed missing-intent error, got %v; stderr:%s", err, errOut.String())
	}

	// Identity stays present after a failed scoped lifecycle step so a retry can
	// re-authorize the current state.
	exists, stateErr := deployment.EnvironmentStateExists(target.Name, "prod")
	if stateErr != nil {
		t.Fatalf("check environment existence: %v", stateErr)
	}
	if !exists {
		t.Fatalf("environment identity must remain after failed destroy")
	}
	if _, err := deployment.ListDeployments(target.Name); err != nil {
		t.Fatalf("deployment registry should remain readable: %v", err)
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
	if !strings.Contains(out.String(), `"data_choice":"keep-data"`) || !strings.Contains(out.String(), `"scope":"scoped"`) {
		t.Fatalf("preview must expose the retained-data choice and scoped deployment: %s", out.String())
	}
	// The identity still exists.
	if _, afterErr := deployment.GetEnvironment(target.Name, "prod"); afterErr != nil {
		t.Fatalf("environment identity should still exist after a preview-only run: %v", afterErr)
	}
}

func TestCLIEnvDestroyConfirmedJSONEmitsOneResult(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	target := configureTestTarget(t)
	t.Setenv("BASEHARBOR_TARGET", target.Name)
	t.Setenv("BASEHARBOR_ENVIRONMENT", "prod")
	if _, err := deployment.GetEnvironment(target.Name, "prod"); err != nil {
		t.Fatalf("seed environment: %v", err)
	}

	var out, errOut bytes.Buffer
	if err := envDestroyCommandRun(context.Background(), []string{"prod", "--yes", "--json"}, &out, &errOut); err != nil {
		t.Fatalf("confirmed env destroy: %v; stderr: %s", err, errOut.String())
	}
	var result envDestroyResult
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("confirmed JSON output must be exactly one result object, got %q: %v", out.String(), err)
	}
	if !result.Success || result.Target != target.Name || result.Environment != "prod" {
		t.Fatalf("unexpected destroy result: %+v", result)
	}
	if exists, err := deployment.EnvironmentStateExists(target.Name, "prod"); err != nil || exists {
		t.Fatalf("confirmed empty-scope destroy should remove the identity: exists=%v err=%v", exists, err)
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

func TestCLIEnvDestroyDeleteDataNoInputRequiresSeparateFlag(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	target := configureTestTarget(t)
	t.Setenv("BASEHARBOR_TARGET", target.Name)
	seedCLIEnvDeployment(t, target.Name, "scoped-app", "prod")
	if _, err := deployment.GetEnvironment(target.Name, "prod"); err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	if err := runWithIO(context.Background(), []string{"--no-input", "env", "destroy", "prod", "--delete-data", "--yes"}, &out, &errOut); err != nil {
		t.Fatalf("missing delete-data authorization should stop without error: %v", err)
	}
	if !strings.Contains(errOut.String(), "Data deletion consent not given") {
		t.Fatalf("expected separate consent refusal, stderr=%s", errOut.String())
	}
	if exists, err := deployment.EnvironmentStateExists(target.Name, "prod"); err != nil || !exists {
		t.Fatalf("identity must remain without separate consent: exists=%v err=%v", exists, err)
	}

	// The explicit non-interactive consent advances to lifecycle validation,
	// where this deliberately incomplete fixture fails closed on missing intent.
	err := runWithIO(context.Background(), []string{"--no-input", "env", "destroy", "prod", "--delete-data", "--confirm-delete-data", "--yes"}, &out, &errOut)
	if err == nil || !strings.Contains(err.Error(), "no applied intent snapshot") {
		t.Fatalf("explicit consent should reach the scoped lifecycle, got %v", err)
	}
	if exists, err := deployment.EnvironmentStateExists(target.Name, "prod"); err != nil || !exists {
		t.Fatalf("identity must remain after lifecycle refusal: exists=%v err=%v", exists, err)
	}
}

func TestEnvironmentDestroyKeepStopsScopedOnlyAndRetainsOwnerAcrossRetry(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	target := configureTestTarget(t)
	if _, err := deployment.GetEnvironment(target.Name, "prod"); err != nil {
		t.Fatal(err)
	}
	seedCLIEnvDeployment(t, target.Name, "scoped-one", "prod")
	seedCLIEnvDeployment(t, target.Name, "scoped-two", "prod")
	seedCLIEnvDeployment(t, target.Name, "shared-app", "prod")
	seedCLIEnvDeployment(t, target.Name, "shared-app", "dev")

	scope, err := deployment.CollectEnvironmentDestroyScope(target.Name, "prod", deployment.EnvironmentDestroyKeepData)
	if err != nil {
		t.Fatal(err)
	}
	if len(scope.Scoped) != 3 || len(scope.Preserved) != 1 {
		t.Fatalf("expected three scoped and one preserved sibling deployment: scoped=%v preserved=%v", scope.Scoped, scope.Preserved)
	}
	seen := map[string]int{}
	apply := func(_ context.Context, dep deployment.EnvironmentDestroyDeployment, choice deployment.EnvironmentDestroyDataChoice) error {
		if choice != deployment.EnvironmentDestroyKeepData {
			t.Fatalf("keep path passed choice %q", choice)
		}
		seen[dep.DeploymentID]++
		if len(seen) == 3 {
			return fmt.Errorf("simulated second-runtime stop failure")
		}
		return nil
	}
	if err := applyAndRemoveEnvironmentIdentity(context.Background(), target.Name, "prod", scope, apply); err == nil {
		t.Fatal("expected partial failure")
	}
	if exists, err := deployment.EnvironmentStateExists(target.Name, "prod"); err != nil || !exists {
		t.Fatalf("environment identity must remain after partial failure: exists=%v err=%v", exists, err)
	}
	for id := range seen {
		if present, err := environmentDestroyDeploymentPresent(target.Name, id); err != nil || !present {
			t.Fatalf("KEEP must retain canonical deployment owner %s: present=%v err=%v", id, present, err)
		}
	}

	// A retry recomputes the scope; each scoped runtime may be stopped again
	// while its durable owner remains, and the other environment's sibling is never selected.
	retryScope, err := deployment.CollectEnvironmentDestroyScope(target.Name, "prod", deployment.EnvironmentDestroyKeepData)
	if err != nil {
		t.Fatal(err)
	}
	var retryIDs []string
	if err := applyAndRemoveEnvironmentIdentity(context.Background(), target.Name, "prod", retryScope, func(_ context.Context, dep deployment.EnvironmentDestroyDeployment, _ deployment.EnvironmentDestroyDataChoice) error {
		retryIDs = append(retryIDs, dep.DeploymentID)
		return nil
	}); err != nil {
		t.Fatalf("retry failed: %v", err)
	}
	if len(retryIDs) != 3 {
		t.Fatalf("retry must stop exactly the three prod deployments, got %v", retryIDs)
	}
	if exists, err := deployment.EnvironmentStateExists(target.Name, "prod"); err != nil || exists {
		t.Fatalf("environment identity should be removed after successful retry: exists=%v err=%v", exists, err)
	}
	if _, err := deployment.GetEnvironment(target.Name, "dev"); err != nil {
		t.Fatalf("other environment was not preserved: %v", err)
	}
}

func TestEnvironmentDestroyDeleteDataKeepsIdentityUntilAllScopedOwnersAreDeleted(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	target := configureTestTarget(t)
	if _, err := deployment.GetEnvironment(target.Name, "prod"); err != nil {
		t.Fatal(err)
	}
	seedCLIEnvDeployment(t, target.Name, "delete-one", "prod")
	seedCLIEnvDeployment(t, target.Name, "delete-two", "prod")
	seedCLIEnvDeployment(t, target.Name, "shared-app", "prod")
	seedCLIEnvDeployment(t, target.Name, "shared-app", "dev")
	scope, err := deployment.CollectEnvironmentDestroyScope(target.Name, "prod", deployment.EnvironmentDestroyDeleteData)
	if err != nil {
		t.Fatal(err)
	}
	if len(scope.Scoped) != 3 || len(scope.Preserved) != 1 {
		t.Fatalf("expected three scoped and one preserved sibling deployment: scoped=%v preserved=%v", scope.Scoped, scope.Preserved)
	}
	ids := map[string]deployment.DeploymentIdentity{}
	for _, record := range mustListCLIEnvDeployments(t, target.Name) {
		ids[record.Identity.DeploymentID] = record.Identity
	}
	processed := 0
	err = applyAndRemoveEnvironmentIdentity(context.Background(), target.Name, "prod", scope, func(_ context.Context, dep deployment.EnvironmentDestroyDeployment, choice deployment.EnvironmentDestroyDataChoice) error {
		if choice != deployment.EnvironmentDestroyDeleteData {
			t.Fatalf("delete path passed choice %q", choice)
		}
		if processed == 1 {
			return fmt.Errorf("simulated later data cleanup failure")
		}
		processed++
		return deployment.DeleteDeploymentRecord(ids[dep.DeploymentID])
	})
	if err == nil {
		t.Fatal("expected partial delete failure")
	}
	if exists, err := deployment.EnvironmentStateExists(target.Name, "prod"); err != nil || !exists {
		t.Fatalf("environment identity must remain after partial delete: exists=%v err=%v", exists, err)
	}

	// Retry from a fresh scope: the already removed owner is absent, while the
	// failed and shared owners remain. Identity removal follows verified cleanup.
	retryScope, err := deployment.CollectEnvironmentDestroyScope(target.Name, "prod", deployment.EnvironmentDestroyDeleteData)
	if err != nil {
		t.Fatal(err)
	}
	if len(retryScope.Scoped) != 2 || len(retryScope.Preserved) != 1 {
		t.Fatalf("retry should contain two remaining scoped and one preserved sibling owner: scoped=%v preserved=%v", retryScope.Scoped, retryScope.Preserved)
	}
	if err := applyAndRemoveEnvironmentIdentity(context.Background(), target.Name, "prod", retryScope, func(_ context.Context, dep deployment.EnvironmentDestroyDeployment, _ deployment.EnvironmentDestroyDataChoice) error {
		return deployment.DeleteDeploymentRecord(ids[dep.DeploymentID])
	}); err != nil {
		t.Fatalf("delete retry failed: %v", err)
	}
	if exists, err := deployment.EnvironmentStateExists(target.Name, "prod"); err != nil || exists {
		t.Fatalf("identity should be removed after verified delete: exists=%v err=%v", exists, err)
	}
	if present, err := environmentDestroyDeploymentPresent(target.Name, ids[scope.Preserved[0].DeploymentID].DeploymentID); err != nil || !present {
		t.Fatalf("shared deployment must be preserved: present=%v err=%v", present, err)
	}
}

func mustListCLIEnvDeployments(t *testing.T, target string) []deployment.DeploymentRecord {
	t.Helper()
	records, err := deployment.ListDeployments(target)
	if err != nil {
		t.Fatal(err)
	}
	return records
}

// ---- helpers ----

// seedCLIEnvDeployment seeds a deployment record for an app on a target/env
// through the production SaveDeploymentRecord path.
func seedCLIEnvDeployment(t *testing.T, target, application, environment string) string {
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
	return id.DeploymentID
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
