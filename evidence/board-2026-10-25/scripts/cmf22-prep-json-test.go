package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// Blocking finding R2 regression from the CMF21 independent review evidence
// (evidence/board-2026-10-25/codex-verdict-1077-6f165d30-r3.md), ported from
// evidence/board-2026-10-25/scripts/cmf21-authorization-review.go against the
// real preparation caller, with the serialized machine-JSON assertions and the
// measured-runtime positive control required by the worker recipe.

// R2: a registry-only preview projection can never authorize the retirement
// preparation gate. The real preparation caller must fail closed with
// provider_retirement_unverifiable and carry the typed materialization cause
// with code/instance/Target context through the serialized machine JSON.
func TestCMF21ProjectionCannotAuthorizeProviderRetirement(t *testing.T) {
	target := configureTestTarget(t)
	manifest := application.New("cmf20-neutral", "dev", true, false, false)
	manifest, err := application.WithBackendPlacementProjection(manifest, map[capability.ProviderKind]capability.ProviderPlacement{
		capability.ProviderPostgreSQL: {Scope: capability.ScopeApplication, Ownership: capability.OwnershipBaseHarbor},
	})
	if err != nil {
		t.Fatal(err)
	}
	root, err := deployment.TargetStateRoot(target.Name)
	if err != nil {
		t.Fatal(err)
	}
	instanceID := "postgresql/application/" + manifest.ApplicationID + "/" + application.ProviderPlacementNameToken(manifest.Environment) + "/" + application.ProviderPlacementNameToken("default")
	registry := capability.NewRegistry()
	if err := registry.Register(capability.ProviderInstance{ID: instanceID, Provider: capability.Provider{Kind: capability.ProviderPostgreSQL, Capabilities: []capability.Kind{capability.SQL}}, Scope: capability.ScopeApplication, Ownership: capability.OwnershipBaseHarbor, OwnerApplicationID: manifest.ApplicationID, OwnerApplication: manifest.Name, OwnerEnvironment: manifest.Environment}); err != nil {
		t.Fatal(err)
	}
	if err := registry.BindDeployment(capability.Resource{Application: manifest.Name, Kind: capability.SQL, Name: "default", Provider: capability.ProviderPostgreSQL}, manifest.ApplicationID, manifest.Environment, instanceID); err != nil {
		t.Fatal(err)
	}
	if err := (capability.RegistryStore{Path: filepath.Join(root, "provider-registry.json")}).Save(registry); err != nil {
		t.Fatal(err)
	}
	target.RuntimeProvider = "cmf20-unsupported-runtime"
	resolved := resolvedApplication{Target: target, TargetStateRoot: root, Manifest: manifest, FromRepository: true, Store: application.Store{Root: filepath.Join(t.TempDir(), "state"), Namespace: target.Name}}
	registerTestDeployment(t, target, manifest, t.TempDir(), t.TempDir())
	withdrawn := resolved
	withdrawn.Manifest = applicationRuntimeRetirementIntent(resolved.Manifest)
	plan, err := prepareApplicationProviderRetirement(context.Background(), withdrawn)
	if err == nil {
		t.Fatalf("registry-only projection passed retirement authorization with unavailable runtime: complete=%v changes=%+v", plan.Complete, plan.Changes)
	}
	if !containsCauseCode(err, "provider_retirement_unverifiable") {
		t.Fatalf("preparation error lost its fail-closed machine cause: %v", err)
	}
	if !containsCauseCode(err, causeRuntimeInventoryNotYetMaterialized) {
		t.Fatalf("preparation lost the typed materialization diagnostic: %v", err)
	}
	t.Logf("fail-closed retirement authorization: %v", err)
	var out bytes.Buffer
	// Match main.go's real CLI classification and JSON encoder.
	if writeErr := writeJSON(&out, machine.ResultError(classifyMachineCLIError(err))); writeErr != nil {
		t.Fatal(writeErr)
	}
	t.Logf("serialized CLI preparation result: %s", out.String())
	var decoded struct {
		Error struct {
			Cause   string `json:"cause"`
			Details struct {
				Diagnostics []struct {
					Code     string `json:"code"`
					Cause    string `json:"cause"`
					Resource string `json:"resource"`
					Details  struct {
						Instance      string `json:"instance"`
						Target        string `json:"target"`
						ApplicationID string `json:"application_id"`
					} `json:"details"`
				} `json:"materialization_diagnostics"`
			} `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Error.Cause != "provider_retirement_unverifiable" {
		t.Fatalf("serialized failure lost the fail-closed cause: %s", out.String())
	}
	if len(decoded.Error.Details.Diagnostics) == 0 {
		t.Fatalf("serialized failure lost the materialization diagnostic: %s", out.String())
	}
	for _, diagnostic := range decoded.Error.Details.Diagnostics {
		if diagnostic.Code != string(machine.ErrorRuntimeUnavailable) || diagnostic.Cause != causeRuntimeInventoryNotYetMaterialized || diagnostic.Resource != instanceID || diagnostic.Details.Instance != instanceID || diagnostic.Details.Target != resolved.Target.Name || diagnostic.Details.ApplicationID != resolved.Manifest.ApplicationID {
			t.Fatalf("serialized diagnostic lost code/instance/Target context: %s", out.String())
		}
	}
}

// R2 positive control: an actual measured runtime observation (zero owned
// containers via the canonical inventory) still authorizes valid retirement
// preparation with the complete remove-runtime plan.
func TestCMF21MeasuredRuntimeStillAuthorizesProviderRetirement(t *testing.T) {
	target := configureTestTarget(t)
	manifest := application.New("cmf21-positive", "dev", true, false, false)
	manifest, err := application.WithBackendPlacementProjection(manifest, map[capability.ProviderKind]capability.ProviderPlacement{
		capability.ProviderPostgreSQL: {Scope: capability.ScopeApplication, Ownership: capability.OwnershipBaseHarbor},
	})
	if err != nil {
		t.Fatal(err)
	}
	root, err := deployment.TargetStateRoot(target.Name)
	if err != nil {
		t.Fatal(err)
	}
	instanceID := "postgresql/application/" + manifest.ApplicationID + "/" + application.ProviderPlacementNameToken(manifest.Environment) + "/" + application.ProviderPlacementNameToken("default")
	registry := capability.NewRegistry()
	if err := registry.Register(capability.ProviderInstance{ID: instanceID, Provider: capability.Provider{Kind: capability.ProviderPostgreSQL, Capabilities: []capability.Kind{capability.SQL}}, Scope: capability.ScopeApplication, Ownership: capability.OwnershipBaseHarbor, OwnerApplicationID: manifest.ApplicationID, OwnerApplication: manifest.Name, OwnerEnvironment: manifest.Environment}); err != nil {
		t.Fatal(err)
	}
	if err := registry.BindDeployment(capability.Resource{Application: manifest.Name, Kind: capability.SQL, Name: "default", Provider: capability.ProviderPostgreSQL}, manifest.ApplicationID, manifest.Environment, instanceID); err != nil {
		t.Fatal(err)
	}
	if err := (capability.RegistryStore{Path: filepath.Join(root, "provider-registry.json")}).Save(registry); err != nil {
		t.Fatal(err)
	}
	registerTestDeployment(t, target, manifest, t.TempDir(), t.TempDir())
	deploymentRoot, err := deployment.DeploymentRoot(deployment.DeploymentIdentity{DeploymentID: testDeploymentID, ApplicationID: manifest.ApplicationID, Target: target.Name, Application: manifest.Name, Environment: manifest.Environment})
	if err != nil {
		t.Fatal(err)
	}
	resolved := resolvedApplication{Target: target, TargetStateRoot: root, Manifest: manifest, FromRepository: true, Store: application.Store{Root: filepath.Join(deploymentRoot, "state"), Namespace: target.Name}}
	files := application.RuntimeFilesFor(resolved.Store, resolved.Manifest)
	if err := os.MkdirAll(files.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	compose, err := application.RuntimeComposeYAMLForProject(resolved.Manifest, files.ResourceProject)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(files.Compose, []byte(compose), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(files.Env, []byte("preserved=protected\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	target.RuntimeProvider = "cmf21-measured-fake"
	original := detectRuntimeForTargetFunc
	t.Cleanup(func() { detectRuntimeForTargetFunc = original })
	detectRuntimeForTargetFunc = func(context.Context, deployment.ResolvedTarget) (bhruntime.RuntimeProvider, error) {
		return cmf21MeasuredRuntime{}, nil
	}
	withdrawn := resolved
	withdrawn.Manifest = applicationRuntimeRetirementIntent(resolved.Manifest)
	facts, diagnostic := observeApplicationProviderFactsWithDiagnostics(context.Background(), withdrawn)
	if diagnostic != nil {
		t.Fatalf("materialized inventory must not surface a preview diagnostic: %v", diagnostic)
	}
	fact, found := facts[instanceID]
	if !found || !providerFactIsRuntimeMeasurement(fact) || *fact.Containers.Value != 0 {
		t.Fatalf("fixture must yield a measured zero-container runtime fact: %+v ok=%v", fact, found)
	}
	plan, err := prepareApplicationProviderRetirement(context.Background(), withdrawn)
	if err != nil {
		t.Fatalf("measured runtime preparation was rejected: %v", err)
	}
	if !plan.Complete {
		t.Fatalf("measured runtime preparation lost plan completeness: %+v", plan)
	}
	authorized := false
	for _, change := range plan.Changes {
		if change.InstanceID != instanceID {
			continue
		}
		authorized = change.Action == "stop" || change.Action == "remove-runtime"
	}
	if !authorized {
		t.Fatalf("measured runtime did not authorize retirement preparation: %+v", plan.Changes)
	}
}

type cmf21MeasuredRuntime struct {
	bhruntime.RuntimeProvider
}

func (cmf21MeasuredRuntime) Kind() bhruntime.ProviderKind { return "cmf21-measured" }

func (cmf21MeasuredRuntime) InspectProjectResources(_ context.Context, _ string, expected []bhruntime.ProjectResource) ([]bhruntime.ProjectResource, error) {
	return expected, nil
}

func (cmf21MeasuredRuntime) ListRuntimeContainers(context.Context) ([]bhruntime.RuntimeContainer, error) {
	return nil, nil
}
