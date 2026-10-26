package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"go.yaml.in/yaml/v3"

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
	retryErr := run(withProviderChangeConsent(ctx, &ack))
	var rejected *machine.Error
	if !errors.As(retryErr, &rejected) || rejected == nil || rejected.CauseCode != "stale-impact-consent" {
		return retryErr
	}
	// Identifiers locate the refused boundary without rendering either plan's
	// materials, paths, credentials, or the rejected observation itself.
	approvedID := repositoryFixturePlanID(plan.PlanID)
	rejectedID, boundary, changedFields := "unavailable", "unavailable", "unavailable"
	if rejectedPlan, ok := rejected.Details.(application.ProviderImpactPlan); ok {
		rejectedID = repositoryFixturePlanID(rejectedPlan.PlanID)
		changedFields = repositoryFixtureChangedPublicFields(plan, rejectedPlan)
		if approvedID != "unavailable" && rejectedID != "unavailable" {
			boundary = "across-invocation"
			if approvedID == rejectedID {
				boundary = "within-invocation"
			}
		}
	}
	return fmt.Errorf("repository fixture stale consent: acknowledged_plan_id=%s rejected_plan_id=%s boundary=%s changed_public_fields=%s: %w", approvedID, rejectedID, boundary, changedFields, retryErr)
}

// Report only fixed public field names, never values or material references.
// Revision-only drift remains distinguishable from changed consequences.
func repositoryFixtureChangedPublicFields(before, after application.ProviderImpactPlan) string {
	left, right := reflect.ValueOf(before), reflect.ValueOf(after)
	names := []string{}
	for i := 0; i < left.NumField(); i++ {
		if !reflect.DeepEqual(left.Field(i).Interface(), right.Field(i).Interface()) {
			names = append(names, strings.Split(left.Type().Field(i).Tag.Get("json"), ",")[0])
		}
	}
	if len(names) == 0 {
		return "none"
	}
	return strings.Join(names, ",")
}

func repositoryFixturePlanID(id string) string {
	data, err := hex.DecodeString(id)
	if err != nil || len(data) != sha256.Size {
		return "unavailable"
	}
	return id
}

// Repository identity metadata is allowed; runtime authority and confidential
// material remain exclusively under the canonical Target/deployment UUID.
func verifyRepositoryFixtureState(resolved resolvedApplication, authored []byte) error {
	if err := resolved.DeploymentIdentity.Validate(); err != nil {
		return err
	}
	if resolved.DeploymentIdentity.ApplicationID != resolved.Manifest.ApplicationID || resolved.DeploymentIdentity.Target != resolved.Target.Name {
		return errors.New("deployment identity differs from selected application/Target")
	}
	targetRoot, err := deployment.TargetStateRoot(resolved.Target.Name)
	if err != nil {
		return err
	}
	root, err := deployment.DeploymentRoot(resolved.DeploymentIdentity)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(resolved.RepositoryRoot, root)
	if err != nil || rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))) {
		return errors.New("canonical runtime state leaked into repository")
	}
	if resolved.TargetStateRoot != targetRoot || resolved.DeploymentStateRoot != root || resolved.Store.Root != filepath.Join(root, "state") || resolved.Store.Namespace != resolved.Target.Name {
		return errors.New("runtime state is not bound to canonical Target/deployment UUID")
	}
	record, err := deployment.LoadDeploymentRecord(resolved.DeploymentIdentity)
	if err != nil {
		return err
	}
	if record.Identity != resolved.DeploymentIdentity || record.Source.Repository != resolved.RepositoryRoot || record.Source.Manifest != resolved.ManifestPath || !record.Observed.Ready || record.Observed.VerifiedAt.IsZero() {
		return fmt.Errorf("canonical deployment does not verify the applied repository owner: identity_match=%t repository_match=%t manifest_match=%t ready=%t verified=%t", record.Identity == resolved.DeploymentIdentity, record.Source.Repository == resolved.RepositoryRoot, record.Source.Manifest == resolved.ManifestPath, record.Observed.Ready, !record.Observed.VerifiedAt.IsZero())
	}
	actual, err := os.ReadFile(resolved.ManifestPath)
	if err != nil {
		return err
	}
	if !bytes.Equal(actual, authored) {
		return errors.New("apply rewrote authored repository intent")
	}
	metadata := filepath.Join(resolved.RepositoryRoot, ".baseharbor")
	owner := application.Store{Root: filepath.Join(metadata, "apps")}
	identity, path, err := owner.Load(resolved.Manifest.Name)
	if err != nil {
		return err
	}
	if identity.ApplicationID != resolved.Manifest.ApplicationID || identity.Name != resolved.Manifest.Name || identity.Environment != resolved.Manifest.Environment {
		return errors.New("repository identity metadata differs from canonical owner")
	}
	ignore := filepath.Join(metadata, ".gitignore")
	allowed := map[string]bool{metadata: true, owner.Root: true, filepath.Dir(path): true, path: true, ignore: true}
	return filepath.WalkDir(metadata, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !allowed[path] || entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("unexpected repository-local runtime/confidential state: %s", path)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if path == ignore {
			// Store.Sync's public Git safety rule is the sole 0644 exception.
			// Its parent, identity material and native owner must remain protected.
			if !info.Mode().IsRegular() || info.Mode().Perm() != 0644 || !bhruntime.ProtectedStateOwner(path, repositoryFixtureIgnoreOwnerInfo{info}) {
				return errors.New("repository Git safety rule has an unexpected mode or native owner")
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if string(data) != "*\n!.gitignore\n" {
				return errors.New("repository Git safety rule differs from Store.Sync contract")
			}
		} else if !bhruntime.ProtectedStateOwner(path, info) || (!entry.IsDir() && (!info.Mode().IsRegular() || info.Mode().Perm() != 0600)) {
			return errors.New("repository identity metadata is not protected under its native owner")
		}
		return nil
	})
}

