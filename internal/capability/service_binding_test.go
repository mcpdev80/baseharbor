package capability

import "testing"

func TestValidateServiceBindingOutputName(t *testing.T) {
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
		"baha.identity-ref",
	} {
		if err := ValidateServiceBindingOutputName(name); err != nil {
			t.Fatalf("%q rejected: %v", name, err)
		}
	}
	for _, name := range []string{"", "hostname", "connectionHost", "user", "pass", "connectionString", "baha."} {
		if err := ValidateServiceBindingOutputName(name); err == nil {
			t.Fatalf("%q unexpectedly accepted", name)
		}
	}
}
