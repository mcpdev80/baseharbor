package application

import (
	"reflect"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

func TestResolveCapabilityResourcesUsesReferenceProviders(t *testing.T) {
	m := Manifest{
		Version:     CurrentVersion,
		Name:        "mailflow",
		Environment: "dev",
		Services: Services{
			Postgres: true,
			Redis:    true,
			Secrets:  true,
		},
	}
	contract, err := PortableContractFromManifest(m)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ResolveCapabilityResources(contract)
	if err != nil {
		t.Fatal(err)
	}
	want := []capability.Resource{
		{Application: "mailflow", Kind: capability.SQL, Name: "default", Provider: capability.ProviderPostgreSQL},
		{Application: "mailflow", Kind: capability.KeyValue, Name: "default", Provider: capability.ProviderValkey},
		{Application: "mailflow", Kind: capability.Secrets, Name: "default", Provider: capability.ProviderOpenBao},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("resources = %#v, want %#v", got, want)
	}
}

func TestCapabilityBindingsUseStableApplicationWorkloadIdentity(t *testing.T) {
	m := Manifest{
		Version:     CurrentVersion,
		Name:        "mailflow",
		Environment: "production",
		Services:    Services{Postgres: true},
	}
	bindings, err := CapabilityBindings(m)
	if err != nil {
		t.Fatal(err)
	}
	if len(bindings) != 1 || bindings[0].Workload != "application/mailflow" {
		t.Fatalf("bindings = %#v", bindings)
	}
	if bindings[0].Resource.Provider != capability.ProviderPostgreSQL {
		t.Fatalf("provider = %q", bindings[0].Resource.Provider)
	}
}

func TestCapabilityBindingsUseLogicalServiceForHTTPExposure(t *testing.T) {
	m := New("frontend", "production", false, false, false)
	m.Services.Postgres = false
	m.Workload = WorkloadConfig{Compose: "compose.yaml", Services: []string{"web"}}
	m.Exposures = []HTTPExposureRequirement{{Name: "public", Service: "web", Port: 8080, Protocol: "http"}}

	bindings, err := CapabilityBindings(m)
	if err != nil {
		t.Fatal(err)
	}
	if len(bindings) != 1 {
		t.Fatalf("bindings = %#v", bindings)
	}
	if bindings[0].Resource.Kind != capability.ExposureHTTP || bindings[0].Resource.Provider != capability.ProviderCaddy {
		t.Fatalf("unexpected exposure resource %#v", bindings[0].Resource)
	}
	if bindings[0].Workload != "service/web" {
		t.Fatalf("exposure workload binding = %q, want service/web", bindings[0].Workload)
	}
	if bindings[0].HTTPExposure == nil {
		t.Fatal("exposure binding metadata is missing")
	}
	want := capability.HTTPExposureBinding{Service: "web", TargetPort: 8080, Protocol: "http", Visibility: "public"}
	if *bindings[0].HTTPExposure != want {
		t.Fatalf("exposure metadata = %#v, want %#v", *bindings[0].HTTPExposure, want)
	}
}

func TestExposureCapabilityUsesCaddyAndServiceBinding(t *testing.T) {
	m := Manifest{
		Version:     CurrentVersion,
		Name:        "frontend",
		Environment: "production",
		Workload: WorkloadConfig{
			Compose:  "compose.yaml",
			Services: []string{"web"},
		},
		Exposures: []HTTPExposureRequirement{
			{Name: "public", Service: "web", Port: 8080, Protocol: "http"},
		},
	}
	contract, err := PortableContractFromManifest(m)
	if err != nil {
		t.Fatal(err)
	}
	resources, err := ResolveCapabilityResources(contract)
	if err != nil {
		t.Fatal(err)
	}
	if len(resources) != 1 {
		t.Fatalf("resources = %#v", resources)
	}
	if resources[0].Kind != capability.ExposureHTTP || resources[0].Provider != capability.ProviderCaddy {
		t.Fatalf("unexpected exposure resource %#v", resources[0])
	}
	bindings, err := CapabilityBindings(m)
	if err != nil {
		t.Fatal(err)
	}
	if len(bindings) != 1 || bindings[0].Workload != "service/web" {
		t.Fatalf("unexpected exposure bindings %#v", bindings)
	}
}


func TestCapabilityBindingsAttachSecureMetadataToManagedSecrets(t *testing.T) {
	m := Manifest{
		Version:     CurrentVersion,
		Name:        "mailflow",
		Environment: "production",
		Services:    Services{Secrets: true},
		Secrets: SecretRequirements{Required: []SecretRequirement{
			{Name: "SMTP_PASSWORD"},
		}},
	}
	bindings, err := CapabilityBindings(m)
	if err != nil {
		t.Fatal(err)
	}
	if len(bindings) != 1 {
		t.Fatalf("bindings = %#v", bindings)
	}
	binding := bindings[0]
	if binding.Resource.Kind != capability.Secrets {
		t.Fatalf("resource kind = %q", binding.Resource.Kind)
	}
	if binding.Security == nil {
		t.Fatal("managed secrets binding is missing secure metadata")
	}
	if err := binding.Security.Validate(); err != nil {
		t.Fatalf("secure binding invalid: %v", err)
	}
	if binding.Security.Identity == nil || binding.Security.Identity.Subject != "spiffe://baseharbor/apps/mailflow/production" {
		t.Fatalf("identity = %#v", binding.Security.Identity)
	}
	if len(binding.Security.Secrets) != 1 || binding.Security.Secrets[0].Name != "SMTP_PASSWORD" {
		t.Fatalf("secrets = %#v", binding.Security.Secrets)
	}
}
