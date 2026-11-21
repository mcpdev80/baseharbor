package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/devgateway"
	"github.com/mcpdev80/baseharbor/internal/exposure"
	"github.com/mcpdev80/baseharbor/internal/identityprovider"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// This gate does not install management Core or the runtime broker. Its scope
// is actual selected Identity discovery/JWKS readiness, native add/remove/re-add
// and retained credentials/data, plus actual end-user OAuth/PKCE and sessions.
func TestSelectiveIdentityRuntimeAddRemoveReAdd(t *testing.T) {
	if os.Getenv("BASEHARBOR_IDENTITY_LIFECYCLE_ACCEPTANCE") != "1" {
		t.Skip("requires isolated exact-SHA Docker/rootless Podman Identity acceptance")
	}
	ctx, cancel := context.WithTimeout(machineNoninteractiveContext(context.Background()), 15*time.Minute)
	defer cancel()
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	target := configureTestTargetEmpty(t)
	t.Setenv("BASEHARBOR_TARGET", target.Name)
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
	provider, err := detectRuntimeForTarget(ctx, target)
	if err != nil {
		t.Fatal(err)
	}
	assertCorelessHost(t, ctx, provider, target)
	baselineContainers, err := provider.ListRuntimeContainers(ctx)
	if err != nil {
		t.Fatal("inventory isolated runtime baseline", err)
	}
	baselineIDs := runtimeContainerIDSet(baselineContainers)
	t.Chdir(t.TempDir())
	if err := os.WriteFile("compose.yaml", []byte(corelessHTTPWorkload), 0600); err != nil {
		t.Fatal(err)
	}
	writeIntent := func(enabled bool) {
		content := selectiveIdentityOAuthRuntimeIntent(enabled)
		if err := os.WriteFile("baseharbor.yaml", []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	writeIntent(true)
	var out bytes.Buffer
	if err := runWithIO(ctx, []string{"app", "init", "--input", "tls_mode=local", "--yes", "--json"}, &out, &out); err != nil {
		t.Fatalf("initialize authored ID-free workload: %v\n%s", err, out.String())
	}
	initialized, err := resolveApplication(ctx, application.Store{}, nil, "plan")
	if err != nil {
		t.Fatal(err)
	}
	applicationID := initialized.Manifest.ApplicationID
	if applicationID == "" {
		t.Fatal("initialization has no stable application identity")
	}
	environmentDestroyed := false
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(machineNoninteractiveContext(context.Background()), 2*time.Minute)
		defer cleanupCancel()
		if _, err := exposure.RetireRuntime(cleanupCtx, provider, initialized.Manifest, application.RuntimeFilesFor(initialized.Store, initialized.Manifest)); err != nil && !os.IsNotExist(err) {
			t.Errorf("isolated callback exposure cleanup: %v", err)
		}
		if !environmentDestroyed {
			if err := destroySelectiveApplicationWithConsent(cleanupCtx); err != nil {
				t.Errorf("isolated workload cleanup: %v", err)
			}
		}
		placement := capability.ProviderPlacement{Scope: capability.ScopeShared, Ownership: capability.OwnershipBaseHarbor}
		if _, err := identityprovider.RetireKeycloakRuntime(cleanupCtx, provider, initialized.Manifest, placement, initialized.TargetStateRoot, target.Name); err != nil && !os.IsNotExist(err) {
			t.Errorf("isolated Identity container cleanup: %v", err)
		}
		if err := removeApplicationGatewayOwners(cleanupCtx, provider, initialized, "app/"+initialized.Manifest.Name+"/"+initialized.Manifest.Environment, "shared/keycloak"); err != nil {
			t.Errorf("isolated Identity gateway cleanup: %v", err)
		}
		if containers, err := provider.ListRuntimeContainers(cleanupCtx); err != nil {
			t.Errorf("inspect isolated native cleanup: %v", err)
		} else if remaining := runtimeContainersNotInBaseline(containers, baselineIDs); len(remaining) != 0 {
			t.Errorf("isolated native cleanup retained test containers: %v", remaining)
		}
	})
	var originalEnv [32]byte
	var retained []bhruntime.ProjectResource
	var oauth *identityOAuthRuntimeSession
	for phase, enabled := range []bool{true, false, true} {
		writeIntent(enabled)
		resolved, err := resolveApplication(ctx, application.Store{}, nil, "apply")
		if err != nil {
			t.Fatal(err)
		}
		plan, err := buildResolvedApplicationPlan(ctx, resolved)
		if err != nil {
			t.Fatal(err)
		}
		applyCtx, candidateDigest := withConfirmCandidateDigestForResolved(ctx, resolved)
		if resolved.FromRepository && candidateDigest == "" {
			t.Fatal("repository candidate has no verifiable confirmation digest")
		}
		if plan.ProviderPlan.Impact != nil {
			impact := plan.ProviderPlan.Impact
			applyCtx = withProviderChangeConsent(applyCtx, &application.ProviderChangeConsent{PlanID: impact.PlanID, First: true, Second: true})
		}
		out.Reset()
		if err := runWithIO(applyCtx, []string{"app", "apply", "--skip-memory-preflight"}, &out, &out); err != nil {
			// Keep the exact failing readiness checks in the bounded receipt.
			if current, statusErr := resolveApplication(ctx, application.Store{}, nil, "status"); statusErr == nil {
				status, _, inspectErr := collectResolvedApplicationStatus(ctx, current)
				t.Logf("failed apply readiness: %+v inspection=%v", status, inspectErr)
			}
			t.Fatalf("Identity phase %d enabled=%t: %v\n%s", phase, enabled, err, out.String())
		}
		resolved, err = resolveApplication(ctx, application.Store{}, nil, "status")
		if err != nil {
			t.Fatal(err)
		}
		if resolved.Manifest.ApplicationID != applicationID {
			t.Fatal("capability change replaced application identity")
		}
		status, _, err := collectResolvedApplicationStatus(ctx, resolved)
		if err != nil || !status.Ready {
			t.Fatalf("phase %d runtime verification: %+v %v", phase, status, err)
		}
		placement := capability.ProviderPlacement{Scope: capability.ScopeShared, Ownership: capability.OwnershipBaseHarbor}
		files, resources, err := identityprovider.ExistingKeycloakRuntimeInventoryAt(initialized.Manifest, placement, initialized.TargetStateRoot, target.Name)
		if err != nil {
			t.Fatal(err)
		}
		actual, err := provider.InspectProjectResources(ctx, files.Project, resources)
		if err != nil {
			t.Fatal(err)
		}
		if phase == 0 {
			data, err := os.ReadFile(files.Env)
			if err != nil {
				t.Fatal(err)
			}
			originalEnv = sha256.Sum256(data)
			for _, resource := range actual {
				if resource.Kind == "volume" {
					retained = append(retained, resource)
				}
			}
			if len(retained) == 0 {
				t.Fatal("Identity data has no observed owned volume")
			}
		} else {
			data, err := os.ReadFile(files.Env)
			if err != nil || sha256.Sum256(data) != originalEnv {
				t.Fatal("Identity change replaced retained credential/configuration material")
			}
			for _, volume := range retained {
				if !application.ResourceNamedExists(actual, volume) {
					t.Fatalf("phase %d lost retained volume %s", phase, volume.Name)
				}
			}
		}
		containers, err := provider.ListRuntimeContainers(ctx)
		if err != nil {
			t.Fatal(err)
		}
		identityCount := 0
		callbackCount := 0
		appFiles := application.RuntimeFilesFor(resolved.Store, resolved.Manifest)
		callbackInventory, err := exposure.ExistingRuntimeInventory(initialized.Manifest, appFiles)
		if err != nil {
			t.Fatal(err)
		}
		gatewayFiles, err := devgateway.FilesFor(target.Name)
		if err != nil {
			t.Fatal(err)
		}
		for _, container := range containers {
			if _, existed := baselineIDs[container.ID]; existed {
				continue
			}
			matched := false
			for _, resource := range resources {
				if resource.Kind == "container" && bhruntime.ContainerMatchesProjectResource(provider.Kind(), container, files.Project, resource) {
					identityCount++
					matched = true
					break
				}
			}
			for _, resource := range callbackInventory.Resources {
				if bhruntime.ContainerMatchesProjectResource(provider.Kind(), container, callbackInventory.State.Project, resource) {
					callbackCount++
					matched = true
				}
			}
			workload := bhruntime.ProjectResource{Kind: "container", Name: appFiles.Project + "-api-1"}
			matched = matched || bhruntime.ContainerMatchesProjectResource(provider.Kind(), container, appFiles.Project, workload)
			// The selected development Identity access route uses its own TLS
			// gateway. It is an actual dependency, never a full Core exemption.
			if enabled {
				gateway := bhruntime.ProjectResource{Kind: "container", Name: gatewayFiles.Project + "-dev-gateway-1"}
				matched = matched || bhruntime.ContainerMatchesProjectResource(provider.Kind(), container, gatewayFiles.Project, gateway)
			}
			if !matched {
				t.Fatalf("unexpected realization in isolated runtime: project=%s service=%s", container.Project, container.Service)
			}
		}
		if (enabled && callbackCount != 1) || (!enabled && callbackCount != 0) {
			t.Fatalf("phase %d actual callback exposure count=%d", phase, callbackCount)
		}
		if (enabled && identityCount == 0) || (!enabled && identityCount != 0) {
			t.Fatalf("phase %d native Identity count=%d", phase, identityCount)
		}
		if enabled {
			if phase == 0 {
				oauth = startIdentityOAuthRuntimeSession(t, ctx, resolved, files)
				oauth.refresh(t, ctx)
			} else {
				oauth.refresh(t, ctx)
				oauth.login(t, ctx, false)
			}
			if err := application.VerifyIdentityBinding(resolved.Manifest, appFiles); err != nil {
				t.Fatal(err)
			}
		} else {
			oauth.assertUnavailable(t, ctx)
			t.Log("Identity runtime removed; owned volumes and credential/configuration files retained")
		}
		if data, err := os.ReadFile("baseharbor.yaml"); err != nil || strings.Contains(string(data), applicationID) {
			t.Fatal("lifecycle wrote an allocated ID into authored intent")
		}
		if _, err := os.Stat(filepath.Join(resolved.TargetStateRoot, "service-access", "issuer")); err != nil {
			t.Fatal("standalone service issuer is missing")
		}
		t.Logf("runtime=%s phase=%d Identity=%t selected Identity containers=%d actual total=%d retained data volumes=%d selected callback exposures=%d", provider.Kind(), phase, enabled, identityCount, len(containers), len(retained), callbackCount)
	}
	qualifySharedIdentityConsumers(t, ctx, provider, initialized, originalEnv, retained)

	// Exercise environment-scoped destruction against the real selected runtime
	// after the workload and Identity lifecycle have completed. DELETE is
	// explicitly consented here so the existing app lifecycle removes the
	// deployment owner before the environment identity is removed.
	var destroyOutput, destroyError bytes.Buffer
	if err := runWithIO(ctx, []string{"env", "destroy", "dev", "--delete-data", "--confirm-delete-data", "--yes", "--json"}, &destroyOutput, &destroyError); err != nil {
		t.Fatalf("native environment destroy: %v\nstdout: %s\nstderr: %s", err, destroyOutput.String(), destroyError.String())
	}
	var destroyResult envDestroyResult
	if err := json.Unmarshal(destroyOutput.Bytes(), &destroyResult); err != nil {
		t.Fatalf("native environment destroy returned invalid JSON: %v\n%s", err, destroyOutput.String())
	}
	if !destroyResult.Success || destroyResult.Environment != "dev" || len(destroyResult.Scoped) != 1 {
		t.Fatalf("native environment destroy did not remove its scoped deployment: %+v", destroyResult)
	}
	if exists, err := deployment.EnvironmentStateExists(target.Name, "dev"); err != nil || exists {
		t.Fatalf("native environment identity remains after destroy: exists=%v err=%v", exists, err)
	}
	for _, record := range mustListCLIEnvDeployments(t, target.Name) {
		if record.Identity.ApplicationID == applicationID && record.Identity.Environment == "dev" {
			t.Fatalf("native environment destroy left scoped deployment owner: %+v", record.Identity)
		}
	}
	environmentDestroyed = true
}

