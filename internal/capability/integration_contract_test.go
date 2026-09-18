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
func (d *adapterTestDriver) Provision(context.Context, Resource) error          { return nil }
func (d *adapterTestDriver) Bind(context.Context, Resource, Binding) error      { return nil }
func (d *adapterTestDriver) Verify(context.Context, Resource, Binding) error    { return nil }

func TestCurrentReferenceIntegrationsConform(t *testing.T) {
	for _, descriptor := range []IntegrationDescriptor{
		PostgreSQLIntegration,
		ValkeyIntegration,
		OpenBaoIntegration,
	} {
		report := CheckIntegrationContract(descriptor)
		if report.Status != ConformancePass {
			t.Fatalf("%s conformance = %#v", descriptor.Provider.Kind, report)
		}
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
		Protocol: ProviderProtocolV1,
		Provider: Provider{
			Kind:         ProviderPostgreSQL,
			Capabilities: []Kind{SQL, KeyValue},
		},
		Capabilities: []SpecificationID{SQLV1.ID},
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
	for _, spec := range []CapabilitySpecification{SQLV1, KeyValueV1, SecretsV1} {
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
