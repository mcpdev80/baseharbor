package capability

import "testing"

func TestSecureBindingValidateAcceptsProviderNeutralReferences(t *testing.T) {
	binding := SecureBinding{
		Identity: &WorkloadIdentityBinding{
			Subject:   "application/mailflow:worker",
			Reference: "baseharbor://identity/mailflow/worker",
		},
		Credentials: []CredentialReference{{
			Name:      "database",
			Reference: "baseharbor://credential/mailflow/postgres-primary",
		}},
		Trust: []TrustMaterialReference{{
			Name:      "runtime-ca",
			Reference: "baseharbor://trust/mailflow/runtime-ca",
			Format:    "x509-pem",
		}},
		Authorization: []AuthorizationMetadata{{
			Audience: "runtime-secrets",
			Scopes:   []string{"secrets.read"},
		}},
		Secrets: []SecretReference{{
			Name:      "SMTP_PASSWORD",
			Reference: "baseharbor://secret/mailflow/SMTP_PASSWORD",
		}},
		Lifecycle: SecurityLifecycleSupport{
			Renewable: true,
			Rotatable: true,
			Revocable: true,
		},
		Diagnostics: []Diagnostic{{
			Code:     "binding-ready",
			Severity: SeverityInfo,
			Message:  "secure binding metadata is ready",
		}},
	}

	if err := binding.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestSecureBindingRejectsDuplicateCredentialNames(t *testing.T) {
	binding := SecureBinding{
		Credentials: []CredentialReference{
			{Name: "database", Reference: "baseharbor://credential/one"},
			{Name: "database", Reference: "baseharbor://credential/two"},
		},
	}
	if err := binding.Validate(); err == nil {
		t.Fatal("duplicate credential name accepted")
	}
}

func TestSecureBindingRejectsEmptyReferences(t *testing.T) {
	binding := SecureBinding{
		Secrets: []SecretReference{{Name: "API_TOKEN"}},
	}
	if err := binding.Validate(); err == nil {
		t.Fatal("empty secret reference accepted")
	}
}

func TestSecureBindingRejectsControlCharactersInReferences(t *testing.T) {
	binding := SecureBinding{
		Credentials: []CredentialReference{{
			Name:      "database",
			Reference: "baseharbor://credential/mailflow\nleak",
		}},
	}
	if err := binding.Validate(); err == nil {
		t.Fatal("credential reference containing newline accepted")
	}
}

func TestSecureBindingRejectsInvalidDiagnosticSeverity(t *testing.T) {
	binding := SecureBinding{
		Diagnostics: []Diagnostic{{
			Code:     "binding-state",
			Severity: Severity("debug"),
			Message:  "unexpected",
		}},
	}
	if err := binding.Validate(); err == nil {
		t.Fatal("invalid diagnostic severity accepted")
	}
}

func TestSecureBindingRejectsCredentialBearingReference(t *testing.T) {
	binding := SecureBinding{
		Credentials: []CredentialReference{{
			Name:      "database",
			Reference: "https://user:password@example.invalid/credential",
		}},
	}
	if err := binding.Validate(); err == nil {
		t.Fatal("credential-bearing URL accepted as secure binding reference")
	}
}
