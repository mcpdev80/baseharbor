package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/telemetry"
	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
	"github.com/mcpdev80/baseharbor/internal/traces"
)

func TestRetirementConvergesAcknowledgedUnusedCollectorWithoutNewBindingRelease(t *testing.T) {
	resolved := retainedNativeBackendFixture(t)
	t.Setenv(application.TracesEnabledEnv, "true")
	prior := resolved.Manifest
	resolved.Manifest = application.WithOTLPTelemetry(prior, "traces")
	if err := recordAppliedDeployment(context.Background(), resolved, application.RuntimeFiles{}); err != nil {
		t.Fatal(err)
	}
	if err := application.ReconcileReferenceProviderRegistryAt(resolved.TargetStateRoot, resolved.Manifest, capability.Resource{Application: resolved.Manifest.Name, Kind: capability.Traces, Name: "default", Provider: capability.ProviderTempo}); err != nil {
		t.Fatal(err)
	}
	issuer := serviceissuer.New(t)
	_, placement, err := traces.EnsureProviderFilesAt(t.Context(), issuer, resolved.TargetStateRoot, resolved.Target.Name, resolved.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	files, err := telemetry.EnsureProviderFilesWithTraceBackendForEnvironmentAt(t.Context(), issuer, "http://tempo:4318", placement.Network, "dev", resolved.TargetStateRoot, resolved.Target.Name)
	if err != nil {
		t.Fatal(err)
	}
	_, resources, err := telemetry.ExistingRuntimeInventoryAt(resolved.TargetStateRoot, resolved.Target.Name, placement.Network)
	if err != nil {
		t.Fatal(err)
	}
	if err := application.ReleaseApplicationProviderReferencesAt(resolved.TargetStateRoot, resolved.Manifest); err != nil {
		t.Fatal(err)
	}
	registry, err := (capability.RegistryStore{Path: filepath.Join(resolved.TargetStateRoot, "provider-registry.json")}).Load()
	if err != nil {
		t.Fatal(err)
	}
	id := ""
	for _, instance := range registry.Instances {
		if instance.Provider.Kind == capability.ProviderOTelCollector {
			id = instance.ID
		}
	}
	if id == "" || len(registry.BindingsForProviderInstance(id)) != 0 {
		t.Fatal("fixture lacks previously released Collector owner")
	}
	t.Setenv(application.TracesEnabledEnv, "")
	canonicalOwner := resolved
	resolved.Manifest = prior
	retained := []bhruntime.ProjectResource{}
	for _, r := range resources {
		if r.Kind != "container" {
			retained = append(retained, r)
		}
	}
	runtime := &standaloneRetirementFixture{nativeBackendRetirementFixture{before: resources, after: retained}}
	for _, instance := range registry.Instances {
		if instance.ID == id {
			if _, err := existingStandaloneProviderInventory(canonicalOwner, runtime, instance, registry); err != nil {
				t.Fatal("fixture lacks canonical Collector authority", err)
			}
		}
	}
	before, err := os.ReadFile(files.Env)
	if err != nil {
		t.Fatal(err)
	}
	plan := application.ProviderPlan{Version: application.ProviderPlanVersion, Target: resolved.Target.Name, Complete: true, Changes: []application.ProviderChange{{InstanceID: id, Action: "stop", Ownership: capability.OwnershipBaseHarbor, Consumers: []string{}}}}
	var output bytes.Buffer
	if err := retireReleasedApplicationProviders(t.Context(), resolved, runtime, nil, plan, &output); err != nil {
		t.Fatal(err)
	}
	if len(runtime.selected) == 0 {
		t.Fatal("acknowledged unused Collector remains running because its binding was released earlier")
	}
	for _, r := range runtime.selected {
		if r.Kind != "container" {
			t.Fatal("retirement selected persistent material")
		}
	}
	after, err := os.ReadFile(files.Env)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("retirement changed protected credentials")
	}
	for _, kind := range []string{"incomplete", "foreign Target", "foreign owner", "active consumer", "required dependency", "wrong protocol"} {
		t.Run(kind, func(t *testing.T) {
			bad := plan
			bad.Changes = append([]application.ProviderChange(nil), plan.Changes...)
			switch kind {
			case "incomplete":
				bad.Complete = false
			case "foreign Target":
				bad.Target = "foreign"
			case "foreign owner":
				bad.Changes[0].Ownership = capability.OwnershipExternal
			case "active consumer":
				bad.Changes[0].Consumers = []string{"application/sibling/dev"}
			case "required dependency":
				bad.Required = []string{id}
			case "wrong protocol":
				bad.Version = "unknown"
			}
			runtime.selected = nil
			runtime.inspections = 0
			if err := retireReleasedApplicationProviders(t.Context(), resolved, runtime, nil, bad, &output); err == nil || len(runtime.selected) != 0 {
				t.Fatal("unverified retirement was admitted or mutated runtime")
			}
		})
	}
	// A concurrent/new consumer is not authority merely because an older plan
	// still says the module is unused. Its actual binding must be released too.
	if err := application.ReconcileReferenceProviderRegistryAt(resolved.TargetStateRoot, canonicalOwner.Manifest, capability.Resource{Application: canonicalOwner.Manifest.Name, Kind: capability.Traces, Name: "default", Provider: capability.ProviderTempo}); err != nil {
		t.Fatal(err)
	}
	runtime.selected = nil
	runtime.inspections = 0
	if err := retireReleasedApplicationProviders(t.Context(), resolved, runtime, nil, plan, &output); err == nil || len(runtime.selected) != 0 {
		t.Fatal("new unreleased binding was ignored")
	}
}
