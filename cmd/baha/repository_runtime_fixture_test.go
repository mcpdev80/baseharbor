package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func runRepositoryFixtureApply(ctx context.Context, args []string, out *bytes.Buffer) error {
	return runRepositoryFixtureApplyCall(ctx, args, func(callCtx context.Context) error {
		out.Reset()
		return runWithIO(callCtx, args, out, out)
	})
}

// Like the backup fixture, obtain the actual invocation's typed refusal first.
// Only its current non-destructive plan authorizes one retry; no stale refresh.
func runRepositoryFixtureApplyCall(ctx context.Context, args []string, run func(context.Context) error) error {
	switch strings.Join(args, " ") {
	case "app apply", "--verbose app apply", "app up":
	default:
		return errors.New("unsupported repository apply fixture operation")
	}
	ctx = withProviderChangeConsent(machineNoninteractiveContext(ctx), nil)
	err := run(ctx)
	if err == nil {
		return nil
	}
	var failure *machine.Error
	if !errors.As(err, &failure) || failure == nil || failure.Code != machine.ErrorApprovalRequired ||
		failure.CauseCode != "impact-confirmation-required" {
		return err
	}
	plan, ok := failure.Details.(application.ProviderImpactPlan)
	if !ok || plan.Destructive {
		return err
	}
	ack := application.ProviderChangeConsent{PlanID: plan.PlanID, First: true, Second: plan.ConfirmationsRequired == 2}
	if consentErr := application.ValidateProviderChangeConsent(plan, ack); consentErr != nil {
		return consentErr
	}
	return run(withProviderChangeConsent(ctx, &ack))
}

type repositoryFixtureVolumeIdentity struct {
	Name      string
	CreatedAt string
	Driver    string
	Labels    map[string]string
}

func repositoryFixtureVolume(t *testing.T, ctx context.Context, provider bhruntime.RuntimeProvider, resource bhruntime.ProjectResource) repositoryFixtureVolumeIdentity {
	t.Helper()
	direct, ok := provider.(interface {
		DirectOutput(context.Context, ...string) (string, error)
	})
	if !ok {
		t.Fatal("selected runtime lacks bound native volume inspection")
	}
	raw, err := direct.DirectOutput(ctx, "volume", "inspect", resource.Name)
	if err != nil {
		t.Fatal("inspect retained native volume", err)
	}
	var values []repositoryFixtureVolumeIdentity
	if json.Unmarshal([]byte(raw), &values) != nil || len(values) != 1 ||
		values[0].Name != resource.Name || values[0].CreatedAt == "" || values[0].Driver == "" {
		t.Fatal("retained native volume identity is not verifiable")
	}
	return values[0]
}

