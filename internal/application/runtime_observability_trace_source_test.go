package application

import (
	"fmt"
	"os"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/observability"
)

// writeTestRuntimeOTLPBinding materializes the same protected runtime.env
// projection the OTLP driver Bind writes, so registry resolution reads the
// receiver identity through its production reader.
func writeTestRuntimeOTLPBinding(t *testing.T, m Manifest, provider, hostEndpoint, containerEndpoint string) {
	t.Helper()
	// A registered external receiver is selected by the deployment's operator
	// policy, exactly as TelemetryProviderForDeployment reads it.
	if provider == string(capability.ProviderExternalOTLP) {
		t.Setenv(ProviderExternalReferenceEnv(capability.ProviderExternalOTLP), "test-receiver")
	} else {
		t.Setenv(ProviderExternalReferenceEnv(capability.ProviderExternalOTLP), "")
		t.Setenv(OTLPEndpointEnv, "")
		t.Setenv(OTLPProviderEnv, "")
	}
	files := RuntimeFilesFor(Store{}, m)
	if err := os.MkdirAll(files.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	content := fmt.Sprintf("OTLP_PROVIDER=%s\nOTLP_HOST_ENDPOINT=%s\nOTLP_CONTAINER_ENDPOINT=%s\n", provider, hostEndpoint, containerEndpoint)
	if err := os.WriteFile(files.Env, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// The runtime-component reconcile is the production caller of the
// provider-explicit reconciliation: with every collection policy disabled its
// desired set is empty, and it must still reclaim the component's stale
// registrations under its own provider. A shared registration another provider
// owns under the SAME ID stays alive through that transition.
func TestRuntimeComponentReconcileEmptyDesiredKeepsForeignSameID(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	m := workloadOnlyTraceManifest("component-sameid")
	componentID := "shared-collector:component-sameid"
	foreign := observability.SignalSource{
		ID: componentID, Kind: observability.SignalTraces,
		Provider: capability.ProviderOTelCollector, Class: observability.SourcePlatformProvider,
		Scope: capability.ScopeShared, Target: "foreign", Protocol: "otlp",
		Mode: capability.ObservabilityNative, Verification: capability.ObservabilityVerifyNone,
	}
	if err := observability.UpdateSignal(foreign); err != nil {
		t.Fatal(err)
	}
	component := RuntimeComponentObservability{
		ID: componentID, Provider: capability.ProviderRuntimeBroker,
		Class: observability.SourceApplicationProvider, Scope: capability.ScopeApplication,
		OwnerApplication: m.Name,
		MetricsNetwork:   "baseharbor-test-net", MetricsTarget: "runtime://collector/metrics", MetricsPath: "/metrics",
		LogsTarget: "runtime://collector/logs", TracesTarget: "runtime://collector",
	}
	// Enabled policies register the component's signals alongside the foreign
	// same-ID trace registration.
	if err := ReconcileRuntimeComponentObservability(m, component); err != nil {
		t.Fatal(err)
	}
	live, err := observability.RegistryHasSignal(componentID, capability.ProviderRuntimeBroker, observability.SignalTraces)
	if err != nil {
		t.Fatal(err)
	}
	if !live {
		t.Fatal("fixture: enabled policies must register the component's trace signal")
	}

	// Disabling every collection policy empties the desired set: the stale
	// component registrations must go, the foreign provider's signal must not.
	t.Setenv(MetricsEnabledEnv, "false")
	t.Setenv(LogsEnabledEnv, "false")
	t.Setenv(TracesEnabledEnv, "false")
	if err := ReconcileRuntimeComponentObservability(m, component); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []observability.SignalKind{observability.SignalMetrics, observability.SignalLogs, observability.SignalTraces} {
		live, err := observability.RegistryHasSignal(componentID, capability.ProviderRuntimeBroker, kind)
		if err != nil {
			t.Fatal(err)
		}
		if live {
			t.Fatalf("disabled policies left the component's stale %v signal registered", kind)
		}
	}
	live, err = observability.RegistryHasSignal(componentID, capability.ProviderOTelCollector, observability.SignalTraces)
	if err != nil {
		t.Fatal(err)
	}
	if !live {
		t.Fatal("empty-desired component reconciliation co-deleted a foreign provider's same-ID signal")
	}
}

func workloadOnlyTraceManifest(name string) Manifest {
	m := WithWorkloadComponents(New(name, "dev", false, false, false), "api")
	return WithOTLPTelemetry(m, "traces")
}

func assertTraceSourceProviders(t *testing.T, m Manifest, want map[string]capability.ProviderKind) {
	t.Helper()
	placement, err := ResolveProviderPlacement(m, capability.ProviderTempo)
	if err != nil {
		t.Fatal(err)
	}
	sources, err := observability.ListTraces(placement, []string{m.Name}, true, true)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]capability.ProviderKind{}
	for _, source := range sources {
		if source.Kind != observability.SignalTraces {
			t.Fatalf("ListTraces returned %s signal %q", source.Kind, source.ID)
		}
		if source.Protocol != "otlp" && source.Protocol != "interaction" {
			t.Fatalf("trace source %q has protocol %q", source.ID, source.Protocol)
		}
		got[source.ID] = source.Provider
	}
	for id, wantProvider := range want {
		if got[id] != wantProvider {
			t.Fatalf("trace source %q provider = %q, want %q (all: %v)", id, got[id], wantProvider, got)
		}
	}
	for id, provider := range got {
		if provider == capability.ProviderPrometheus {
			t.Fatalf("trace source %q is attributed to the Prometheus metrics backend, which receives no OTLP spans", id)
		}
	}
}

// The workload trace consumer must never be attributed to the Prometheus
// identity: that pairing produced "native OTLP trace source prometheus:<project>
// has no runtime probe adapter" on every managed trace apply of a workload app
// with an OTLP trace signal.
func TestWorkloadTraceSourceNeverAttributedToPrometheus(t *testing.T) {
	for _, tc := range []struct{ name, provider, host, container string }{
		{"managed-collector", string(capability.ProviderOTelCollector), "https://127.0.0.1:43183", "https://otel-collector:4318"},
		{"registered-external", string(capability.ProviderExternalOTLP), "https://receiver.example.test:43189", "https://receiver.example.test:43189"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
			m := workloadOnlyTraceManifest("trace-owner-" + tc.name)
			writeTestRuntimeOTLPBinding(t, m, tc.provider, tc.host, tc.container)
			project := RuntimeProjectName(m)
			if err := reconcileManagedRuntimeObservability(m, project); err != nil {
				t.Fatal(err)
			}
			assertTraceSourceProviders(t, m, map[string]capability.ProviderKind{
				fmt.Sprintf("%s:%s", tc.provider, project): capability.ProviderKind(tc.provider),
			})
			prometheusID := fmt.Sprintf("%s:%s", capability.ProviderPrometheus, project)
			placement, err := ResolveProviderPlacement(m, capability.ProviderPrometheus)
			if err != nil {
				t.Fatal(err)
			}
			metrics, err := observability.ListMetrics(placement, []string{m.Name}, true, true)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, source := range metrics {
				if source.ID == prometheusID {
					found = true
				}
			}
			if !found {
				t.Fatalf("Prometheus workload identity %q must remain registered for metrics: %+v", prometheusID, metrics)
			}
		})
	}
}

// Replacing the managed collector with a registered external OTLP source must
// move the workload trace consumer to that receiver, and returning to the
// managed transport must move it back. Stale receiver-owned identities cannot
// survive either direction: verification would then export against a receiver
// the application no longer uses.
func TestWorkloadTraceSourceFollowsTelemetryReplacement(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	m := workloadOnlyTraceManifest("trace-handoff")
	project := RuntimeProjectName(m)

	writeTestRuntimeOTLPBinding(t, m, string(capability.ProviderOTelCollector), "https://127.0.0.1:43183", "https://otel-collector:4318")
	if err := reconcileManagedRuntimeObservability(m, project); err != nil {
		t.Fatal(err)
	}
	collectorID := fmt.Sprintf("%s:%s", capability.ProviderOTelCollector, project)
	assertTraceSourceProviders(t, m, map[string]capability.ProviderKind{collectorID: capability.ProviderOTelCollector})

	externalEndpoint := "https://receiver.example.test:43189"
	writeTestRuntimeOTLPBinding(t, m, string(capability.ProviderExternalOTLP), externalEndpoint, externalEndpoint)
	if err := reconcileManagedRuntimeObservability(m, project); err != nil {
		t.Fatal(err)
	}
	externalID := fmt.Sprintf("%s:%s", capability.ProviderExternalOTLP, project)
	sources, err := observability.ListTraces(mustPlacement(t, m), []string{m.Name}, true, true)
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]capability.ProviderKind{}
	for _, source := range sources {
		ids[source.ID] = source.Provider
	}
	if _, stale := ids[collectorID]; stale {
		t.Fatalf("collector trace source survived external replacement: %v", ids)
	}
	if ids[externalID] != capability.ProviderExternalOTLP {
		t.Fatalf("external replacement did not register its trace consumer: %v", ids)
	}

	writeTestRuntimeOTLPBinding(t, m, string(capability.ProviderOTelCollector), "https://127.0.0.1:43183", "https://otel-collector:4318")
	if err := reconcileManagedRuntimeObservability(m, project); err != nil {
		t.Fatal(err)
	}
	sources, err = observability.ListTraces(mustPlacement(t, m), []string{m.Name}, true, true)
	if err != nil {
		t.Fatal(err)
	}
	ids = map[string]capability.ProviderKind{}
	for _, source := range sources {
		ids[source.ID] = source.Provider
	}
	if _, stale := ids[externalID]; stale {
		t.Fatalf("external trace source survived return to the managed collector: %v", ids)
	}
	if ids[collectorID] != capability.ProviderOTelCollector {
		t.Fatalf("return did not re-register the collector trace consumer: %v", ids)
	}
}

