package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/externalprovider"
	"github.com/mcpdev80/baseharbor/internal/machine"
)

func corelessResolvedFixture(t *testing.T) resolvedApplication {
	t.Helper()
	target := configureTestTarget(t)
	root := t.TempDir()
	m := application.WithWorkloadComponents(application.New("plain", "dev", false, false, false), "api")
	if err := os.WriteFile(filepath.Join(root, "compose.yaml"), []byte("services:\n  api:\n    image: alpine:3.22\n    command: [sleep, infinity]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	state, err := deployment.TargetStateRoot(target.Name)
	if err != nil {
		t.Fatal(err)
	}
	return resolvedApplication{Target: target, TargetStateRoot: state, Manifest: m, RepositoryRoot: root, ManifestPath: filepath.Join(root, "baseharbor.yaml"), FromRepository: true}
}

func TestCorelessDecisionNeverInvokesCoreBootstrap(t *testing.T) {
	resolved := corelessResolvedFixture(t)
	previous := applicationCorePrerequisite
	applicationCorePrerequisite = func(context.Context, io.Reader, io.Writer) error {
		t.Fatal("plain workload invoked Core provisioning")
		return nil
	}
	t.Cleanup(func() { applicationCorePrerequisite = previous })
	if err := requireResolvedApplicationCore(machineNoninteractiveContext(context.Background()), resolved, strings.NewReader(""), io.Discard); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(resolved.TargetStateRoot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("read-only decision wrote Target state: %v", err)
	}
}

func TestCoreDecisionMissingSourceFailsBeforeBootstrap(t *testing.T) {
	resolved := corelessResolvedFixture(t)
	if err := os.Remove(filepath.Join(resolved.RepositoryRoot, "compose.yaml")); err != nil {
		t.Fatal(err)
	}
	previous := applicationCorePrerequisite
	applicationCorePrerequisite = func(context.Context, io.Reader, io.Writer) error { t.Fatal("missing source invoked Core"); return nil }
	t.Cleanup(func() { applicationCorePrerequisite = previous })
	if err := requireResolvedApplicationCore(context.Background(), resolved, strings.NewReader(""), io.Discard); err == nil {
		t.Fatal("missing workload source accepted")
	}
}

func TestSelectiveSQLDecisionNonTTYYesDoesNotBootstrap(t *testing.T) {
	resolved := corelessResolvedFixture(t)
	resolved.Manifest.Services.SQL = true
	previous := applicationCoreBootstrap
	applicationCoreBootstrap = func(context.Context, io.Reader, io.Writer, runtimeUpOptions) (coreinstallation.State, error) {
		t.Fatal("non-TTY app --yes installed Core")
		return coreinstallation.State{}, nil
	}
	t.Cleanup(func() { applicationCoreBootstrap = previous })
	var out bytes.Buffer
	err := requireResolvedApplicationCore(withAssumeYes(machineNoninteractiveContext(context.Background()), true), resolved, strings.NewReader(""), &out)
	if err != nil {
		t.Fatalf("SQL-only unexpectedly requires Core: %v", err)
	}
	root, err := targetRuntimeStateRoot(resolved.Target)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Core state materialized without installation consent: %v", err)
	}
}

func TestCapabilityRemovalPreservesSharedProviderOwnerAndOtherApp(t *testing.T) {
	resolved := corelessResolvedFixture(t)
	first := resolved.Manifest
	first.Services.SQL = true
	second := application.New("other", "dev", true, false, false)
	if err := application.ReconcileReferenceProviderRegistryAt(resolved.TargetStateRoot, first); err != nil {
		t.Fatal(err)
	}
	if err := application.ReconcileReferenceProviderRegistryAt(resolved.TargetStateRoot, second); err != nil {
		t.Fatal(err)
	}
	if err := application.ReconcileReferenceProviderRegistryAt(resolved.TargetStateRoot, resolved.Manifest); err != nil {
		t.Fatal(err)
	}
	store := capability.RegistryStore{Path: filepath.Join(resolved.TargetStateRoot, "provider-registry.json")}
	registry, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(registry.Instances) != 1 || len(registry.Bindings) != 1 || registry.Bindings[0].ApplicationID != second.ApplicationID {
		t.Fatalf("shared ownership/consumer lost: %+v", registry)
	}
	second.Services.SQL = false
	second = application.WithWorkloadComponents(second, "api")
	if err := application.ReconcileReferenceProviderRegistryAt(resolved.TargetStateRoot, second); err != nil {
		t.Fatal(err)
	}
	registry, err = store.Load()
	if err != nil || len(registry.Instances) != 1 || len(registry.Bindings) != 0 {
		t.Fatalf("last capability removal reclaimed provider ownership: %+v %v", registry, err)
	}
}

func TestCoreDecisionCorruptProviderRegistryBeforeMutation(t *testing.T) {
	resolved := corelessResolvedFixture(t)
	resolved.Manifest.Services.SQL = true
	if err := os.MkdirAll(resolved.TargetStateRoot, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(resolved.TargetStateRoot, "provider-registry.json")
	original := []byte("{broken")
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	previous := applicationCorePrerequisite
	applicationCorePrerequisite = func(context.Context, io.Reader, io.Writer) error {
		t.Fatal("corrupt registry invoked Core provisioning")
		return nil
	}
	t.Cleanup(func() { applicationCorePrerequisite = previous })
	if err := requireResolvedApplicationCore(context.Background(), resolved, strings.NewReader(""), io.Discard); err == nil {
		t.Fatal("corrupt registry accepted")
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(original) {
		t.Fatal("rejected registry was mutated")
	}
}

func TestCoreDecisionUnsupportedBYOBeforeMutation(t *testing.T) {
	resolved := corelessResolvedFixture(t)
	resolved.Manifest.Services.SQL = true
	resolved.FromRepository = false
	if err := application.RegisterExternalProviderAt(resolved.TargetStateRoot, externalprovider.Registration{
		ID: "existing-byo", ProviderID: "company/postgresql", Endpoint: "postgres://db.example:5432/app",
		CredentialRef: "file:///run/secrets/company-db.json",
		Provider:      capability.Provider{Kind: capability.ProviderPostgreSQL, Capabilities: []capability.Kind{capability.SQL}},
		Trust:         externalprovider.Trust{Mode: externalprovider.TrustSystem},
	}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(resolved.TargetStateRoot, "provider-registry.json")
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(application.ProviderScopeEnv(capability.ProviderPostgreSQL), "external")
	t.Setenv(application.ProviderExternalReferenceEnv(capability.ProviderPostgreSQL), "existing-byo")
	previous := applicationCorePrerequisite
	applicationCorePrerequisite = func(context.Context, io.Reader, io.Writer) error {
		t.Fatal("unsupported BYO invoked Core provisioning")
		return nil
	}
	t.Cleanup(func() { applicationCorePrerequisite = previous })
	if err := requireResolvedApplicationCore(context.Background(), resolved, strings.NewReader(""), io.Discard); err != nil {
		t.Fatalf("unsupported native BYO suppressed read-only planning: %v", err)
	}
	plan, err := buildResolvedApplicationPlan(context.Background(), resolved)
	if err != nil {
		t.Fatalf("registered BYO could not be planned: %v", err)
	}
	if plan.ProviderPlan.Impact != nil {
		t.Fatal("first selection invented an existing consumer to replace")
	}
	err = authorizeApplicationProviderChanges(context.Background(), resolved, nil, io.Discard)
	var failure *machine.Error
	if !errors.As(err, &failure) || failure.CauseCode != "external_provider_adapter_unavailable" {
		t.Fatalf("unsupported BYO execution crossed authority or lost its concrete plan: %v", err)
	}
	if after, err := os.ReadFile(path); err != nil || !bytes.Equal(original, after) {
		t.Fatalf("unsupported BYO changed its registered owner: %v", err)
	}
}

func TestCapabilityRemovalThenOtherDestroyRetainsUnboundSharedOwnership(t *testing.T) {
	resolved := corelessResolvedFixture(t)
	m := resolved.Manifest
	m.Services.SQL = true
	if err := application.ReconcileReferenceProviderRegistryAt(resolved.TargetStateRoot, m); err != nil {
		t.Fatal(err)
	}
	if err := application.ReconcileReferenceProviderRegistryAt(resolved.TargetStateRoot, resolved.Manifest); err != nil {
		t.Fatal(err)
	}
	if err := application.ReleaseApplicationProviderRegistryAt(resolved.TargetStateRoot, resolved.Manifest); err != nil {
		t.Fatal(err)
	}
	registry, err := (capability.RegistryStore{Path: filepath.Join(resolved.TargetStateRoot, "provider-registry.json")}).Load()
	if err != nil || len(registry.Instances) != 1 || len(registry.Bindings) != 0 {
		t.Fatalf("explicit app destroy reclaimed target-owned provider inventory: %+v %v", registry, err)
	}
	footprint, err := resolveApplicationLifecycleFootprint(resolved)
	if err != nil || len(footprint.Unused) != 2 {
		t.Fatalf("retained unused provider invisible: %+v %v", footprint, err)
	}
}

func TestFootprintPlanDoesNotClaimProjectedRegistrationIsLiveOrBound(t *testing.T) {
	resolved := corelessResolvedFixture(t)
	resolved.Manifest.Services.SQL = true
	plan, err := buildResolvedApplicationPlan(context.Background(), resolved)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Footprint.Requirements) != 1 || plan.Footprint.Requirements[0].Binding != "requested/unbound" || len(plan.Footprint.Additional) != 2 {
		t.Fatalf("invented live footprint: %+v", plan.Footprint)
	}
	for _, provider := range plan.Footprint.Additional {
		if provider.State != application.ProviderNewRequired || provider.Containers.Classification != "unknown" {
			t.Fatalf("projected binding/module claimed live: %+v", provider)
		}
	}
}

func TestUnverifiedConsumptionDoesNotInstallCore(t *testing.T) {
	resolved := corelessResolvedFixture(t)
	resolved.Manifest.Consumes = []application.ConsumptionRequirement{{Name: "api", Component: "api", Interface: "http"}}
	previous := applicationCorePrerequisite
	applicationCorePrerequisite = func(context.Context, io.Reader, io.Writer) error {
		t.Fatal("unknown consumption provisioned Core")
		return nil
	}
	t.Cleanup(func() { applicationCorePrerequisite = previous })
	err := requireResolvedApplicationCore(context.Background(), resolved, strings.NewReader(""), io.Discard)
	var failure *machine.Error
	if !errors.As(err, &failure) || failure.CauseCode != "provider_dependencies_unverifiable" {
		t.Fatalf("unknown dependencies authorized mutation: %v", err)
	}
	if _, err := os.Stat(resolved.TargetStateRoot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unknown consumption mutated Target: %v", err)
	}
}

func TestSelectiveSQLDecisionReaderWithoutTTYDoesNotBootstrap(t *testing.T) {
	resolved := corelessResolvedFixture(t)
	resolved.Manifest.Services.SQL = true
	previous := applicationCoreBootstrap
	applicationCoreBootstrap = func(context.Context, io.Reader, io.Writer, runtimeUpOptions) (coreinstallation.State, error) {
		t.Fatal("non-TTY reader installed Core without noninteractive flag")
		return coreinstallation.State{}, nil
	}
	t.Cleanup(func() { applicationCoreBootstrap = previous })
	err := requireResolvedApplicationCore(withAssumeYes(context.Background(), true), resolved, strings.NewReader(""), io.Discard)
	if err != nil {
		t.Fatalf("SQL-only unexpectedly requires Core: %v", err)
	}
}