// Default destroy retires runtime, retaining data and its canonical recovery
// owner. Verify those physical effects and the unrelated installation separately.
func verifyRepositoryFixtureRetirement(t *testing.T, ctx context.Context, resolved resolvedApplication) {
	t.Helper()
	before, err := deployment.LoadDeploymentRecord(resolved.DeploymentIdentity)
	if err != nil {
		t.Fatal(err)
	}
	files, _, owned, err := nativeBackendInventory(resolved)
	if err != nil {
		t.Fatal("native backend owner inventory", err)
	}
	provider, err := detectRuntimeForTarget(ctx, resolved.Target)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := provider.InspectProjectResources(ctx, files.Project, owned)
	if err != nil {
		t.Fatal(err)
	}
	volumes := map[bhruntime.ProjectResource]repositoryFixtureVolumeIdentity{}
	for _, resource := range actual {
		if resource.Kind == "volume" {
			volumes[resource] = repositoryFixtureVolume(t, ctx, provider, resource)
		}
	}
	if len(volumes) == 0 {
		t.Fatal("SQL fixture must have an actual owned persistent volume before retirement")
	}
	material, err := runtimeMutationOwnerBytes(files.Env, false)
	if err != nil {
		t.Fatal("read protected backend material before retirement", err)
	}
	materialHash := sha256.Sum256(material)
	clear(material)
	withdrawn, plan, err := buildApplicationRuntimeRetirementPlan(ctx, resolved)
	if err != nil {
		t.Fatal(err)
	}
	_, coreFiles, err := openBaoRuntime(ctx)
	if err != nil {
		t.Fatal(err)
	}
	containers, err := provider.ListRuntimeContainers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	core := map[string]string{}
	for _, container := range containers {
		if container.Project == coreFiles.Project && container.Running {
			core[container.ID] = container.Service
		}
	}
	if len(core) == 0 {
		t.Fatal("shared installation containers must be running before application retirement")
	}
	if err := destroySelectiveApplicationWithConsent(ctx); err != nil {
		t.Fatal("retire repository fixture with the current typed plan", err)
	}
	record, err := deployment.LoadDeploymentRecord(resolved.DeploymentIdentity)
	if err != nil || record.Identity != before.Identity || record.Observed.State != "runtime-retired" ||
		record.Observed.Ready || record.Observed.VerifiedAt.IsZero() ||
		record.Applied.GeneratedState["retired_intent"] != string(before.Applied.Intent) ||
		record.Applied.GeneratedState["previous_intent"] != "" {
		t.Fatalf("ordinary destroy lost the verified retained canonical owner: %v", err)
	}
	result, err := applicationDestroyResult(resolved, false)
	if err != nil || !result.OwnerPreserved || result.DataDeleted == nil || *result.DataDeleted {
		t.Fatalf("retirement receipt does not preserve owned data: %v", err)
	}
	if err := verifyApplicationRuntimeRetirement(resolved, plan, observeApplicationProviderFacts(ctx, withdrawn)); err != nil {
		t.Fatal("actual provider runtime retirement is not verified", err)
	}
	remaining, err := provider.InspectProjectResources(ctx, files.Project, owned)
	if err != nil {
		t.Fatal(err)
	}
	for _, resource := range remaining {
		if resource.Kind == "container" {
			t.Fatal("owned application backend container remains after retirement")
		}
	}
	for volume, identity := range volumes {
		if !application.ResourceNamedExists(remaining, volume) ||
			!reflect.DeepEqual(repositoryFixtureVolume(t, ctx, provider, volume), identity) {
			t.Fatal("ordinary retirement removed or replaced an owned persistent volume")
		}
	}
	material, err = runtimeMutationOwnerBytes(files.Env, false)
	if err != nil || sha256.Sum256(material) != materialHash {
		clear(material)
		t.Fatal("ordinary retirement changed protected backend material")
	}
	clear(material)
	containers, err = provider.ListRuntimeContainers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	retainedCore := map[string]string{}
	for _, container := range containers {
		if container.Project == coreFiles.Project && container.Running {
			retainedCore[container.ID] = container.Service
		}
	}
	if !reflect.DeepEqual(core, retainedCore) {
		t.Fatal("application retirement changed unrelated shared installation containers")
	}
	t.Log("ordinary retirement verified: owned runtime absent, canonical identity/data/material retained, unrelated shared installation unchanged")
}