func mustPlacement(t *testing.T, m Manifest) capability.ProviderPlacement {
	t.Helper()
	placement, err := ResolveProviderPlacement(m, capability.ProviderTempo)
	if err != nil {
		t.Fatal(err)
	}
	return placement
}

func traceSources(t *testing.T, m Manifest) []observability.SignalSource {
	t.Helper()
	sources, err := observability.ListTraces(mustPlacement(t, m), []string{m.Name}, true, true)
	if err != nil {
		t.Fatal(err)
	}
	return sources
}

func traceIDs(sources []observability.SignalSource) map[string]capability.ProviderKind {
	ids := map[string]capability.ProviderKind{}
	for _, source := range sources {
		ids[source.ID] = source.Provider
	}
	return ids
}

// Losing the OTLP receiver (the external reference is unset without a managed
// transport returning in the same apply, or a binding is retired) must reclaim
// every receiver-owned trace identity of that workload: the collector
// registration and the external registration alike. A surviving identity would
// keep verification pointed at a receiver the application no longer binds to.
func TestWorkloadTraceSourceReclaimsBothReceiversWhenUnbound(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	m := workloadOnlyTraceManifest("trace-unbound-prune")
	project := RuntimeProjectName(m)

	// Seed the two receiver identities a workload can accumulate over its
	// lifetime, and assert the state each step must be in. The registry keeps
	// at most one trace signal per source ID, so a receiver transition replaces
	// the identity rather than adding to it; the unbind step below is where
	// both kinds must be reclaimed by a single call.
	collectorID := fmt.Sprintf("%s:%s", capability.ProviderOTelCollector, project)
	externalID := fmt.Sprintf("%s:%s", capability.ProviderExternalOTLP, project)
	writeTestRuntimeOTLPBinding(t, m, string(capability.ProviderOTelCollector), "https://127.0.0.1:43183", "https://otel-collector:4318")
	if err := reconcileManagedRuntimeObservability(m, project); err != nil {
		t.Fatal(err)
	}
	if got := traceIDs(traceSources(t, m)); got[collectorID] != capability.ProviderOTelCollector {
		t.Fatalf("managed apply did not register the collector identity: %v", got)
	}
	external := "https://receiver.example.test:43189"
	writeTestRuntimeOTLPBinding(t, m, string(capability.ProviderExternalOTLP), external, external)
	if err := reconcileManagedRuntimeObservability(m, project); err != nil {
		t.Fatal(err)
	}
	if got := traceIDs(traceSources(t, m)); got[externalID] != capability.ProviderExternalOTLP {
		t.Fatalf("external replacement did not move the identity: %v", got)
	}
	// A registry left inconsistent by an interrupted mutation is the state the
	// unbind reconciliation must clean up in one call.
	if err := observability.UpdateSignal(workloadReceiverTraceSource(m, project, capability.ProviderOTelCollector)); err != nil {
		t.Fatal(err)
	}
	if got := traceIDs(traceSources(t, m)); got[collectorID] == "" || got[externalID] == "" {
		t.Fatalf("fixture did not accumulate both receiver identities: %v", got)
	}

	// Returning to the managed transport clears the external reference. That is
	// the state in which both receiver identities must be gone: the current one
	// is re-registered by the reconciliation itself, and the retired one must
	// not linger for verification to reach.
	t.Setenv(ProviderExternalReferenceEnv(capability.ProviderExternalOTLP), "")
	t.Setenv(OTLPEndpointEnv, "")
	t.Setenv(OTLPProviderEnv, "")
	if err := reconcileManagedRuntimeObservability(m, project); err != nil {
		t.Fatal(err)
	}
	got := traceIDs(traceSources(t, m))
	if _, stale := got[externalID]; stale {
		t.Fatalf("external trace identity survived the return to the managed collector: %v", got)
	}
	if got[collectorID] != capability.ProviderOTelCollector {
		t.Fatalf("return to the managed collector did not register its identity: %v", got)
	}
}