func selectiveIdentityOAuthRuntimeIntent(enabled bool) string {
	content := fmt.Sprintf("version: 1\napp:\n  name: selective-identity\n  environment: dev\nworkload:\n  components:\n    - api\nservices:\n  identity: %t\n", enabled)
	if enabled {
		content += "identity:\n  callback_paths:\n    - /oauth/callback\nexposure:\n  http:\n    - name: oauth\n      service: api\n      port: 8080\n      protocol: https\n      visibility: public\n"
	}
	return content
}

func runtimeContainerIDSet(containers []bhruntime.RuntimeContainer) map[string]struct{} {
	ids := make(map[string]struct{}, len(containers))
	for _, container := range containers {
		if container.ID != "" {
			ids[container.ID] = struct{}{}
		}
	}
	return ids
}

func runtimeContainersNotInBaseline(containers []bhruntime.RuntimeContainer, baseline map[string]struct{}) []bhruntime.RuntimeContainer {
	remaining := make([]bhruntime.RuntimeContainer, 0, len(containers))
	for _, container := range containers {
		if _, existed := baseline[container.ID]; !existed {
			remaining = append(remaining, container)
		}
	}
	return remaining
}

func TestRuntimeContainersNotInBaseline(t *testing.T) {
	baseline := runtimeContainerIDSet([]bhruntime.RuntimeContainer{{ID: "registry"}})
	current := []bhruntime.RuntimeContainer{
		{ID: "registry", Name: "baseharbor-pre-release-registry"},
		{ID: "new-app", Name: "isolated-app-api-1"},
	}
	remaining := runtimeContainersNotInBaseline(current, baseline)
	if len(remaining) != 1 || remaining[0].ID != "new-app" {
		t.Fatalf("unexpected containers outside runtime baseline: %+v", remaining)
	}
}
