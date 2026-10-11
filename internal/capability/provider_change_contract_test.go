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