// Removing the OTLP trace intent after a managed deployment must reclaim the
// collector identity that earlier apply registered: no other cleanup path keys
// on it once the manifest no longer requests OTLP delivery.
func TestWorkloadTraceSignalReclaimedWhenOTLPIntentRemoved(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	m := workloadOnlyTraceManifest("trace-intent-off")
	project := RuntimeProjectName(m)
	writeTestRuntimeOTLPBinding(t, m, string(capability.ProviderOTelCollector), "https://127.0.0.1:43183", "https://otel-collector:4318")
	if err := reconcileManagedRuntimeObservability(m, project); err != nil {
		t.Fatal(err)
	}
	collectorID := fmt.Sprintf("%s:%s", capability.ProviderOTelCollector, project)
	if got := traceIDs(traceSources(t, m)); got[collectorID] != capability.ProviderOTelCollector {
		t.Fatalf("fixture did not register the collector identity: %v", got)
	}

	// The authored manifest drops the telemetry block entirely.
	trimmed := m
	trimmed.Telemetry = TelemetryRequirements{}
	if HasOTLPTelemetry(trimmed) {
		t.Fatal("fixture must no longer declare OTLP intent")
	}
	if err := reconcileApplicationTraceSource(trimmed, project, false); err != nil {
		t.Fatal(err)
	}
	if got := traceIDs(traceSources(t, trimmed)); got[collectorID] != "" {
		t.Fatalf("collector trace identity survived the OTLP intent removal: %v", got)
	}
}

