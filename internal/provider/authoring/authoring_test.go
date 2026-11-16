package authoring

import (
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

func TestDescriptorUsesExistingProviderConformance(t *testing.T) {
	d := Descriptor{
		ID:               "example/postgresql",
		Version:          "1.0.0",
		ProviderProtocol: capability.ProviderProtocolV1,
		ServiceKinds:     []string{"sql"},
		ServiceContracts: []string{"database.sql/v1"},
		SupportedScopes:  []string{"application"},
	}
	report := Check(d)
	if report.Status != capability.ConformancePass {
		t.Fatalf("conformance = %#v", report)
	}
}

func TestInitAndLoad(t *testing.T) {
	root := t.TempDir() + "/provider"
	result, err := Init(root, "example/postgresql")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Files) != 5 {
		t.Fatalf("files = %#v", result.Files)
	}
	descriptor, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if descriptor.ID != "example/postgresql" {
		t.Fatalf("id = %q", descriptor.ID)
	}
}

func TestDescriptorSupportsEveryNewV0419ServiceContract(t *testing.T) {
	cases := []struct {
		contract string
		service  string
		kind     capability.Kind
	}{
		{"database.key-value/v1", "key-value", capability.DurableKeyValue},
		{"database.document/v1", "document-database", capability.DocumentDatabase},
		{"messaging.queue/v1", "messaging", capability.MessagingQueue},
		{"messaging.pubsub/v1", "messaging", capability.MessagingPubSub},
		{"messaging.stream/v1", "messaging", capability.MessagingStream},
		{"exposure.http/v1", "exposure", capability.ExposureHTTP},
	}
	for _, tc := range cases {
		t.Run(tc.contract, func(t *testing.T) {
			d := Descriptor{
				ID:               "example/provider",
				Version:          "1.0.0",
				ProviderProtocol: capability.ProviderProtocolV1,
				ServiceKinds:     []string{tc.service},
				ServiceContracts: []string{tc.contract},
				SupportedScopes:  []string{"application"},
			}
			integration, err := d.IntegrationDescriptor()
			if err != nil {
				t.Fatal(err)
			}
			if len(integration.Provider.Capabilities) != 1 || integration.Provider.Capabilities[0] != tc.kind {
				t.Fatalf("capabilities = %#v, want %q", integration.Provider.Capabilities, tc.kind)
			}
			if len(integration.Services) != 1 || string(integration.Services[0]) != tc.service {
				t.Fatalf("services = %#v, want %q", integration.Services, tc.service)
			}
		})
	}
}