// ProtectedStateOwner verifies current native UID and single-link authority.
// Exact 0644 is checked separately above; only its public-read bits are excluded
// from that private-file predicate for this non-confidential Git safety rule.
type repositoryFixtureIgnoreOwnerInfo struct{ os.FileInfo }

func (info repositoryFixtureIgnoreOwnerInfo) Mode() os.FileMode { return info.FileInfo.Mode() &^ 0044 }

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

func TestRepositoryFixtureStateAllowsOnlyIdentityMetadata(t *testing.T) {
	configureTestTarget(t)
	root := t.TempDir()
	t.Chdir(root)
	ctx := t.Context()
	// Exercise the actual authoring command, including InitializeIdentity and
	// Store.Sync/ensureStateGitIgnore, instead of a reduced resolver-only setup.
	if err := runWithIO(ctx, []string{"app", "init", "plain", "--sql", "--cache"}, io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	selection, err := application.ResolveRepositoryEnvironment(root, "")
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := resolveApplication(ctx, application.Store{}, nil, "apply")
	if err != nil {
		t.Fatal(err)
	}
	authored, err := os.ReadFile(resolved.ManifestPath)
	if err != nil {
		t.Fatal(err)
	}
	// Use the production observation writer: a successful apply of a sleeping
	// workload is running-unverified, and must never satisfy this verifier.
	for _, ready := range []bool{false, true} {
		state := "running"
		if ready {
			state = "ready"
		}
		if err := recordAppliedDeploymentObservation(ctx, resolved, application.RuntimeFiles{}, state, ready); err != nil {
			t.Fatal(err)
		}
		err := verifyRepositoryFixtureState(resolved, authored)
		if (err == nil) != ready {
			t.Fatalf("canonical readiness verification ready=%t: %v", ready, err)
		}
	}
	record, err := deployment.LoadDeploymentRecord(resolved.DeploymentIdentity)
	if err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*deployment.DeploymentRecord){
		func(r *deployment.DeploymentRecord) { r.Source.Repository = root + "-foreign" },
		func(r *deployment.DeploymentRecord) { r.Source.Manifest = filepath.Join(root, "foreign.yaml") },
		func(r *deployment.DeploymentRecord) { r.Observed.VerifiedAt = time.Time{} },
	} {
		changed := record
		change(&changed)
		if err := deployment.SaveDeploymentRecord(changed); err != nil {
			t.Fatal(err)
		}
		if err := verifyRepositoryFixtureState(resolved, authored); err == nil {
			t.Fatal("foreign/unverified applied repository owner accepted")
		}
		if err := deployment.SaveDeploymentRecord(record); err != nil {
			t.Fatal(err)
		}
	}
	ignore := filepath.Join(root, ".baseharbor", ".gitignore")
	for _, kind := range []string{"altered-ignore", "linked-ignore", "writable-ignore", "public-parent"} {
		t.Run(kind, func(t *testing.T) {
			switch kind {
			case "altered-ignore":
				if err := os.WriteFile(ignore, []byte("!credentials.env\n"), 0644); err != nil {
					t.Fatal(err)
				}
			case "linked-ignore":
				target := filepath.Join(t.TempDir(), "ignore")
				if err := os.WriteFile(target, []byte("*\n!.gitignore\n"), 0644); err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(ignore); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, ignore); err != nil {
					t.Fatal(err)
				}
			case "writable-ignore":
				if err := os.Chmod(ignore, 0666); err != nil {
					t.Fatal(err)
				}
			case "public-parent":
				if err := os.Chmod(filepath.Dir(ignore), 0755); err != nil {
					t.Fatal(err)
				}
			}
			if err := verifyRepositoryFixtureState(resolved, authored); err == nil {
				t.Fatal("invalid Git safety rule or unprotected parent accepted")
			}
			if kind == "public-parent" {
				if err := os.Chmod(filepath.Dir(ignore), 0700); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := os.Remove(ignore); err != nil {
					t.Fatal(err)
				}
				// Reuse the production owner to restore the exact legitimate metadata.
				if _, err := (application.Store{Root: filepath.Join(root, ".baseharbor", "apps")}).Sync(resolved.Manifest); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
	for _, path := range []string{"credentials.env", "deployment.json", "runtime/compose.yaml"} {
		t.Run(path, func(t *testing.T) {
			path = filepath.Join(selection.RepositoryRoot, ".baseharbor", path)
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte("unexpected material"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := verifyRepositoryFixtureState(resolved, authored); err == nil {
				t.Fatal("repository runtime material accepted")
			}
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if filepath.Base(filepath.Dir(path)) == "runtime" {
				if err := os.Remove(filepath.Dir(path)); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
	for _, change := range []func(*resolvedApplication){
		func(r *resolvedApplication) { r.TargetStateRoot = selection.RepositoryRoot },
		func(r *resolvedApplication) { r.DeploymentStateRoot = selection.RepositoryRoot },
		func(r *resolvedApplication) {
			r.Store.Root = filepath.Join(selection.RepositoryRoot, ".baseharbor", "apps")
		},
		func(r *resolvedApplication) { r.DeploymentIdentity.ApplicationID = application.MustNewApplicationID() },
	} {
		changed := resolved
		change(&changed)
		if err := verifyRepositoryFixtureState(changed, authored); err == nil {
			t.Fatal("foreign/noncanonical runtime authority accepted")
		}
	}
	if err := verifyRepositoryFixtureState(resolved, append(authored, ' ')); err == nil {
		t.Fatal("rewritten authored source accepted")
	}
}

func TestRepositoryFixtureApplyReResolvesUnallocatedSQLCacheProposal(t *testing.T) {
	configureTestTarget(t)
	selection := minimalRepositorySelection(t)
	t.Chdir(selection.RepositoryRoot)
	source := "version: 1\napp:\n  name: fresh\n  environment: dev\nservices:\n  sql: true\n  cache: true\nworkload:\n  components:\n    - api\n"
	if err := os.WriteFile(selection.ManifestPath, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	var identities []string
	err := runRepositoryFixtureApplyCall(t.Context(), []string{"app", "apply"}, func(ctx context.Context) error {
		resolved, err := resolveApplication(ctx, application.Store{}, nil, "apply")
		if err != nil {
			return err
		}
		identities = append(identities, resolved.DeploymentIdentity.DeploymentID)
		return authorizeApplicationProviderChanges(ctx, resolved, nil, io.Discard)
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(identities) != 2 || identities[0] == identities[1] {
		t.Fatalf("fixture did not exercise independent unallocated proposals: %v", identities)
	}
}

func TestRepositoryFixtureStaleDiagnosticPreservesCauseAndOnlyHashIdentifiers(t *testing.T) {
	approvedHash, changedHash := sha256.Sum256([]byte("approved")), sha256.Sum256([]byte("changed"))
	approvedID, changedID := hex.EncodeToString(approvedHash[:]), hex.EncodeToString(changedHash[:])
	baseline := application.ProviderImpactPlan{Version: application.ProviderImpactVersion, PlanID: approvedID, Revision: "first"}
	changed := baseline
	changed.Revision = "second"
	changed.PlanID = changedID
	if fields := repositoryFixtureChangedPublicFields(baseline, changed); fields != "revision,plan_id" {
		t.Fatalf("revision-only evidence mislabeled: %s", fields)
	}
	if fields := repositoryFixtureChangedPublicFields(baseline, baseline); fields != "none" {
		t.Fatalf("unchanged public plan mislabeled: %s", fields)
	}

	for _, tc := range []struct{ name, rejectedID, boundary string }{
		{"across", changedID, "across-invocation"},
		{"within", approvedID, "within-invocation"},
		{"invalid", "private-material-never-render", "unavailable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plan := application.ProviderImpactPlan{Version: application.ProviderImpactVersion, PlanID: approvedID, ConfirmationsRequired: 2}
			rejected := plan
			rejected.PlanID = tc.rejectedID
			rejected.Impacts = []application.ProviderImpact{{Notices: []string{"secret-material-never-render"}}}
			cause := providerImpactConsentError(rejected, &application.ProviderChangeError{Code: "stale-impact-consent"})
			calls := 0
			err := runRepositoryFixtureApplyCall(t.Context(), []string{"app", "apply"}, func(context.Context) error {
				calls++
				if calls == 1 {
					return providerImpactConsentError(plan, &application.ProviderChangeError{Code: "impact-confirmation-required"})
				}
				return cause
			})
			var failure *machine.Error
			if calls != 2 || !errors.Is(err, cause) || !errors.As(err, &failure) || failure.CauseCode != "stale-impact-consent" {
				t.Fatalf("lost cause or changed retry bound: calls=%d err=%v", calls, err)
			}
			for _, wanted := range []string{"acknowledged_plan_id=" + approvedID, "boundary=" + tc.boundary, "changed_public_fields="} {
				if !strings.Contains(err.Error(), wanted) {
					t.Fatalf("missing identifier diagnostic %s", wanted)
				}
			}
			if tc.name != "invalid" && !strings.Contains(err.Error(), "rejected_plan_id="+tc.rejectedID) {
				t.Fatal("rejected hash identifier missing")
			}
			for _, forbidden := range []string{"private-material-never-render", "secret-material-never-render"} {
				if strings.Contains(err.Error(), forbidden) {
					t.Fatal("plan material leaked through diagnostic")
				}
			}
		})
	}
}

// Exercise the exact authored native fixture, including Compose's escaped
// container variables. These shell regressions prove the probe contract only;
// the separate native eight-test gate proves real runtime/backend readiness.
func TestRepositoryBackendProbeFixtureChecksAuthenticatedReadiness(t *testing.T) {
	var model struct {
		Services map[string]struct {
			Healthcheck struct {
				Test     []string
				Interval string
				Timeout  string
				Retries  int
			}
		}
	}
	if err := yaml.Unmarshal([]byte(repositoryBackendProbeCompose), &model); err != nil {
		t.Fatal(err)
	}
	for _, probe := range []struct{ service, binary, expected, arguments string }{
		{"pg-probe", "psql", "1", `[ "$#" = 3 ] && [ "$1" = "$DATABASE_URL" ] && [ "$2" = -tAc ] && [ "$3" = 'SELECT 1' ]`},
		{"valkey-probe", "valkey-cli", "PONG", `[ "$#" = 5 ] && [ "$1" = -u ] && [ "$2" = "$REDIS_URL" ] && [ "$3" = --cacert ] && [ "$4" = "$REDIS_CA_FILE" ] && [ "$5" = ping ]`},
	} {
		t.Run(probe.service, func(t *testing.T) {
			h := model.Services[probe.service].Healthcheck
			if len(h.Test) != 2 || h.Test[0] != "CMD-SHELL" || h.Interval != "1s" || h.Timeout != "5s" || h.Retries != 30 {
				t.Fatalf("missing bounded container readiness probe: %#v", h)
			}
			bin := t.TempDir()
			stub := "#!/bin/sh\n" + probe.arguments + " || exit 99\nprintf '%s\\n' \"$PROBE_OUTPUT\"\nexit \"$PROBE_EXIT\"\n"
			if err := os.WriteFile(filepath.Join(bin, probe.binary), []byte(stub), 0700); err != nil {
				t.Fatal(err)
			}
			for _, result := range []struct {
				name, output, exit string
				pass               bool
			}{
				{"authenticated-success", probe.expected, "0", true},
				{"wrong-response", "unexpected", "0", false},
				{"empty-response", "", "0", false},
				{"command-failure", probe.expected, "1", false},
			} {
				t.Run(result.name, func(t *testing.T) {
					command := exec.Command("sh", "-ec", strings.ReplaceAll(h.Test[1], "$$", "$"))
					command.Env = []string{"PATH=" + bin, "DATABASE_URL=postgresql://fixture/database", "REDIS_URL=rediss://fixture", "REDIS_CA_FILE=/fixture/ca.pem", "PROBE_OUTPUT=" + result.output, "PROBE_EXIT=" + result.exit}
					if err := command.Run(); (err == nil) != result.pass {
						t.Fatalf("probe success=%t, want %t: %v", err == nil, result.pass, err)
					}
				})
			}
		})
	}
}

// The existing selected cli-guided-tests lane requires this bounded fixture
// suite. Native runtime execution remains separately required by its workflow.
func TestGuidedRepositoryRuntimeFixtureRegressionSuite(t *testing.T) {
	for _, test := range []struct {
		name string
		run  func(*testing.T)
	}{
		{"authenticated-native-backend-healthchecks", TestRepositoryBackendProbeFixtureChecksAuthenticatedReadiness},
		{"production-workload-health-status", TestBuildWorkloadServiceStatuses},
		{"fresh-authority-and-stale-rejection", TestRepositoryFixtureApplyUsesActualFreshAuthorityAndDoesNotRefreshStalePlan},
		{"invalid-and-wrong-operation-rejection", TestRepositoryFixtureApplyRejectsUnknownInvalidDestructiveAndWrongOperations},
		{"mandatory-native-eight-test-scope", TestRepositoryFixtureMandatoryNativeScopeIsPreserved},
		{"canonical-state-and-repository-material-isolation", TestRepositoryFixtureStateAllowsOnlyIdentityMetadata},
		{"independent-unallocated-sql-cache-proposals", TestRepositoryFixtureApplyReResolvesUnallocatedSQLCacheProposal},
		{"identifier-only-stale-diagnostic", TestRepositoryFixtureStaleDiagnosticPreservesCauseAndOnlyHashIdentifiers},
		{"production-retirement-current-scope-and-data-consent", TestApplicationDestructionRequiresCurrentOuterScopeAndSeparateDataConsent},
		{"production-retirement-stale-publication-rejection", TestApplicationDestructionConsentInvalidatedByRetainedPublicationChange},
		{"production-concrete-consent-without-writes", TestApplicationImpactAuthorizationRequiresConcreteConsentWithoutWrites},
		{"production-secrets-stable-consent", TestSelectiveSecretsPlanHasStableConsentWithoutAuthorityChanges},
		{"production-retirement-owner-material", TestProviderOnlyRetirementDoesNotInventRuntimeMaterial},
	} {
		t.Run(test.name, test.run)
	}
}