// Receiver identities are project-scoped, so disabling trace collection in one
// environment must not reclaim another environment's trace verification.
func TestReceiverTracePruneIsScopedToReconciledProject(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	dev := WithWorkloadComponents(New("trace-scope", "dev", false, false, false), "api")
	dev = WithOTLPTelemetry(dev, "traces")
	prod := Manifest{Version: 1, ApplicationID: dev.ApplicationID, Name: dev.Name, Environment: "prod", Workload: dev.Workload, Telemetry: dev.Telemetry}

	// Seed both environments' collector identities, as the two applies own them.
	for _, m := range []Manifest{dev, prod} {
		if err := observability.UpdateSignal(workloadReceiverTraceSource(m, RuntimeProjectName(m), capability.ProviderOTelCollector)); err != nil {
			t.Fatal(err)
		}
	}
	devID := fmt.Sprintf("%s:%s", capability.ProviderOTelCollector, RuntimeProjectName(dev))
	prodID := fmt.Sprintf("%s:%s", capability.ProviderOTelCollector, RuntimeProjectName(prod))
	if got := traceIDs(traceSources(t, dev)); got[devID] == "" || got[prodID] == "" {
		t.Fatalf("fixture did not register both environments: %v", got)
	}

	// Disable provider trace collection for the dev deployment only.
	t.Setenv(TracesEnabledEnv, "false")
	if err := reconcileApplicationTraceSource(dev, RuntimeProjectName(dev), false); err != nil {
		t.Fatal(err)
	}
	got := traceIDs(traceSources(t, dev))
	if got[devID] != "" {
		t.Fatalf("dev trace identity survived its own disabled policy: %v", got)
	}
	if got[prodID] != capability.ProviderOTelCollector {
		t.Fatalf("disabling dev reclaimed the prod trace identity: %v", got)
	}
}

