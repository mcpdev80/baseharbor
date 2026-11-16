package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/objectstorage"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/telemetry"
	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
	"github.com/mcpdev80/baseharbor/internal/traces"
)

type standaloneRetirementFixture struct{ nativeBackendRetirementFixture }

func (f *standaloneRetirementFixture) LogCollectionMode() bhruntime.LogCollectionMode {
	return bhruntime.LogCollectionSyslog
}

func TestStandaloneRetirementPreservesMaterialAndUnselectedModule(t *testing.T) {
	resolved := retainedNativeBackendFixture(t)
	resolved.Manifest = application.WithObjectStorageBuckets(resolved.Manifest, "assets")
	if err := application.ReconcileReferenceProviderRegistryAt(resolved.TargetStateRoot, resolved.Manifest); err != nil {
		t.Fatal(err)
	}
	registry, err := application.PreviewReferenceProviderRegistryAt(resolved.TargetStateRoot, resolved.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	var instance capability.ProviderInstance
	for _, candidate := range registry.Instances {
		if candidate.Provider.Kind == capability.ProviderSeaweedFS {
			instance = candidate
		}
	}
	files, err := objectstorage.EnsureProviderFilesAt(t.Context(), serviceissuer.New(t), resolved.TargetStateRoot, resolved.Target.Name)
	if err != nil {
		t.Fatal(err)
	}
	_, resources, err := objectstorage.ExistingRuntimeInventoryAt(resolved.TargetStateRoot, resolved.Target.Name)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(files.Env)
	remaining := []bhruntime.ProjectResource{}
	for _, resource := range resources {
		if resource.Kind != "container" {
			remaining = append(remaining, resource)
		}
	}
	f := &standaloneRetirementFixture{nativeBackendRetirementFixture{before: resources, after: remaining}}
	var output bytes.Buffer
	if err := retireStandaloneApplicationProvider(t.Context(), resolved, f, instance, registry, &output); err != nil {
		t.Fatal(err)
	}
	if len(f.selected) == 0 {
		t.Fatal("retirement did not select owned module containers")
	}
	for _, resource := range f.selected {
		if resource.Kind != "container" || !strings.Contains(resource.Name, "seaweedfs") {
			t.Fatalf("unselected sibling or material retired: %+v", resource)
		}
	}
	after, _ := os.ReadFile(files.Env)
	if !bytes.Equal(before, after) {
		t.Fatal("retirement regenerated credentials")
	}
	if !strings.Contains(output.String(), files.Dir) {
		t.Fatal("retained final location missing")
	}
	instance.Ownership = capability.OwnershipExternal
	f.selected = nil
	if err := retireStandaloneApplicationProvider(context.Background(), resolved, f, instance, registry, io.Discard); err == nil || len(f.selected) != 0 {
		t.Fatal("foreign owner mutated")
	}
}

func TestCollectorReleasedBindingRecoversOnlyExactCanonicalTraceOwner(t *testing.T) {
	resolved := retainedNativeBackendFixture(t)
	t.Setenv(application.TracesEnabledEnv, "true")
	resolved.Manifest = application.WithOTLPTelemetry(resolved.Manifest, "traces")
	issuer := serviceissuer.New(t)
	_, tracePlacement, err := traces.EnsureProviderFilesAt(t.Context(), issuer, resolved.TargetStateRoot, resolved.Target.Name, resolved.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	files, err := telemetry.EnsureProviderFilesWithTraceBackendForEnvironmentAt(t.Context(), issuer, "http://tempo:4318", tracePlacement.Network, "dev", resolved.TargetStateRoot, resolved.Target.Name)
	if err != nil {
		t.Fatal(err)
	}
	collector := capability.ProviderInstance{ID: "otel/shared", Provider: capability.OTelCollector, Scope: capability.ScopeShared, Ownership: capability.OwnershipBaseHarbor}
	trace := capability.ProviderInstance{ID: "tempo/shared", Provider: capability.Tempo, Scope: capability.ScopeShared, Ownership: capability.OwnershipBaseHarbor}
	registry := capability.NewRegistry()
	registry.Instances = []capability.ProviderInstance{collector, trace}
	network, dependency, err := retainedCollectorTraceBinding(resolved, collector, registry)
	if err != nil || network != tracePlacement.Network || dependency != trace.ID {
		t.Fatalf("released binding lost protected dependency: %s %s %v", network, dependency, err)
	}
	duplicate := trace
	duplicate.ID = "tempo/ambiguous"
	registry.Instances = append(registry.Instances, duplicate)
	if _, _, err := retainedCollectorTraceBinding(resolved, collector, registry); err == nil {
		t.Fatal("ambiguous canonical module admitted")
	}
	registry.Instances = []capability.ProviderInstance{collector, trace}
	data, _ := os.ReadFile(files.Compose)
	if err := os.WriteFile(files.Compose, append(data, []byte("\n# changed binding\n")...), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := retainedCollectorTraceBinding(resolved, collector, registry); err == nil {
		t.Fatal("modified Collector binding reconstructed as authoritative")
	}
}
