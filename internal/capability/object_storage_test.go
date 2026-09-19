package capability

import (
	"context"
	"testing"
)

type alternateS3Driver struct{}

func (alternateS3Driver) Descriptor() Provider {
	return Provider{Kind: ProviderKind("external-s3"), Capabilities: []Kind{ObjectStorageS3}}
}
func (alternateS3Driver) Preflight(context.Context, Resource, Binding) error { return nil }
func (alternateS3Driver) Provision(context.Context, Resource, Binding) error { return nil }
func (alternateS3Driver) Bind(context.Context, Resource, Binding) error      { return nil }
func (alternateS3Driver) Verify(context.Context, Resource, Binding) error    { return nil }

func TestObjectStorageS3ContractIsProviderReplaceable(t *testing.T) {
	security := SecureBinding{
		Credentials: []CredentialReference{
			{Name: "access-key-id", Reference: "baseharbor://applications/demo/prod/object-storage/assets/credentials/access-key-id"},
			{Name: "secret-access-key", Reference: "baseharbor://applications/demo/prod/object-storage/assets/credentials/secret-access-key"},
		},
	}
	request := Request{
		Requirement:     Requirement{Kind: ObjectStorageS3, Name: "assets"},
		Workload:        "application/demo",
		ObjectStorageS3: &ObjectStorageS3Binding{Bucket: "assets"},
		Security:        &security,
	}
	seaweed := request
	seaweed.Driver = structS3Driver{provider: SeaweedFS}
	alternate := request
	alternate.Driver = alternateS3Driver{}

	seaweedPlan, err := BuildPlan("demo", []Request{seaweed})
	if err != nil {
		t.Fatal(err)
	}
	alternatePlan, err := BuildPlan("demo", []Request{alternate})
	if err != nil {
		t.Fatal(err)
	}
	a := seaweedPlan.Items[0].Binding
	b := alternatePlan.Items[0].Binding
	if a.ObjectStorageS3.Bucket != b.ObjectStorageS3.Bucket || a.Workload != b.Workload {
		t.Fatalf("provider replacement changed portable binding: %#v vs %#v", a, b)
	}
	if a.Security.Credentials[0].Reference != b.Security.Credentials[0].Reference {
		t.Fatal("provider replacement changed secure credential reference")
	}
}

type structS3Driver struct{ provider Provider }

func (d structS3Driver) Descriptor() Provider                             { return d.provider }
func (structS3Driver) Preflight(context.Context, Resource, Binding) error { return nil }
func (structS3Driver) Provision(context.Context, Resource, Binding) error { return nil }
func (structS3Driver) Bind(context.Context, Resource, Binding) error      { return nil }
func (structS3Driver) Verify(context.Context, Resource, Binding) error    { return nil }

func TestSeaweedFSReferenceIntegrationConforms(t *testing.T) {
	report := CheckIntegrationContract(SeaweedFSIntegration)
	if report.Status != ConformancePass {
		t.Fatalf("conformance = %#v", report)
	}
	if len(SeaweedFSIntegration.Capabilities) != 1 || SeaweedFSIntegration.Capabilities[0] != ObjectStorageS3V1.ID {
		t.Fatalf("capabilities = %#v", SeaweedFSIntegration.Capabilities)
	}
}
