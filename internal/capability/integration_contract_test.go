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
		KeycloakIntegration,
		ExternalOIDCIntegration,
		RabbitMQIntegration,
		MongoDBIntegration,
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
	specs := Specifications()
	if len(specs) != 15 {
		t.Fatalf("shipped capability catalog has %d specifications, want 15: %#v", len(specs), specs)
	}
	for _, spec := range specs {
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

// TestPostgreSQLAndValkeyMetricsAreNotRequiresAdapter documents the 0428-182 fix:
// postgres and valkey declare NO collectable metrics signal because there is no
// exporter, no compose service and no runtime metrics producer. Declaring them as
// ObservabilityRequiresAdapter would promise an adapter realization that does not
// exist (integration_contract.go:249 forces adapter realization for that status),
// so the metrics signal is ObservabilityUnsupported and therefore not collectable.
func TestPostgreSQLAndValkeyMetricsAreNotRequiresAdapter(t *testing.T) {
	for _, descriptor := range []IntegrationDescriptor{PostgreSQLIntegration, ValkeyIntegration} {
		var metricsSignal ProviderObservabilitySignal
		for _, signal := range descriptor.Observability.Signals {
			if signal.Kind == ObservabilityMetrics {
				metricsSignal = signal
			}
		}
		if metricsSignal.Status != ObservabilityUnsupported {
			t.Fatalf("%s metrics status = %s, want ObservabilityUnsupported", descriptor.Provider.Kind, metricsSignal.Status)
		}
		if metricsSignal.Mode != "" {
			t.Fatalf("%s metrics must carry no realization mode for status unsupported, got %q", descriptor.Provider.Kind, metricsSignal.Mode)
		}
		if metricsSignal.Protocol != "" {
			t.Fatalf("%s metrics must carry no protocol for status unsupported, got %q", descriptor.Provider.Kind, metricsSignal.Protocol)
		}
		if metricsSignal.Verification != ObservabilityVerifyNone {
			t.Fatalf("%s metrics must skip verification for status unsupported", descriptor.Provider.Kind)
		}
		if metricsSignal.Collectable() {
			t.Fatalf("%s metrics must not be collectable", descriptor.Provider.Kind)
		}
	}
}

// TestPostgreSQLAndValkeyMetricsWithoutProducerFailCatalog is the 0428-182
// regression: these two reference integrations have no metrics producer, so a
// RequiresAdapter declaration would promise a producer that does not exist. Keep
// the assertion scoped to those affected integrations; future adapter-backed
// metrics may legitimately use RequiresAdapter once they have a real producer.
func TestPostgreSQLAndValkeyMetricsWithoutProducerFailCatalog(t *testing.T) {
	for _, descriptor := range []IntegrationDescriptor{PostgreSQLIntegration, ValkeyIntegration} {
		for _, signal := range descriptor.Observability.Signals {
			if signal.Kind == ObservabilityMetrics && signal.Status == ObservabilityRequiresAdapter {
				t.Fatalf("%s metrics declares %s but has no metrics producer", descriptor.Provider.Kind, signal.Status)
			}
		}
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
