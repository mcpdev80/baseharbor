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


func TestManagedSecretsSecureBindingsAreApplicationAndEnvironmentScoped(t *testing.T) {
	alpha := ManagedSecretsSecureBinding(Manifest{
		Version: CurrentVersion, Name: "alpha", Environment: "production",
		Services: Services{Secrets: true},
	})
	beta := ManagedSecretsSecureBinding(Manifest{
		Version: CurrentVersion, Name: "beta", Environment: "production",
		Services: Services{Secrets: true},
	})
	alphaDev := ManagedSecretsSecureBinding(Manifest{
		Version: CurrentVersion, Name: "alpha", Environment: "dev",
		Services: Services{Secrets: true},
	})

	if alpha.Identity == nil || beta.Identity == nil || alphaDev.Identity == nil {
		t.Fatal("expected workload identities")
	}
	if alpha.Identity.Subject == beta.Identity.Subject || alpha.Identity.Reference == beta.Identity.Reference {
		t.Fatal("different applications share secure binding identity")
	}
	if alpha.Identity.Subject == alphaDev.Identity.Subject || alpha.Identity.Reference == alphaDev.Identity.Reference {
		t.Fatal("different environments share secure binding identity")
	}
	if alpha.Credentials[0].Reference == beta.Credentials[0].Reference ||
		alpha.Credentials[0].Reference == alphaDev.Credentials[0].Reference {
		t.Fatal("credential references are not application/environment scoped")
	}
}