// Receiver-owned workload sources of other applications must stay registered
// while this workload transitions: the prune is exact-identity, never by
// provider attribute.
func TestWorkloadTracePruneKeepsForeignSources(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	foreign := observability.SignalSource{
		ID:       "opentelemetry-collector:" + RuntimeProjectName(workloadOnlyTraceManifest("foreign-owner")),
		Kind:     observability.SignalTraces,
		Provider: capability.ProviderOTelCollector,
		Class:    observability.SourcePlatformProvider,
		Scope:    capability.ScopeShared,
		Target:   "collector", Protocol: "otlp",
		Mode:         capability.ObservabilityNative,
		Verification: capability.ObservabilityVerifySpan,
	}
	if err := observability.UpdateSignal(foreign); err != nil {
		t.Fatal(err)
	}

	m := workloadOnlyTraceManifest("prune-owner")
	project := RuntimeProjectName(m)
	external := "https://receiver.example.test:43189"
	writeTestRuntimeOTLPBinding(t, m, string(capability.ProviderExternalOTLP), external, external)
	if err := reconcileManagedRuntimeObservability(m, project); err != nil {
		t.Fatal(err)
	}
	// Seed the other receiver kind the way a stale registry would carry it, then
	// perform a receiver transition the workload itself controls.
	if err := observability.UpdateSignal(workloadReceiverTraceSource(m, project, capability.ProviderOTelCollector)); err != nil {
		t.Fatal(err)
	}
	if err := observability.UpdateSignal(workloadReceiverTraceSource(m, project, capability.ProviderPrometheus)); err != nil {
		t.Fatal(err)
	}
	t.Setenv(ProviderExternalReferenceEnv(capability.ProviderExternalOTLP), "")
	t.Setenv(OTLPEndpointEnv, "")
	t.Setenv(OTLPProviderEnv, "")
	if err := reconcileManagedRuntimeObservability(m, project); err != nil {
		t.Fatal(err)
	}

	placement, err := ResolveProviderPlacement(m, capability.ProviderTempo)
	if err != nil {
		t.Fatal(err)
	}
	sources, err := observability.ListTraces(placement, []string{m.Name, ""}, true, true)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, source := range sources {
		if source.ID == foreign.ID && source.Class == observability.SourcePlatformProvider {
			found = true
		}
		if source.OwnerApplication == m.Name && source.Provider == capability.ProviderExternalOTLP {
			t.Fatalf("retired receiver identity survived the transition: %+v", source)
		}
		if source.OwnerApplication == m.Name && source.Provider == capability.ProviderPrometheus {
			t.Fatalf("legacy Prometheus trace signal survived the transition: %+v", source)
		}
	}
	if !found {
		t.Fatalf("foreign shared collector trace source was reclaimed by workload reconciliation: %+v", sources)
	}
}

// Registry IDs are only unique per provider - RegisterProviderSignals replaces
// by ID plus provider, and the Tempo lifecycle registers <provider>:<project>
// signal sources of its own - so both workload mutation paths, the signal
// reconciliation and the trace prune, must key on the provider as well. A
// trace source another provider registered under the SAME ID stays alive
// across a full reconcile and the later intent removal.
func TestWorkloadTracePruneKeepsSameIDUnderOtherProvider(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	m := workloadOnlyTraceManifest("prune-sameid")
	project := RuntimeProjectName(m)
	id := fmt.Sprintf("%s:%s", capability.ProviderExternalOTLP, project)
	foreign := observability.SignalSource{
		ID:       id,
		Kind:     observability.SignalTraces,
		Provider: capability.ProviderOTelCollector,
		Class:    observability.SourcePlatformProvider,
		Scope:    capability.ScopeShared,
		Target:   "foreign", Protocol: "otlp",
		Mode:         capability.ObservabilityNative,
		Verification: capability.ObservabilityVerifySpan,
	}
	if err := observability.UpdateSignal(foreign); err != nil {
		t.Fatal(err)
	}

	// Path one: the workload reconciles its trace source while the foreign
	// same-ID registration exists - the reconcile may only replace signals it
	// owns under the external receiver's provider.
	external := "https://receiver.example.test:43189"
	writeTestRuntimeOTLPBinding(t, m, string(capability.ProviderExternalOTLP), external, external)
	if err := reconcileManagedRuntimeObservability(m, project); err != nil {
		t.Fatal(err)
	}
	live, err := observability.RegistryHasSignal(id, capability.ProviderOTelCollector, observability.SignalTraces)
	if err != nil {
		t.Fatal(err)
	}
	if !live {
		t.Fatal("same-ID trace source under another provider died during reconciliation")
	}

	// Path two: remove the OTLP intent - this workload's external-receiver
	// identity must be reclaimed by the provider-keyed prune, and the foreign
	// same-ID source must still live.
	trimmed := m
	trimmed.Telemetry = TelemetryRequirements{}
	if err := reconcileApplicationTraceSource(trimmed, project, false); err != nil {
		t.Fatal(err)
	}

	live, err = observability.RegistryHasSignal(id, capability.ProviderExternalOTLP, observability.SignalTraces)
	if err != nil {
		t.Fatal(err)
	}
	if live {
		t.Fatal("workload's own external-receiver trace identity survived the intent removal")
	}
	live, err = observability.RegistryHasSignal(id, capability.ProviderOTelCollector, observability.SignalTraces)
	if err != nil {
		t.Fatal(err)
	}
	if !live {
		t.Fatal("same-ID trace source under another provider was co-deleted by the workload prune")
	}
}

