package capability

import (
	"context"
	"testing"
)

type adapterTestDriver struct {
	provider Provider
}

func (d *adapterTestDriver) Descriptor() Provider                               { return d.provider }
func (d *adapterTestDriver) Preflight(context.Context, Resource, Binding) error { return nil }
func (d *adapterTestDriver) Provision(context.Context, Resource, Binding) error { return nil }
func (d *adapterTestDriver) Bind(context.Context, Resource, Binding) error      { return nil }
func (d *adapterTestDriver) Verify(context.Context, Resource, Binding) error    { return nil }

func TestCurrentReferenceIntegrationsConform(t *testing.T) {
	for _, descriptor := range []IntegrationDescriptor{
		PostgreSQLIntegration,
		ValkeyIntegration,
		OpenBaoIntegration,
		CaddyIntegration,
		SeaweedFSIntegration,
		OTelCollectorIntegration,
		ExternalOTLPIntegration,
		PrometheusIntegration,
		LokiIntegration,
		TempoIntegration,
	} {
		report := CheckIntegrationContract(descriptor)
		if report.Status != ConformancePass {
			t.Fatalf("%s conformance = %#v", descriptor.Provider.Kind, report)
		}
	}
}

func TestIntegrationDescriptorRejectsServiceCapabilityMismatch(t *testing.T) {
	descriptor := PostgreSQLIntegration
	descriptor.Services = []ServiceKind{ServiceCache}
	if err := descriptor.Validate(); err == nil {
		t.Fatal("provider service/capability mismatch accepted")
	}
}

func TestIntegrationDescriptorRequiresProviderImplementationVersion(t *testing.T) {
	descriptor := PostgreSQLIntegration
	descriptor.Version = ""
	if err := descriptor.Validate(); err == nil {
		t.Fatal("provider without implementation version accepted")
	}
}

func TestIntegrationDescriptorRequiresProviderID(t *testing.T) {
	descriptor := PostgreSQLIntegration
	descriptor.ID = ""
	if err := descriptor.Validate(); err == nil {
		t.Fatal("provider without distribution id accepted")
	}
}

func TestIntegrationDescriptorRejectsUnversionedCapability(t *testing.T) {
	descriptor := PostgreSQLIntegration
	descriptor.Capabilities = []SpecificationID{"database.sql"}
	if err := descriptor.Validate(); err == nil {
		t.Fatal("unversioned capability specification accepted")
	}
}

func TestIntegrationDescriptorRejectsCapabilityProviderMismatch(t *testing.T) {
	descriptor := PostgreSQLIntegration
	descriptor.Capabilities = []SpecificationID{KeyValueV1.ID}
	if err := descriptor.Validate(); err == nil {
		t.Fatal("provider claimed unsupported capability specification")
	}
}

func TestIntegrationDescriptorRequiresEveryProviderCapabilityToHaveSpecification(t *testing.T) {
	descriptor := IntegrationDescriptor{
		ID: "baseharbor/test-provider", Version: "0.1.0",
		Protocol: ProviderProtocolV1,
		Provider: Provider{
			Kind:         ProviderPostgreSQL,
			Capabilities: []Kind{SQL, KeyValue},
		},
		Capabilities:    []SpecificationID{SQLV1.ID},
		SupportedScopes: []ProviderScope{ScopeApplication},
	}
	if err := descriptor.Validate(); err == nil {
		t.Fatal("provider capability without specification accepted")
	}
}

func TestDriverAdapterUsesExistingLifecycleDriver(t *testing.T) {
	driver := &adapterTestDriver{provider: PostgreSQL}
	adapter, err := NewDriverAdapter(driver, PostgreSQLIntegration)
	if err != nil {
		t.Fatal(err)
	}
	if adapter.Descriptor().Kind != ProviderPostgreSQL {
		t.Fatalf("provider = %q", adapter.Descriptor().Kind)
	}
	if adapter.IntegrationDescriptor().Protocol != ProviderProtocolV1 {
		t.Fatalf("protocol = %q", adapter.IntegrationDescriptor().Protocol)
	}
}

func TestDriverAdapterRejectsDifferentProvider(t *testing.T) {
	driver := &adapterTestDriver{provider: Valkey}
	if _, err := NewDriverAdapter(driver, PostgreSQLIntegration); err == nil {
		t.Fatal("mismatched driver/provider accepted")
	}
}

