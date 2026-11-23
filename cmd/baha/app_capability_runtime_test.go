package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/applicationsecret"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/runtimebroker"
)

// Run after the fresh no-Core gate, in a separate process on the same isolated
// host. This explicitly installs management Core, then reuses that authority.
func TestApplicationCapabilityLifecycleExistingCore(t *testing.T) {
	if os.Getenv("BASEHARBOR_CORELESS_ACCEPTANCE") != "1" {
		t.Skip("requires isolated exact-SHA runtime acceptance")
	}
	ctx, cancel := context.WithTimeout(machineNoninteractiveContext(context.Background()), 25*time.Minute)
	defer cancel()
	ctx = testOperatorContext(ctx)
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	target := configureTestTargetEmpty(t)
	if os.Getenv("BASEHARBOR_TEST_RUNTIME") == "podman" {
		cfg, err := deployment.LoadConfig()
		if err != nil {
			t.Fatal(err)
		}
		definition := cfg.Targets[target.Name]
		definition.Runtime.Provider = "podman"
		cfg.Targets[target.Name] = definition
		if err := cfg.Save(); err != nil {
			t.Fatal(err)
		}
		target, err = cfg.ResolveTarget("", "")
		if err != nil {
			t.Fatal(err)
		}
	}
	root := t.TempDir()
	t.Chdir(root)
	runtime, err := detectRuntimeForTarget(ctx, target)
	if err != nil {
		t.Fatal(err)
	}
	assertCorelessHost(t, ctx, runtime, target)
	var out bytes.Buffer
	first, err := installCore(ctx, strings.NewReader(""), &out, runtimeUpOptions{Yes: true, ControlPlaneOnly: true, MachineRole: coreinstallation.Development, RecoveryFile: filepath.Join(t.TempDir(), "recovery.json")})
	if err != nil || !first.Ready {
		logCoreBootstrapFailure(t, runtime, target.Name, target.RuntimeProvider)
		t.Fatalf("explicit management install: %v\n%s", err, out.String())
	}
	t.Logf("explicit Core installation_id=%s target=%s", first.ID, target.Name)
	// The managed Core path requires an explicit operator-auth policy even in
	// this isolated acceptance runtime. Bind the fixture principal to that same
	// Target/environment so app doctor exercises the configured authenticated
	// path instead of failing on a missing policy.
	if err := deployment.SetOperatorAuthForTargetEnvironment(target.Name, "dev", deployment.OperatorAuthEnvironmentConfig{
		Provider: "external-oidc", Issuer: "https://issuer.test", ClientID: "cli-test",
	}); err != nil {
		t.Fatalf("configure isolated operator-auth boundary: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cleanupCancel()
		if err := approvedNativeFixtureCleanup(t, testOperatorContext(machineNoninteractiveContext(cleanupCtx)), &bytes.Buffer{}); err != nil {
			t.Errorf("isolated Core cleanup: %v", err)
		}
	})
	parent := t
	var sqlFirst resolvedApplication
	for _, kind := range []string{"sql", "sql-second", "secrets", "identity", "https"} {
		if !t.Run(kind, func(t *testing.T) {
			appRoot := parent.TempDir()
			t.Chdir(appRoot)
			m := application.WithWorkloadComponents(application.New("capability-"+kind, "dev", false, false, false), "api")
			switch kind {
			case "sql", "sql-second":
				m.Services.SQL = true
			case "secrets":
				m = application.WithGeneratedSecret(m, "API_TOKEN", "hex", 32)
			case "identity":
				m = application.WithIdentity(m)
			case "https":
				m = application.WithHTTPExposure(m, "web", "api", 8080, "https")
			}
			if err := writeInitializedAuthoredManifest(t, "baseharbor.yaml", m, 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile("compose.yaml", []byte(corelessHTTPWorkload), 0600); err != nil {
				t.Fatal(err)
			}
			parent.Cleanup(func() {
				cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 2*time.Minute)
				defer cleanupCancel()
				previous, _ := os.Getwd()
				if err := os.Chdir(appRoot); err != nil {
					parent.Error(err)
					return
				}
				defer os.Chdir(previous)
				resetCoreBootstrapFixtureApplication(parent, testOperatorContext(machineNoninteractiveContext(cleanupCtx)))
			})
			if kind == "https" {
				out.Reset()
				if err := runWithIO(ctx, []string{"app", "init", "--input", "tls_mode=local", "--yes"}, &out, &out); err != nil {
					t.Fatalf("HTTPS inputs: %v\n%s", err, out.String())
				}
			}
			var secretsBaseline map[string]bhruntime.RuntimeContainer
			if kind == "secrets" {
				// Include the running Core and previously qualified SQL sibling
				// applications. The Secrets lifecycle must leave every one intact.
				containers, err := runtime.ListRuntimeContainers(ctx)
				if err != nil {
					t.Fatalf("inventory Core/provider baseline before Secrets workload: %v", err)
				}
				secretsBaseline = runtimeContainerSnapshotSet(containers)
			}
			for _, command := range [][]string{{"app", "apply", "--skip-memory-preflight"}, {"app", "status", "--json"}, {"app", "doctor", "--json"}} {
				out.Reset()
				commandCtx := ctx
				if len(command) > 1 && command[1] == "apply" {
					commandCtx = coreBootstrapFixtureConsent(t, ctx)
				}
				if err := runWithIO(commandCtx, command, &out, &out); err != nil {
					t.Fatalf("%v: %v\n%s", command, err, out.String())
				}
			}
			resolved, err := resolveApplication(ctx, application.Store{}, nil, "status")
			if err != nil {
				t.Fatal(err)
			}
			status, _, err := collectResolvedApplicationStatus(ctx, resolved)
			if err != nil || !status.Ready {
				t.Fatalf("capability verification: %+v %v", status, err)
			}
			coreRoot, err := targetRuntimeStateRoot(target)
			if err != nil {
				t.Fatal(err)
			}
			current, err := coreinstallation.Load(coreRoot)
			if err != nil || current.ID != first.ID {
				t.Fatalf("existing Core was replaced: %v", err)
			}
			if kind == "secrets" {
				qualifyCoreBackedSecretsLifecycle(t, ctx, runtime, resolved, current, secretsBaseline)
			}
			if kind == "sql" {
				sqlFirst = resolved
			}
			if kind == "sql-second" {
				store := capability.RegistryStore{Path: filepath.Join(resolved.TargetStateRoot, "provider-registry.json")}
				registry, err := store.Load()
				if err != nil {
					t.Fatal(err)
				}
				firstID, secondID := "", ""
				for _, binding := range registry.Bindings {
					if binding.Resource.Kind == capability.SQL {
						if binding.ApplicationID == sqlFirst.Manifest.ApplicationID {
							firstID = binding.ProviderInstanceID
						}
						if binding.ApplicationID == m.ApplicationID {
							secondID = binding.ProviderInstanceID
						}
					}
				}
				if firstID == "" || firstID != secondID {
					t.Fatalf("two apps did not reuse native Target binding: %s %s", firstID, secondID)
				}
				reduced := sqlFirst.Manifest
				reduced.Services.SQL = false
				if err := writeInitializedAuthoredManifest(t, sqlFirst.ManifestPath, reduced, 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Chdir(sqlFirst.RepositoryRoot); err != nil {
					t.Fatal(err)
				}
				out.Reset()
				if err := runWithIO(coreBootstrapFixtureConsent(t, ctx), []string{"app", "apply", "--skip-memory-preflight"}, &out, &out); err != nil {
					t.Fatalf("capability removal: %v\n%s", err, out.String())
				}
				if err := os.Chdir(appRoot); err != nil {
					t.Fatal(err)
				}
				status, _, err = collectResolvedApplicationStatus(ctx, resolved)
				if err != nil || !status.Ready {
					t.Fatalf("capability removal broke another app: %+v %v", status, err)
				}
				registry, err = store.Load()
				if err != nil {
					t.Fatal(err)
				}
				surviving := false
				for _, binding := range registry.Bindings {
					if binding.ApplicationID == m.ApplicationID && binding.ProviderInstanceID == secondID {
						surviving = true
					}
				}
				if !surviving {
					t.Fatal("capability removal erased other consumer binding")
				}
				t.Logf("two apps reused %s; removal retained other app live", secondID)
				// Reattach the retained scope through the existing lifecycle so
				// explicit app destroy can reclaim this isolated test's resources.
				// Capability removal itself deliberately does not perform GC.
				if err := writeInitializedAuthoredManifest(t, sqlFirst.ManifestPath, sqlFirst.Manifest, 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Chdir(sqlFirst.RepositoryRoot); err != nil {
					t.Fatal(err)
				}
				out.Reset()
				if err := runWithIO(coreBootstrapFixtureConsent(t, ctx), []string{"app", "apply", "--skip-memory-preflight"}, &out, &out); err != nil {
					t.Fatalf("reattach retained test scope: %v\n%s", err, out.String())
				}
				if err := os.Chdir(appRoot); err != nil {
					t.Fatal(err)
				}
			}
		}) {
			return
		}
	}
}

// This lifecycle reuses the installation already created by this acceptance.
// Core owns OpenBao independently, so app capability removal and stop must
// preserve the complete pre-existing Core/provider runtime baseline.
func qualifyCoreBackedSecretsLifecycle(t *testing.T, ctx context.Context, runtime bhruntime.RuntimeProvider, initial resolvedApplication, initialOwner coreinstallation.State, baseline map[string]bhruntime.RuntimeContainer) {
	t.Helper()
	root, err := targetRuntimeStateRoot(initial.Target)
	if err != nil {
		t.Fatal(err)
	}
	inventory, err := bhruntime.ExistingControlPlaneRuntimeInventory(root, bhruntime.SharedProjectName(initial.Target.Name))
	if err != nil {
		t.Fatalf("inspect existing Core provider inventory: %v", err)
	}
	actual, err := runtime.InspectProjectResources(ctx, inventory.Files.Project, inventory.Resources)
	if err != nil {
		t.Fatalf("inspect existing Core provider resources: %v", err)
	}
	var retainedVolumes []bhruntime.ProjectResource
	for _, resource := range actual {
		if resource.Kind == "volume" {
			retainedVolumes = append(retainedVolumes, resource)
		}
	}
	if len(retainedVolumes) == 0 {
		t.Fatal("existing Core Secrets provider has no actual owned data volume")
	}
	data, err := os.ReadFile(inventory.Files.Env)
	if err != nil {
		t.Fatalf("read existing Core provider configuration: %v", err)
	}
	envDigest := sha256.Sum256(data)
	secretDigest := coreBackedApplicationSecretDigest(t, ctx, runtime, inventory, initial)
	assertCoreBackedSecretsState(t, ctx, runtime, inventory, initial, initialOwner, baseline, retainedVolumes, envDigest, true, true, true)

	manifestPath := initial.ManifestPath
	for _, enabled := range []bool{false, true} {
		updated := initial.Manifest
		updated.Services.Secrets = enabled
		if !enabled {
			updated.Secrets = application.SecretRequirements{}
		}
		if err := writeInitializedAuthoredManifest(t, manifestPath, updated, 0600); err != nil {
			t.Fatalf("write Core-backed Secrets intent enabled=%t: %v", enabled, err)
		}
		var out bytes.Buffer
		if err := runWithIO(coreBootstrapFixtureConsent(t, ctx), []string{"app", "apply", "--skip-memory-preflight"}, &out, &out); err != nil {
			t.Fatalf("Core-backed Secrets apply enabled=%t: %v\n%s", enabled, err, out.String())
		}
		current, err := resolveApplication(ctx, application.Store{}, nil, "status")
		if err != nil {
			t.Fatalf("resolve Core-backed Secrets phase enabled=%t: %v", enabled, err)
		}
		status, _, err := collectResolvedApplicationStatus(ctx, current)
		if err != nil || !status.Ready || current.Manifest.ApplicationID != initial.Manifest.ApplicationID || current.DeploymentIdentity != initial.DeploymentIdentity {
			t.Fatalf("Core-backed Secrets phase enabled=%t lost app readiness/identity: %+v %v", enabled, status, err)
		}
		assertCoreBackedSecretsState(t, ctx, runtime, inventory, current, initialOwner, baseline, retainedVolumes, envDigest, enabled, enabled, true)
		if enabled && coreBackedApplicationSecretDigest(t, ctx, runtime, inventory, current) != secretDigest {
			t.Fatal("Secrets re-add lost or regenerated the retained application secret")
		}
	}

	// app down removes the application workload and broker, preserves application
	// data, and records a verified runtime-retired owner. Core's provider remains
	// protected by its independent installation owner.
	if err := stopSelectiveApplicationWithConsent(ctx, &bytes.Buffer{}); err != nil {
		t.Fatalf("retire Core-backed Secrets application runtime: %v", err)
	}
	retired, err := resolveApplication(ctx, application.Store{}, nil, "status")
	if err != nil {
		t.Fatalf("resolve retired Core-backed Secrets owner: %v", err)
	}
	status, _, err := collectResolvedApplicationStatus(ctx, retired)
	if err != nil || status.State != "stopped" || status.Ready || retired.Manifest.ApplicationID != initial.Manifest.ApplicationID || retired.DeploymentIdentity != initial.DeploymentIdentity {
		t.Fatalf("Core-backed Secrets retirement lost canonical stopped owner: %+v %v", status, err)
	}
	record, err := deployment.LoadDeploymentRecord(retired.DeploymentIdentity)
	if err != nil || record.Observed.State != "runtime-retired" || record.Observed.Ready || record.Applied.GeneratedState["previous_intent"] != "" || record.Applied.GeneratedState["retired_intent"] == "" {
		t.Fatalf("Core-backed Secrets retirement receipt/owner is invalid: %+v %v", record, err)
	}
	result, err := applicationDestroyResult(retired, false)
	if err != nil || !result.OwnerPreserved || result.DataDeleted == nil || *result.DataDeleted || result.ProviderChangeResult == nil {
		t.Fatalf("Core-backed Secrets retirement result: %+v %v", result, err)
	}
	// app down removes the entire app-owned Compose project after stopping it,
	// so the broker container is absent (while its generated application secret
	// remains available from the independent Core provider).
	assertCoreBackedSecretsState(t, ctx, runtime, inventory, retired, initialOwner, baseline, retainedVolumes, envDigest, false, false, false)
	if coreBackedApplicationSecretDigest(t, ctx, runtime, inventory, retired) != secretDigest {
		t.Fatal("app down lost the retained Core-backed application secret")
	}
	if err := writeInitializedAuthoredManifest(t, manifestPath, initial.Manifest, 0600); err != nil {
		t.Fatalf("restore Core-backed Secrets intent for reactivation: %v", err)
	}
	var out bytes.Buffer
	if err := runWithIO(coreBootstrapFixtureConsent(t, ctx), []string{"app", "apply", "--skip-memory-preflight"}, &out, &out); err != nil {
		t.Fatalf("reactivate Core-backed Secrets application: %v\n%s", err, out.String())
	}
	reactivated, err := resolveApplication(ctx, application.Store{}, nil, "status")
	if err != nil || reactivated.Manifest.ApplicationID != initial.Manifest.ApplicationID || reactivated.DeploymentIdentity != initial.DeploymentIdentity {
		t.Fatalf("Secrets reactivation regenerated canonical app/deployment owner: %v", err)
	}
	status, _, err = collectResolvedApplicationStatus(ctx, reactivated)
	if err != nil || !status.Ready {
		t.Fatalf("reactivated Core-backed Secrets workload is not ready: %+v %v", status, err)
	}
	if coreBackedApplicationSecretDigest(t, ctx, runtime, inventory, reactivated) != secretDigest {
		t.Fatal("runtime retirement/reactivation lost the original application secret")
	}
	assertCoreBackedSecretsState(t, ctx, runtime, inventory, reactivated, initialOwner, baseline, retainedVolumes, envDigest, true, true, true)
}

// TestApplicationCapabilityLifecycleFootprintShowDigestConsent proves the
// selected data-capabilities acceptance shape end to end: the canonical
// read-only 'app footprint show -o json' probe reads the applied application,
// and the repository-source record-deployment binds the EXACT --confirm-
// candidate digest. It exercises the failing-then-succeeding digest path: a
// first apply without a context digest fails closed (errConsentRequiredFor-
// CandidateSubmission) and the exact digest the production choke point prints
// in its 'Re-run with --confirm-candidate <digest>' hint is captured, then the
// single re-invocation through the existing coreBootstrapFixtureConsent helper
// binds the identical submission. The footprint probe runs in the same cell
// before and after the consented apply so the cell proves the read-only probe
// reads a candidate that was bound with its own consented digest — no
// missing-digest abort and no interactive operator auth on piped input.
func TestApplicationCapabilityLifecycleFootprintShowDigestConsent(t *testing.T) {
	if os.Getenv("BASEHARBOR_CORELESS_ACCEPTANCE") != "1" {
		t.Skip("requires isolated exact-SHA runtime acceptance")
	}
	ctx, cancel := context.WithTimeout(machineNoninteractiveContext(context.Background()), 25*time.Minute)
	defer cancel()
	ctx = testOperatorContext(ctx)
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	target := configureTestTargetEmpty(t)
	if os.Getenv("BASEHARBOR_TEST_RUNTIME") == "podman" {
		// Mirrors TestApplicationCapabilityLifecycleExistingCore (:34-47): the
		// selected podman data-capabilities gate sets BASEHARBOR_TEST_RUNTIME=podman
		// but the empty target defaults to the rootful docker provider, so the
		// runtime selection below fails with "detected Docker endpoint rootful vs
		// Target rootless". Pin the provider override before detecting the runtime.
		cfg, err := deployment.LoadConfig()
		if err != nil {
			t.Fatal(err)
		}
		definition := cfg.Targets[target.Name]
		definition.Runtime.Provider = "podman"
		cfg.Targets[target.Name] = definition
		if err := cfg.Save(); err != nil {
			t.Fatal(err)
		}
		target, err = cfg.ResolveTarget("", "")
		if err != nil {
			t.Fatal(err)
		}
	}
	root := t.TempDir()
	t.Chdir(root)
	runtime, err := detectRuntimeForTarget(ctx, target)
	if err != nil {
		t.Fatal(err)
	}
	assertCorelessHost(t, ctx, runtime, target)
	var out bytes.Buffer
	first, err := installCore(ctx, strings.NewReader(""), &out, runtimeUpOptions{Yes: true, ControlPlaneOnly: true, MachineRole: coreinstallation.Development, RecoveryFile: filepath.Join(t.TempDir(), "recovery.json")})
	if err != nil || !first.Ready {
		logCoreBootstrapFailure(t, runtime, target.Name, target.RuntimeProvider)
		t.Fatalf("explicit management install: %v\n%s", err, out.String())
	}
	if err := deployment.SetOperatorAuthForTargetEnvironment(target.Name, "dev", deployment.OperatorAuthEnvironmentConfig{
		Provider: "external-oidc", Issuer: "https://issuer.test", ClientID: "cli-test",
	}); err != nil {
		t.Fatalf("configure isolated operator-auth boundary: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cleanupCancel()
		if err := approvedNativeFixtureCleanup(t, testOperatorContext(machineNoninteractiveContext(cleanupCtx)), &bytes.Buffer{}); err != nil {
			t.Errorf("isolated Core cleanup: %v", err)
		}
	})

	m := application.WithWorkloadComponents(application.New("footprint-consent", "dev", false, false, false), "api")
	if err := writeInitializedAuthoredManifest(t, "baseharbor.yaml", m, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("compose.yaml", []byte(corelessHTTPWorkload), 0600); err != nil {
		t.Fatal(err)
	}
	defer func() {
		// Robust teardown: chdir back to the safe root first, then run the
		// fixture cleanup via t.Cleanup's verified-owner path so a mid-test
		// failure still drains the isolated Core/target without aborting the
		// reset. resetCoreBootstrapFixtureApplication requires an already-resolved
		// target/deployment owner and fails closed once the cell is torn down,
		// so prefer the cleanup helper instead.
		if _, werr := os.Getwd(); werr == nil {
			_ = os.Chdir(root)
		}
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cleanupCancel()
		if err := approvedNativeFixtureCleanup(t, testOperatorContext(machineNoninteractiveContext(cleanupCtx)), &bytes.Buffer{}); err != nil {
			t.Errorf("footprint-consent fixture cleanup: %v", err)
		}
	}()

	// The canonical read-only footprint probe is available before any apply so
	// the cell reads the pre-apply footprint through the same JSON path.
	if err := runWithIO(ctx, []string{"app", "footprint", "show", "-o", "json"}, &out, &out); err != nil {
		t.Fatalf("pre-apply footprint show: %v\n%s", err, out.String())
	}

	// First apply carries no context digest: the fail-closed candidate choke
	// point must refuse and print the exact confirm digest to re-run with.
	out.Reset()
	err = runWithIO(ctx, []string{"app", "apply", "--skip-memory-preflight"}, &out, &out)
	if err == nil {
		t.Fatalf("apply without --confirm-candidate digest must fail closed")
	}
	if !strings.Contains(err.Error(), "--confirm-candidate") {
		t.Fatalf("apply failure did not name the exact confirm digest: %v\n%s", err, out.String())
	}
	// The fail-closed missing-consent error surfaces the exact confirm digest only
	// in the usage Hint (rendered to captured stdout), not in err.Error(), which
	// carries just the message. Extract from the combination of err.Error() and
	// the captured out buffer so the digest the choke point printed is recovered.
	confirmDigest := extractFootprintShowConfirmDigest(err.Error() + "\n" + out.String())
	if confirmDigest == "" {
		t.Fatalf("could not parse the exact --confirm-candidate digest from the apply refusal: %v\n%s", err, out.String())
	}

	// The single re-invocation through the existing consent helper binds the
	// identical submission the production choke point fingerprinted.
	out.Reset()
	if err := runWithIO(coreBootstrapFixtureConsent(t, ctx), []string{"app", "apply", "--skip-memory-preflight"}, &out, &out); err != nil {
		t.Fatalf("consented re-apply after digest capture: %v\n%s", err, out.String())
	}

	// The read-only footprint probe now reads the consented candidate in the
	// same cell: no missing-digest abort, no interactive operator auth.
	out.Reset()
	if err := runWithIO(ctx, []string{"app", "footprint", "show", "-o", "json"}, &out, &out); err != nil {
		t.Fatalf("post-apply footprint show: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "footprint") {
		t.Fatalf("post-apply footprint show did not emit the canonical JSON footprint: %s", out.String())
	}
	t.Logf("footprint-show digest-consent cycle complete: apply refused without digest %s, consented re-apply bound it, read-only footprint show succeeded", confirmDigest)
}

// extractFootprintShowConfirmDigest parses the exact 64-hex confirm digest from
// an apply refusal transcript. It accepts both the interactive prompt form
// ("confirm-hash <digest>") and the fail-closed missing-consent hint
// ("Re-run with --confirm-candidate <digest> ... to acknowledge this exact
// submission."), so a test can capture the digest the production choke point
// prints on either path and re-run with it. This gate path is wired into the
// data-capabilities and coreless-lifecycle native gates (scripts/ci-native-pr-gate.sh)
// and the core-capability shard (scripts/pre-release-integration.py).
func extractFootprintShowConfirmDigest(out string) string {
	re := regexp.MustCompile(`--confirm-candidate\s+([0-9a-f]{64})`)
	if m := re.FindStringSubmatch(out); m != nil {
		return m[1]
	}
	for _, line := range strings.Split(out, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "confirm-hash ") {
			digest := strings.TrimSpace(strings.TrimPrefix(trimmed, "confirm-hash "))
			if digest != "" && strings.Count(digest, " ") == 0 {
				return digest
			}
		}
	}
	return ""
}

func coreBackedApplicationSecretDigest(t *testing.T, ctx context.Context, runtime bhruntime.RuntimeProvider, inventory bhruntime.ControlPlaneRuntimeInventory, resolved resolvedApplication) [32]byte {
	t.Helper()
	files := application.RuntimeFilesFor(resolved.Store, resolved.Manifest)
	service := applicationsecret.NewForApplicationRuntime(resolved.Store, runtime, inventory.Files, resolved.Manifest, files)
	value, err := service.Get(ctx, resolved.Manifest.Name, "API_TOKEN")
	if err != nil || len(value) == 0 {
		clear(value)
		t.Fatalf("generated application secret is not readable through its owned Core scope: %v", err)
	}
	digest := sha256.Sum256(value)
	clear(value)
	return digest
}

func assertCoreBackedSecretsState(t *testing.T, ctx context.Context, runtime bhruntime.RuntimeProvider, inventory bhruntime.ControlPlaneRuntimeInventory, resolved resolvedApplication, initialOwner coreinstallation.State, baseline map[string]bhruntime.RuntimeContainer, volumes []bhruntime.ProjectResource, envDigest [32]byte, brokerExists, brokerRunning, workloadExpected bool) {
	t.Helper()
	containers, err := runtime.ListRuntimeContainers(ctx)
	if err != nil {
		t.Fatalf("inspect runtime during Core-backed Secrets lifecycle: %v", err)
	}
	_, changed := runtimeContainerBaselineDiff(containers, baseline)
	if len(changed) != 0 {
		t.Fatalf("Core/provider baseline changed during Secrets lifecycle: %v", changed)
	}
	root, err := targetRuntimeStateRoot(resolved.Target)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := coreinstallation.Load(root)
	if err != nil || owner.ID != initialOwner.ID || owner.Spec != initialOwner.Spec || owner.Ready != initialOwner.Ready {
		t.Fatalf("Core installation owner changed during Secrets lifecycle: before=%+v after=%+v err=%v", initialOwner, owner, err)
	}
	actual, err := runtime.InspectProjectResources(ctx, inventory.Files.Project, inventory.Resources)
	if err != nil {
		t.Fatalf("inspect Core provider inventory during Secrets lifecycle: %v", err)
	}
	// The canonical resource inventory also includes profile-only helpers and
	// successful one-shot jobs such as postgres-init. They are retained in the
	// owner inventory for lifecycle/recovery, but are not expected to exist as
	// persistent containers on every runtime. Require all continuous Core
	// services here; the full runtime baseline below still catches changes to
	// every container actually present before the Secrets lifecycle.
	continuousServices := append(
		inventory.ContinuousServices(inventory.SQLServices),
		inventory.ContinuousServices(inventory.SecretsServices)...,
	)
	for _, service := range continuousServices {
		resource := bhruntime.ProjectResource{
			Kind: "container",
			Name: inventory.Files.Project + "-" + service + "-1",
		}
		if !application.ResourceNamedExists(actual, resource) {
			t.Fatalf("continuous Core provider container disappeared during Secrets lifecycle: %s", resource.Name)
		}
	}
	for _, volume := range volumes {
		if !application.ResourceNamedExists(actual, volume) {
			t.Fatalf("Core provider data volume disappeared during Secrets lifecycle: %s", volume.Name)
		}
	}
	data, err := os.ReadFile(inventory.Files.Env)
	if err != nil || sha256.Sum256(data) != envDigest {
		t.Fatalf("Core provider configuration changed during Secrets lifecycle: %v", err)
	}
	files := application.RuntimeFilesFor(resolved.Store, resolved.Manifest)
	brokerProject := runtimebroker.ProjectNameForRuntime(resolved.Manifest, files)
	brokerResource := bhruntime.ProjectResource{Kind: "container", Name: brokerProject + "-broker-1"}
	brokerCount, runningBrokerCount, workloadCount := 0, 0, 0
	baselineIDs := make(map[string]struct{}, len(baseline))
	for id := range baseline {
		baselineIDs[id] = struct{}{}
	}
	for _, container := range runtimeContainersNotInBaseline(containers, baselineIDs) {
		if bhruntime.ContainerMatchesProjectResource(runtime.Kind(), container, brokerProject, brokerResource) {
			brokerCount++
			if container.Running {
				runningBrokerCount++
			}
			continue
		}
		workload := bhruntime.ProjectResource{Kind: "container", Name: files.Project + "-api-1"}
		if bhruntime.ContainerMatchesProjectResource(runtime.Kind(), container, files.Project, workload) {
			workloadCount++
			continue
		}
		t.Fatalf("Secrets application materialized unexpected runtime project=%s service=%s", container.Project, container.Service)
	}
	if (brokerExists && brokerCount != 1) || (!brokerExists && brokerCount != 0) ||
		(brokerRunning && runningBrokerCount != 1) || (!brokerRunning && runningBrokerCount != 0) ||
		(workloadExpected && workloadCount != 1) || (!workloadExpected && workloadCount != 0) {
		t.Fatalf("Core-backed Secrets runtime differs from request: workload=%d broker=%d running-broker=%d expected-workload=%t expected-broker-exists=%t expected-broker-running=%t", workloadCount, brokerCount, runningBrokerCount, workloadExpected, brokerExists, brokerRunning)
	}
}