func TestRepositoryFixtureApplyUsesActualFreshAuthorityAndDoesNotRefreshStalePlan(t *testing.T) {
	resolved := corelessResolvedFixture(t)
	resolved.Manifest.Services.SQL = true
	ambient := &application.ProviderChangeConsent{PlanID: "ambient", First: true, Second: true, DeleteData: true}
	for _, drift := range []bool{false, true} {
		calls := 0
		err := runRepositoryFixtureApplyCall(withProviderChangeConsent(t.Context(), ambient), []string{"app", "apply"}, func(ctx context.Context) error {
			calls++
			ack, err := invocationProviderChangeConsent(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if calls == 1 && ack != nil {
				t.Fatal("ambient approval reached the first actual invocation")
			}
			if calls == 2 {
				if ack == nil || !ack.First || !ack.Second || ack.DeleteData || ack.PlanID == "ambient" {
					t.Fatal("fixture did not bind its acknowledgement to the actual two-stage plan")
				}
				if drift {
					resolved.Manifest.Services.Cache = true
				}
			}
			return authorizeApplicationProviderChanges(ctx, resolved, nil, io.Discard)
		})
		if calls != 2 {
			t.Fatalf("expected one unapproved attempt and at most one retry, got %d", calls)
		}
		if drift {
			var failure *machine.Error
			if !errors.As(err, &failure) || failure.Code != machine.ErrorConflict {
				t.Fatalf("changed operation plan was automatically refreshed: %v", err)
			}
		} else if err != nil {
			t.Fatalf("fresh actual authority rejected matching explicit acknowledgement: %v", err)
		}
	}
}

func TestRepositoryFixtureApplyRejectsUnknownInvalidDestructiveAndWrongOperations(t *testing.T) {
	plan := application.ProviderImpactPlan{Version: application.ProviderImpactVersion, PlanID: "actual", ConfirmationsRequired: 2}
	unknown := errors.New("unrelated native failure")
	invalid := plan
	invalid.PlanID = ""
	destructive := plan
	destructive.Destructive = true
	var nilFailure *machine.Error
	for _, original := range []error{
		unknown,
		nilFailure,
		providerImpactConsentError(plan, &application.ProviderChangeError{Code: "stale-impact-consent"}),
		&machine.Error{Code: machine.ErrorApprovalRequired, CauseCode: "impact-confirmation-required", Details: map[string]string{"plan_id": "untyped"}},
		&machine.Error{Code: machine.ErrorApprovalRequired, CauseCode: "impact-confirmation-required", Details: applicationDestructionPlan{}},
		providerImpactConsentError(invalid, &application.ProviderChangeError{Code: "impact-confirmation-required"}),
		providerImpactConsentError(destructive, &application.ProviderChangeError{Code: "impact-confirmation-required"}),
	} {
		calls := 0
		err := runRepositoryFixtureApplyCall(t.Context(), []string{"app", "apply"}, func(context.Context) error {
			calls++
			return original
		})
		if err == nil || calls != 1 {
			t.Fatalf("invalid/stale/unrelated failure accepted or retried: calls=%d err=%v", calls, err)
		}
		if original == unknown && !errors.Is(err, unknown) {
			t.Fatal("unrelated native failure was replaced")
		}
	}
	for _, command := range [][]string{{"app", "destroy", "--yes"}, {"app", "restore"}, {"app", "apply", "--delete-data"}} {
		err := runRepositoryFixtureApplyCall(t.Context(), command, func(context.Context) error {
			t.Fatal("different operation reached apply fixture acknowledgement")
			return nil
		})
		if err == nil {
			t.Fatal("unsupported fixture mutation was accepted")
		}
	}
}

func TestRepositoryFixtureMandatoryNativeScopeIsPreserved(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "docker-runtime-acceptance.yml"))
	if err != nil {
		t.Fatal(err)
	}
	data = []byte(strings.Split(string(data), "\n  provider-ha:")[0])
	for _, name := range []string{
		"TestRepositoryWorkflowRealLifecycle", "TestRepositoryComposeWorkloadUsesBaseHarborBackendsInCI",
		"TestRepositoryComposeWorkloadOnlyLifecycleInCI", "TestRepositoryBuildWorkloadRebuildsSourceChangesInCI",
		"TestMultiInstanceComposeLifecycleInCI", "TestExistingControlPlaneRestartRequiresAndUsesRecoveryFile",
		"TestManagedLokiIngestsRealComposeWorkloadLogs", "TestManagedTempoReceivesVerificationTraceThroughCollector",
	} {
		if strings.Count(string(data), name) != 2 {
			t.Fatalf("native workflow selection and required executed receipt changed for %s", name)
		}
	}
}