// The legacy Prometheus-attributed workload trace signal predates this
// reconciliation and must be reclaimed even when the metrics policy is
// disabled - that is the state a later deployment can reach after the earlier
// release registered it. Its metrics and logs signals remain registered: this
// reclaims the trace signal only.
func TestLegacyPrometheusTraceSignalIsReclaimedWithoutMetricsPolicy(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv(MetricsEnabledEnv, "false")
	m := workloadOnlyTraceManifest("legacy-metrics-off")
	project := RuntimeProjectName(m)
	legacyID := fmt.Sprintf("%s:%s", capability.ProviderPrometheus, project)

	// Simulate the legacy state: an earlier apply with metrics enabled left a
	// Prometheus-owned trace signal behind.
	t.Setenv(MetricsEnabledEnv, "true")
	if err := observability.UpdateSignal(observability.SignalSource{
		ID:                 legacyID,
		Kind:               observability.SignalTraces,
		Provider:           capability.ProviderPrometheus,
		Class:              observability.SourceApplicationProvider,
		Scope:              capability.ScopeApplication,
		OwnerApplication:   m.Name,
		Target:             observability.RuntimeTarget(project, "workload"),
		Protocol:           "otlp",
		Mode:               capability.ObservabilityNative,
		SemanticConvention: "http",
		Verification:       capability.ObservabilityVerifySpan,
	}); err != nil {
		t.Fatal(err)
	}
	if err := observability.UpdateSignal(observability.SignalSource{
		ID:               legacyID,
		Kind:             observability.SignalMetrics,
		Provider:         capability.ProviderPrometheus,
		Class:            observability.SourceApplicationProvider,
		Scope:            capability.ScopeApplication,
		OwnerApplication: m.Name,
		Network:          MetricsProviderNetworkNameForNamespace(m, ""),
		Target:           observability.RuntimeTarget(project, "workload"),
		Protocol:         "openmetrics",
		Path:             "/metrics",
		Mode:             capability.ObservabilityRuntime,
		Verification:     capability.ObservabilityVerifyNone,
	}); err != nil {
		t.Fatal(err)
	}
	if got := traceIDs(traceSources(t, m))[legacyID]; got != capability.ProviderPrometheus {
		t.Fatalf("legacy fixture is not in place: %v", got)
	}

	// Reconcile with the metrics policy disabled: this is the path that used to
	// return before reaching any signal reconciliation.
	t.Setenv(MetricsEnabledEnv, "false")
	if err := reconcileManagedRuntimeObservability(m, project); err != nil {
		t.Fatal(err)
	}
	if got := traceIDs(traceSources(t, m))[legacyID]; got != "" {
		t.Fatalf("legacy Prometheus trace signal survived a metrics-disabled reconciliation: %v", got)
	}
	loggedPlacement, err := ResolveProviderPlacement(m, capability.ProviderLoki)
	if err != nil {
		t.Fatal(err)
	}
	logged, err := observability.List(observability.SignalLogs, loggedPlacement, []string{m.Name}, true, true)
	if err != nil {
		t.Fatal(err)
	}
	metricsPlacement, err := ResolveProviderPlacement(m, capability.ProviderPrometheus)
	if err != nil {
		t.Fatal(err)
	}
	metrics, err := observability.List(observability.SignalMetrics, metricsPlacement, []string{m.Name}, true, true)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, source := range metrics {
		if source.ID == legacyID {
			found = true
		}
	}
	if !found {
		t.Fatalf("metrics-disabled reclaim removed the workload's metrics signal too: %+v", metrics)
	}
	for _, source := range logged {
		if source.ID == legacyID {
			t.Fatalf("metrics-disabled reclaim removed the workload's log signal: %+v", logged)
		}
	}
}

