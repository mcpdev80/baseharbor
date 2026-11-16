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

func TestProjectingServiceBindingNamesAtBoundary(t *testing.T) {
	standardBinding := Binding{
		Resource: Resource{Application: "app", Kind: SQL, Name: "pg", Provider: "baseharbor"},
		Security: &SecureBinding{
			Identity:    &WorkloadIdentityBinding{Subject: "spiffe://baseharbor/apps/app", Reference: "openbao://secret"},
			Credentials: []CredentialReference{{Name: "c", Reference: "openbao://c"}},
			Secrets:     []SecretReference{{Name: "s", Reference: "openbao://s"}},
			Trust:       []TrustMaterialReference{{Name: "t", Reference: "ca-bundle"}},
		},
	}
	for _, name := range projectedServiceBindingNames(standardBinding) {
		if err := ValidateServiceBindingName(name); err != nil {
			t.Fatalf("projected entry %q at binding boundary rejected: %v", name, err)
		}
	}

	unknownNamespacedBinding := Binding{
		Resource: Resource{Application: "app", Kind: SQL, Name: "pg", Provider: "baseharbor"},
	}
	_ = projectedServiceBindingNames(unknownNamespacedBinding)
	if err := ValidateServiceBindingName("region"); err == nil {
		t.Fatal("projected entry \"region\" at binding boundary accepted though non-namespaced/unknown")
	}
}

func TestProjectedServiceBindingNamesEmptyWithoutProvider(t *testing.T) {
	binding := Binding{}
	if got := len(projectedServiceBindingNames(binding)); got != 0 {
		t.Fatalf("expected no projected output names for empty binding, got %d", got)
	}
}
