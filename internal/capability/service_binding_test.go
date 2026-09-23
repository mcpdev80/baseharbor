package capability

import "testing"

func TestServiceBindingWellKnownNames(t *testing.T) {
	for _, name := range []string{
		ServiceBindingType,
		ServiceBindingProvider,
		ServiceBindingHost,
		ServiceBindingPort,
		ServiceBindingURI,
		ServiceBindingUsername,
		ServiceBindingPassword,
		ServiceBindingCertificates,
		ServiceBindingPrivateKey,
	} {
		if !IsServiceBindingWellKnownName(name) {
			t.Fatalf("%q not recognized as well-known", name)
		}
		if err := ValidateServiceBindingName(name); err != nil {
			t.Fatalf("%q rejected: %v", name, err)
		}
	}
}

func TestServiceBindingRejectsAliases(t *testing.T) {
	for _, name := range []string{"hostname", "connectionHost", "user", "pass", "connectionString"} {
		if err := ValidateServiceBindingName(name); err == nil {
			t.Fatalf("alias %q accepted", name)
		}
	}
}

func TestServiceBindingAllowsNamespacedExtensions(t *testing.T) {
	if err := ValidateServiceBindingName("baha.identity-ref"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateServiceBindingName("provider.example/region"); err != nil {
		t.Fatal(err)
	}
}