func TestSpecificationIDsAreCanonical(t *testing.T) {
	for _, spec := range []CapabilitySpecification{SQLV1, KeyValueV1, SecretsV1, ExposureHTTPV1, ObjectStorageS3V1, TelemetryOTLPV1, MetricsV1, LogsV1, TracesV1} {
		parsed, err := ParseSpecificationID(spec.ID)
		if err != nil {
			t.Fatal(err)
		}
		if parsed != spec {
			t.Fatalf("parsed = %#v, want %#v", parsed, spec)
		}
	}
}

func TestDriverAdapterRejectsCapabilityDescriptorMismatch(t *testing.T) {
	driver := &adapterTestDriver{provider: Provider{
		Kind:         ProviderPostgreSQL,
		Capabilities: []Kind{SQL, KeyValue},
	}}
	if _, err := NewDriverAdapter(driver, PostgreSQLIntegration); err == nil {
		t.Fatal("same-kind driver with different capability set accepted")
	}
}

func TestIntegrationDescriptorRequiresSupportedPlacementScope(t *testing.T) {
	descriptor := PostgreSQLIntegration
	descriptor.SupportedScopes = nil
	if err := descriptor.Validate(); err == nil {
		t.Fatal("provider without supported placement scope accepted")
	}
}

func TestIntegrationDescriptorRejectsDuplicatePlacementScope(t *testing.T) {
	descriptor := PostgreSQLIntegration
	descriptor.SupportedScopes = []ProviderScope{ScopeApplication, ScopeApplication}
	if err := descriptor.Validate(); err == nil {
		t.Fatal("duplicate provider placement scope accepted")
	}
}

func TestIntegrationDescriptorValidatesObservabilitySignals(t *testing.T) {
	descriptor := TempoIntegration
	descriptor.Observability.Signals = append([]ProviderObservabilitySignal(nil), TempoIntegration.Observability.Signals...)
	descriptor.Observability.Signals[0].Path = "metrics"
	if err := descriptor.Validate(); err == nil {
		t.Fatal("relative provider metrics path accepted")
	}
}

func TestManagedReferenceIntegrationsAuditEveryObservabilitySignal(t *testing.T) {
	managed := []IntegrationDescriptor{
		PostgreSQLIntegration,
		ValkeyIntegration,
		OpenBaoIntegration,
		CaddyIntegration,
		SeaweedFSIntegration,
		OTelCollectorIntegration,
		PrometheusIntegration,
		LokiIntegration,
		TempoIntegration,
	}
	for _, descriptor := range managed {
		seen := map[ObservabilitySignalKind]bool{}
		for _, signal := range descriptor.Observability.Signals {
			seen[signal.Kind] = true
			if signal.Collectable() != (signal.Status == ObservabilitySupported) {
				t.Fatalf("%s signal %s collectable=%v status=%s", descriptor.Provider.Kind, signal.Name, signal.Collectable(), signal.Status)
			}
		}
		for _, kind := range []ObservabilitySignalKind{ObservabilityMetrics, ObservabilityLogs, ObservabilityTraces} {
			if !seen[kind] {
				t.Fatalf("%s does not audit %s observability", descriptor.Provider.Kind, kind)
			}
		}
	}
}

func TestObservabilityRequiresAdapterIsAuditedButNotCollectable(t *testing.T) {
	signal := PostgreSQLIntegration.Observability.Signals[0]
	if signal.Status != ObservabilityRequiresAdapter || signal.Mode != ObservabilityAdapter {
		t.Fatalf("postgres metrics coverage = %#v", signal)
	}
	if signal.Collectable() {
		t.Fatal("adapter-required provider signal was treated as collectable")
	}
}

func TestSupportedProviderSignalRequiresVerification(t *testing.T) {
	descriptor := OTelCollectorIntegration
	descriptor.Observability.Signals = append([]ProviderObservabilitySignal(nil), OTelCollectorIntegration.Observability.Signals...)
	descriptor.Observability.Signals[0].Verification = ObservabilityVerifyNone
	if err := descriptor.Validate(); err == nil {
		t.Fatal("supported provider signal without verification accepted")
	}
}