// Disabling provider trace collection must reclaim the workload's receiver
// trace identities: a surviving registration would keep verification pointed at
// a receiver the deployment stopped collecting. Metrics and log signals of the
// same identities stay registered because their providers still ingest them.
func TestWorkloadReceiverTraceSignalsReclaimedWhenTracesDisabled(t *testing.T) {
	t.Run("managed_collector", func(t *testing.T) {
		t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
		m := workloadOnlyTraceManifest("trace-policy-off")
		m = WithLogsCollection(m, "application")
		project := RuntimeProjectName(m)
		writeTestRuntimeOTLPBinding(t, m, string(capability.ProviderOTelCollector), "https://127.0.0.1:43183", "https://otel-collector:4318")
		if err := reconcileManagedRuntimeObservability(m, project); err != nil {
			t.Fatal(err)
		}
		collectorID := fmt.Sprintf("%s:%s", capability.ProviderOTelCollector, project)
		if got := traceIDs(traceSources(t, m)); got[collectorID] != capability.ProviderOTelCollector {
			t.Fatalf("fixture did not register the collector identity: %v", got)
		}
		loggedPlacement, err := ResolveProviderPlacement(m, capability.ProviderLoki)
		if err != nil {
			t.Fatal(err)
		}
		logged, err := observability.List(observability.SignalLogs, loggedPlacement, []string{m.Name}, true, true)
		if err != nil {
			t.Fatal(err)
		}

		t.Setenv(TracesEnabledEnv, "false")
		if err := reconcileManagedRuntimeObservability(m, project); err != nil {
			t.Fatal(err)
		}
		if got := traceIDs(traceSources(t, m)); got[collectorID] != "" {
			t.Fatalf("collector trace signal survived disabled trace collection: %v", got)
		}
		loggedAfter, err := observability.List(observability.SignalLogs, loggedPlacement, []string{m.Name}, true, true)
		if err != nil {
			t.Fatal(err)
		}
		if len(loggedAfter) != len(logged) {
			t.Fatalf("disabling traces changed the log registrations: before=%+v after=%+v", logged, loggedAfter)
		}
		metricsPlacement, err := ResolveProviderPlacement(m, capability.ProviderPrometheus)
		if err != nil {
			t.Fatal(err)
		}
		metricsAfter, err := observability.List(observability.SignalMetrics, metricsPlacement, []string{m.Name}, true, true)
		if err != nil {
			t.Fatal(err)
		}
		if len(metricsAfter) == 0 {
			t.Fatalf("disabling traces removed the workload metrics identity: %+v", metricsAfter)
		}
	})

	// An external receiver delivers spans without managed trace storage, so the
	// managed trace verification never registered a workload source under it:
	// disabling provider trace collection must not delete a signal the
	// collector transport still provides.
	t.Run("registered_external", func(t *testing.T) {
		t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
		m := workloadOnlyTraceManifest("trace-policy-ext")
		project := RuntimeProjectName(m)
		writeTestRuntimeOTLPBinding(t, m, string(capability.ProviderExternalOTLP), "https://receiver.example.test:43189", "https://receiver.example.test:43189")
		if err := reconcileManagedRuntimeObservability(m, project); err != nil {
			t.Fatal(err)
		}
		externalID := fmt.Sprintf("%s:%s", capability.ProviderExternalOTLP, project)
		if got := traceIDs(traceSources(t, m)); got[externalID] != capability.ProviderExternalOTLP {
			t.Fatalf("fixture did not register the external identity: %v", got)
		}
		t.Setenv(TracesEnabledEnv, "false")
		if err := reconcileApplicationTraceSource(m, project, false); err != nil {
			t.Fatal(err)
		}
		if got := traceIDs(traceSources(t, m)); got[externalID] != capability.ProviderExternalOTLP {
			t.Fatalf("external signal was reclaimed by a path that never registered it: %v", got)
		}
	})
}

