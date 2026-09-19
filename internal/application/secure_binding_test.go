package application

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestManagedSecretsSecureBindingUsesProviderNeutralReferences(t *testing.T) {
	m := Manifest{
		Version:     CurrentVersion,
		Name:        "mailflow",
		Environment: "production",
		Services:    Services{Secrets: true},
		Secrets: SecretRequirements{Required: []SecretRequirement{
			{Name: "SMTP_PASSWORD"},
			{Name: "OPENAI_API_KEY"},
		}},
	}

	binding := ManagedSecretsSecureBinding(m)
	if err := binding.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if binding.Identity == nil || binding.Identity.Subject != "spiffe://baseharbor/apps/mailflow/production" {
		t.Fatalf("identity = %#v", binding.Identity)
	}
	if len(binding.Secrets) != 2 {
		t.Fatalf("secrets = %#v", binding.Secrets)
	}
	data, err := json.Marshal(binding)
	if err != nil {
		t.Fatal(err)
	}
	serialized := string(data)
	for _, forbidden := range []string{"openbao", "approle", "secret_id", "role_id", "policy"} {
		if strings.Contains(strings.ToLower(serialized), forbidden) {
			t.Fatalf("secure binding leaked provider-specific concept %q: %s", forbidden, serialized)
		}
	}
}

func TestManagedSecretsSecureBindingContainsNoSecretValues(t *testing.T) {
	m := Manifest{
		Version:     CurrentVersion,
		Name:        "demo",
		Environment: "dev",
		Services:    Services{Secrets: true},
		Secrets: SecretRequirements{Required: []SecretRequirement{
			{Name: "API_TOKEN"},
		}},
	}
	binding := ManagedSecretsSecureBinding(m)
	data, err := json.Marshal(binding)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "super-secret-value") {
		t.Fatal("secure binding contains secret material")
	}
}
