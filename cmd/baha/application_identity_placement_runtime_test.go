package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/identityprovider"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// Actual CLI placement changes must deliver a functioning OAuth consumer and
// retire only the former realization after destination verification.
func TestManagedIdentityNativePlacementHandoff(t *testing.T) {
	if os.Getenv("BASEHARBOR_IDENTITY_LIFECYCLE_ACCEPTANCE") != "1" {
		t.Skip("requires isolated rootless Identity acceptance")
	}
	ctx, cancel := context.WithTimeout(machineNoninteractiveContext(t.Context()), 18*time.Minute)
	defer cancel()
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv("BASEHARBOR_IDENTITY_PROVIDER", "keycloak")
	t.Setenv(application.ProviderScopeEnv(capability.ProviderKeycloak), "shared")
	target := configureTestTarget(t)
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
	runtime, err := detectRuntimeForTarget(ctx, target)
	if err != nil {
		t.Fatal(err)
	}
	assertCorelessHost(t, ctx, runtime, target)
	t.Chdir(t.TempDir())
	if err := os.WriteFile("compose.yaml", []byte(corelessHTTPWorkload), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("baseharbor.yaml", []byte(selectiveIdentityOAuthRuntimeIntent(true)), 0600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := runWithIO(ctx, []string{"app", "init", "--input", "tls_mode=local", "--yes", "--json"}, &output, &output); err != nil {
		t.Fatal("Identity fixture initialization failed", err)
	}
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(machineNoninteractiveContext(context.Background()), 3*time.Minute)
		defer stop()
		if err := approvedNativeFixtureCleanup(t, cleanup, &bytes.Buffer{}); err != nil {
			t.Errorf("owned placement fixture cleanup failed: %v", err)
		}
		containers, err := runtime.ListRuntimeContainers(cleanup)
		if err != nil || len(containers) != 0 {
			t.Error("owned placement fixture containers remain or cannot be verified")
		}
	})
	var original identityprovider.KeycloakFiles
	var originalEnv, originalSecret []byte
	var originalResources []bhruntime.ProjectResource
	var prior *identityOAuthRuntimeSession
	var priorFiles identityprovider.KeycloakFiles
	var priorResources []bhruntime.ProjectResource
	var appID string
	for phase, scope := range []string{"shared", "application", "shared"} {
		t.Setenv(application.ProviderScopeEnv(capability.ProviderKeycloak), scope)
		resolved, err := resolveApplication(ctx, application.Store{}, nil, "apply")
		if err != nil {
			t.Fatal(err)
		}
		plan, err := buildResolvedApplicationPlan(ctx, resolved)
		if err != nil {
			t.Fatal(err)
		}
		applyContext := ctx
		if phase > 0 {
			impact := plan.ProviderPlan.Impact
			if impact == nil || impact.Attention != "Achtung" || impact.ConfirmationsRequired != 2 {
				t.Fatal("native replacement lacks current unknown-consequence acknowledgements")
			}
			applyContext = withProviderChangeConsent(ctx, &application.ProviderChangeConsent{PlanID: impact.PlanID, First: true, Second: true})
		}
		output.Reset()
		if err := runWithIO(applyContext, []string{"app", "apply", "--skip-memory-preflight"}, &output, &output); err != nil {
			t.Fatalf("native Identity placement phase %d failed: %v\n%s", phase, err, output.String())
		}
		resolved, err = resolveApplication(ctx, application.Store{}, nil, "status")
		if err != nil {
			t.Fatal(err)
		}
		if phase == 0 {
			appID = resolved.Manifest.ApplicationID
		} else if appID != resolved.Manifest.ApplicationID {
			t.Fatal("placement changed canonical application identity")
		}
		status, _, err := collectResolvedApplicationStatus(ctx, resolved)
		if err != nil || !status.Ready {
			t.Fatal("destination readiness is unverified", err)
		}
		placement := capability.ProviderPlacement{Scope: capability.ProviderScope(scope), Ownership: capability.OwnershipBaseHarbor}
		files, resources, err := identityprovider.ExistingKeycloakRuntimeInventoryAt(resolved.Manifest, placement, resolved.TargetStateRoot, target.Name)
		if err != nil {
			t.Fatal(err)
		}
		if phase == 0 {
			original = files
			originalResources = resources
			originalEnv, err = os.ReadFile(files.Env)
			if err != nil {
				t.Fatal(err)
			}
			originalSecret, err = os.ReadFile(filepath.Join(application.RuntimeFilesFor(resolved.Store, resolved.Manifest).Bindings, application.IdentityBindingName, "client-secret"))
			if err != nil {
				t.Fatal(err)
			}
		} else {
			current, err := os.ReadFile(original.Env)
			if err != nil || !bytes.Equal(current, originalEnv) {
				t.Fatal("placement changed original protected provider configuration")
			}
			retained, err := runtime.InspectProjectResources(ctx, original.Project, originalResources)
			if err != nil {
				t.Fatal(err)
			}
			for _, resource := range originalResources {
				if resource.Kind == "volume" && !application.ResourceNamedExists(retained, resource) {
					t.Fatal("placement removed original data volume")
				}
			}
			stopped, err := runtime.InspectProjectResources(ctx, priorFiles.Project, priorResources)
			if err != nil {
				t.Fatal(err)
			}
			for _, resource := range stopped {
				if resource.Kind == "container" {
					t.Fatal("former realization remains running after destination verification")
				}
			}
			prior.assertUnavailable(t, ctx)
			if phase == 2 {
				restored, err := os.ReadFile(filepath.Join(application.RuntimeFilesFor(resolved.Store, resolved.Manifest).Bindings, application.IdentityBindingName, "client-secret"))
				if err != nil || !bytes.Equal(restored, originalSecret) {
					t.Fatal("return to original authority changed its original client credential")
				}
			}
		}
		oauth := startIdentityOAuthRuntimeSession(t, ctx, resolved, files)
		oauth.refresh(t, ctx)
		prior, priorFiles, priorResources = oauth, files, resources
		t.Logf("[native-managed-identity] phase=%d placement=%s OAuth/PKCE, JWT/userinfo, refresh, verified destination and retained original data PASS", phase, scope)
	}
}
