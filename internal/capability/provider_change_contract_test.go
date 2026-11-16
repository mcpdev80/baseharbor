package capability

import "testing"

func TestEveryRegisteredProviderHasChangeContract(t *testing.T) {
	for _, descriptor := range ReferenceIntegrations() {
		t.Run(string(descriptor.Provider.Kind), func(t *testing.T) {
			c, ok := descriptor.ChangeContract()
			if !ok || len(c.Functions) == 0 || len(c.MaterialKinds) == 0 || len(c.Aspects) == 0 {
				t.Fatalf("missing provider impact metadata: %+v", c)
			}
		})
	}
	if _, ok := (IntegrationDescriptor{Provider: Provider{Kind: "unknown-extension"}}).ChangeContract(); ok {
		t.Fatal("undeclared provider reported covered")
	}
}

func TestAncillaryMetadataDoesNotInventSelectableIntegrations(t *testing.T) {
	for _, kind := range []ProviderKind{ProviderRuntimeBroker, ProviderRuntimeExecutor, ProviderRuntimeWorkload} {
		c, ok := (IntegrationDescriptor{Provider: Provider{Kind: kind}}).ChangeContract()
		if !ok || len(c.Functions) == 0 || len(c.MaterialKinds) == 0 {
			t.Fatal("missing existing sidecar consequences")
		}
		if _, err := ReferenceIntegration(kind); err == nil {
			t.Fatal("ancillary metadata became a selectable provider integration")
		}
	}
}
