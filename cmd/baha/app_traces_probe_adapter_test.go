package main

import (
	"context"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/observability"
)

// The managed trace verification of a workload application used to fail with
// "native OTLP trace source prometheus:<project> has no runtime probe adapter"
// because the workload's native trace consumer was registered against the
// Prometheus metrics identity. These cases pin the adapter contract: receiver
// owned sources route to an OTLP export, runtime components route to their own
// probe endpoint, and any remaining pairing fails with a message that names
// what is wrong instead of an unconditional missing-adapter dead end.
func TestNativeTraceSourceProbeAdapterRouting(t *testing.T) {
	prepared := &managedTracesExecution{
		manifest: application.Manifest{Version: 1, Name: "probe-routing", Environment: "dev", Workload: application.WorkloadConfig{Components: []string{"api"}}},
	}
	prepared.runtimeFiles = application.RuntimeFilesFor(application.Store{}, prepared.manifest)
	project := prepared.runtimeFiles.Project
	source := func(provider capability.ProviderKind) observability.SignalSource {
		return observability.SignalSource{
			ID:                 string(provider) + ":" + project,
			Kind:               observability.SignalTraces,
			Provider:           provider,
			Class:              observability.SourceApplicationProvider,
			Scope:              capability.ScopeApplication,
			OwnerApplication:   prepared.manifest.Name,
			Target:             observability.RuntimeTarget(project, "workload"),
			Protocol:           "otlp",
			Mode:               capability.ObservabilityNative,
			SemanticConvention: "http",
			Verification:       capability.ObservabilityVerifySpan,
		}
	}

	for _, receiver := range []capability.ProviderKind{capability.ProviderOTelCollector, capability.ProviderExternalOTLP} {
		t.Run(string(receiver), func(t *testing.T) {
			// No runtime is touched here: the receiver route exports over the
			// application's own binding, so the only possible failure is the
			// missing binding this isolated execution intentionally lacks.
			_, err := nativeTraceSourceProbe(context.Background(), prepared, source(receiver))
			if err == nil {
				t.Fatal("expected the unbound receiver route to fail closed")
			}
			if strings.Contains(err.Error(), "has no runtime probe adapter") {
				t.Fatalf("%s trace source reported a missing probe adapter: %v", receiver, err)
			}
			if !strings.Contains(err.Error(), "binding") {
				t.Fatalf("%s trace source failed for an unrelated reason: %v", receiver, err)
			}
		})
	}

	// The metrics backend receives no spans: the mismatch must be visible.
	_, err := nativeTraceSourceProbe(context.Background(), prepared, source(capability.ProviderPrometheus))
	if err == nil {
		t.Fatal("Prometheus trace source unexpectedly routed")
	}
	if !strings.Contains(err.Error(), `"prometheus"`) || !strings.Contains(err.Error(), "no runtime probe adapter") {
		t.Fatalf("Prometheus trace source failed without naming the receiver mismatch: %v", err)
	}

	for _, component := range []capability.ProviderKind{capability.ProviderRuntimeBroker, capability.ProviderRuntimeExecutor} {
		t.Run(string(component), func(t *testing.T) {
			_, err := nativeTraceSourceProbe(context.Background(), prepared, source(component))
			if err == nil {
				t.Fatalf("%s trace source unexpectedly succeeded without a runtime", component)
			}
			if strings.Contains(err.Error(), "has no runtime probe adapter") {
				t.Fatalf("%s trace source lost its probe adapter: %v", component, err)
			}
		})
	}
}