// Without any materialized receiver the trace signal has no OTLP endpoint, so
// it must stay out of the registry rather than being attributed to a backend
// that cannot serve it. A leftover legacy Prometheus trace signal is reclaimed
// even on this path, while the metrics signal of the same identity stays.
func TestWorkloadTraceSourceRequiresMaterializedReceiver(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	// An app without OTLP telemetry has no receiver and must register no trace
	// source at all, in any receiver-transition state.
	m := WithWorkloadComponents(New("trace-unbound", "dev", false, false, false), "api")
	project := RuntimeProjectName(m)
	if err := reconcileManagedRuntimeObservability(m, project); err != nil {
		t.Fatal(err)
	}
	if got := traceIDs(traceSources(t, m)); len(got) != 0 {
		t.Fatalf("workload without OTLP intent registered a trace source: %v", got)
	}

	// An app that does intend OTLP always names the policy-selected receiver,
	// even before its binding projection exists: the exporter and the registry
	// must not disagree while a deployment is in flight.
	bound := WithOTLPTelemetry(WithWorkloadComponents(New("trace-bound", "dev", false, false, false), "api"), "traces")
	boundProject := RuntimeProjectName(bound)
	if err := reconcileManagedRuntimeObservability(bound, boundProject); err != nil {
		t.Fatal(err)
	}
	collectorID := fmt.Sprintf("%s:%s", capability.ProviderOTelCollector, boundProject)
	if got := traceIDs(traceSources(t, bound)); got[collectorID] != capability.ProviderOTelCollector {
		t.Fatalf("managed deployment registered no collector identity: %v", got)
	}

	// The legacy Prometheus-attributed signal is reclaimed even with the
	// metrics policy disabled, and the metrics signal of that ID survives.
	t.Setenv(MetricsEnabledEnv, "false")
	legacyID := fmt.Sprintf("%s:%s", capability.ProviderPrometheus, project)
	if err := observability.UpdateSignal(workloadReceiverTraceSource(m, project, capability.ProviderPrometheus)); err != nil {
		t.Fatal(err)
	}
	if err := observability.UpdateSignal(observability.SignalSource{
		ID: legacyID, Kind: observability.SignalMetrics, Provider: capability.ProviderPrometheus,
		Class: observability.SourceApplicationProvider, Scope: capability.ScopeApplication,
		OwnerApplication: m.Name, Network: MetricsProviderNetworkNameForNamespace(m, ""),
		Target:   observability.RuntimeTarget(project, "workload"),
		Protocol: "openmetrics", Path: "/metrics", Mode: capability.ObservabilityRuntime,
		Verification: capability.ObservabilityVerifyNone,
	}); err != nil {
		t.Fatal(err)
	}
	if err := reconcileApplicationMetricsSource(m, project, false, false); err != nil {
		t.Fatal(err)
	}
	if got := traceIDs(traceSources(t, m)); got[legacyID] != "" {
		t.Fatalf("legacy Prometheus trace signal survived a metrics-disabled reconciliation: %v", got)
	}
	metricsPlacement, err := ResolveProviderPlacement(m, capability.ProviderPrometheus)
	if err != nil {
		t.Fatal(err)
	}
	metrics, err := observability.List(observability.SignalMetrics, metricsPlacement, []string{m.Name}, true, true)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, source := range metrics {
		if source.ID == legacyID {
			found = true
		}
	}
	if !found {
		t.Fatalf("trace-signal reclaim removed the workload metrics signal too: %+v", metrics)
	}
}

// workloadReceiverTraceSource builds the trace registration one OTLP receiver
// of this workload owns: same identity, class, scope and workload target the
// reconciliation writes.
func workloadReceiverTraceSource(m Manifest, project string, receiver capability.ProviderKind) observability.SignalSource {
	return observability.SignalSource{
		ID:                 fmt.Sprintf("%s:%s", receiver, project),
		Kind:               observability.SignalTraces,
		Provider:           receiver,
		Class:              observability.SourceApplicationProvider,
		Scope:              capability.ScopeApplication,
		OwnerApplication:   m.Name,
		Target:             observability.RuntimeTarget(project, "workload"),
		Protocol:           "otlp",
		Mode:               capability.ObservabilityNative,
		SemanticConvention: "http",
		Verification:       capability.ObservabilityVerifySpan,
	}
}
