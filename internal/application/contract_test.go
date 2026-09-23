package application

import (
	"reflect"
	"testing"
)

func TestPortableContractFromManifestMapsLogicalCapabilities(t *testing.T) {
	m := Manifest{
		Version:     CurrentVersion,
		Name:        "mailflow",
		Environment: "production",
		Services: Services{
			SQLInstances: map[string]ServiceInstance{
				"primary":   {},
				"analytics": {},
			},
			CacheInstances: map[string]ServiceInstance{
				"cache":    {},
				"sessions": {},
			},
			Secrets: true,
		},
		Secrets: SecretRequirements{Required: []SecretRequirement{
			{Name: "OPENAI_API_KEY"},
			{Name: "SESSION_SECRET", Generate: &SecretGeneration{Type: "random", Length: 32}},
		}},
	}

	contract, err := PortableContractFromManifest(m)
	if err != nil {
		t.Fatalf("PortableContractFromManifest() error = %v", err)
	}

	wantCapabilities := []CapabilityRequirement{
		{Kind: CapabilitySQL, Name: "analytics"},
		{Kind: CapabilitySQL, Name: "primary"},
		{Kind: CapabilityKeyValue, Name: "cache"},
		{Kind: CapabilityKeyValue, Name: "sessions"},
	}
	if !reflect.DeepEqual(contract.Capabilities, wantCapabilities) {
		t.Fatalf("Capabilities = %#v, want %#v", contract.Capabilities, wantCapabilities)
	}
	if contract.Application != "mailflow" {
		t.Fatalf("Application = %q, want mailflow", contract.Application)
	}
	if !contract.Secrets.Managed {
		t.Fatal("Secrets.Managed = false, want true")
	}
	if !reflect.DeepEqual(contract.Secrets.Required, m.Secrets.Required) {
		t.Fatalf("Secrets.Required = %#v, want %#v", contract.Secrets.Required, m.Secrets.Required)
	}
}

func TestPortableContractExcludesDeploymentContext(t *testing.T) {
	base := Manifest{
		Version: CurrentVersion,
		Name:    "mailflow",
		Services: Services{
			SQL: true,
		},
	}
	dev := base
	dev.Environment = "dev"
	production := base
	production.Environment = "production"

	devContract, err := PortableContractFromManifest(dev)
	if err != nil {
		t.Fatalf("dev contract error = %v", err)
	}
	productionContract, err := PortableContractFromManifest(production)
	if err != nil {
		t.Fatalf("production contract error = %v", err)
	}
	if !reflect.DeepEqual(devContract, productionContract) {
		t.Fatalf("deployment context leaked into portable contract: dev=%#v production=%#v", devContract, productionContract)
	}
}

func TestPortableContractExcludesComposeWorkloadDetails(t *testing.T) {
	m := Manifest{
		Version:     CurrentVersion,
		Name:        "frontend",
		Environment: "dev",
		Workload: WorkloadConfig{
			Compose:  "compose.yaml",
			Services: []string{"web"},
		},
	}

	contract, err := PortableContractFromManifest(m)
	if err != nil {
		t.Fatalf("PortableContractFromManifest() error = %v", err)
	}
	if len(contract.Capabilities) != 0 {
		t.Fatalf("Capabilities = %#v, want none for workload-only manifest", contract.Capabilities)
	}
	if contract.Secrets.Managed || len(contract.Secrets.Required) != 0 {
		t.Fatalf("Secrets = %#v, want empty", contract.Secrets)
	}
}

func TestPortableContractCopiesSecretGeneration(t *testing.T) {
	m := Manifest{
		Version:     CurrentVersion,
		Name:        "mailflow",
		Environment: "dev",
		Services:    Services{SQL: true, Secrets: true},
		Secrets: SecretRequirements{Required: []SecretRequirement{
			{Name: "SESSION_SECRET", Generate: &SecretGeneration{Type: "random", Length: 32}},
		}},
	}

	contract, err := PortableContractFromManifest(m)
	if err != nil {
		t.Fatalf("PortableContractFromManifest() error = %v", err)
	}
	contract.Secrets.Required[0].Generate.Length = 64
	if got := m.Secrets.Required[0].Generate.Length; got != 32 {
		t.Fatalf("manifest generation mutated through contract copy: got %d, want 32", got)
	}
}

func TestPortableContractRejectsInvalidManifest(t *testing.T) {
	_, err := PortableContractFromManifest(Manifest{Version: CurrentVersion, Name: "Bad Name", Environment: "dev"})
	if err == nil {
		t.Fatal("PortableContractFromManifest() error = nil, want validation error")
	}
}

func TestPortableContractIncludesProviderNeutralExposureIntent(t *testing.T) {
	m := Manifest{
		Version:     CurrentVersion,
		Name:        "frontend",
		Environment: "production",
		Workload:    WorkloadConfig{Compose: "compose.yaml", Services: []string{"web"}},
		Exposures:   []HTTPExposureRequirement{{Name: "public", Service: "web", Port: 8080, Protocol: "http"}},
	}
	contract, err := PortableContractFromManifest(m)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(contract.Exposures, m.Exposures) {
		t.Fatalf("exposure intent mismatch: got %#v want %#v", contract.Exposures, m.Exposures)
	}
	if !reflect.DeepEqual(contract.Capabilities, []CapabilityRequirement{{Kind: CapabilityExposureHTTP, Name: "public"}}) {
		t.Fatalf("unexpected exposure capabilities %#v", contract.Capabilities)
	}
}

func TestPortableContractIncludesMetricsSignalSourceWithoutProviderProduct(t *testing.T) {
	m := Manifest{
		Version:     CurrentVersion,
		Name:        "api",
		Environment: "dev",
		Workload:    WorkloadConfig{Compose: "compose.yaml", Services: []string{"api"}},
		Metrics: MetricsRequirements{Sources: []MetricsSourceRequirement{{
			Name: "application", Service: "api", Port: 8080, Path: "/metrics",
		}}},
	}
	contract, err := PortableContractFromManifest(m)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(contract.Capabilities, []CapabilityRequirement{{Kind: CapabilityMetrics, Name: "application"}}) {
		t.Fatalf("metrics capabilities = %#v", contract.Capabilities)
	}
	if !reflect.DeepEqual(contract.Metrics, m.Metrics.Sources) {
		t.Fatalf("metrics contract = %#v, want %#v", contract.Metrics, m.Metrics.Sources)
	}
}

func TestPortableContractCarriesExplicitLogsIntent(t *testing.T) {
	m := New("demo", "dev", false, false, false)
	m.Services.SQL = false
	m = WithWorkload(m, "compose.yaml", "api", "worker")
	m = WithLogsCollection(m, "application")

	contract, err := PortableContractFromManifest(m)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(contract.Logs, []string{"application"}) {
		t.Fatalf("logs intent = %#v", contract.Logs)
	}
	var got []CapabilityRequirement
	for _, requirement := range contract.Capabilities {
		if requirement.Kind == CapabilityLogs {
			got = append(got, requirement)
		}
	}
	want := []CapabilityRequirement{
		{Kind: CapabilityLogs, Name: "api"},
		{Kind: CapabilityLogs, Name: "worker"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("logs capabilities = %#v, want %#v", got, want)
	}
}

func TestPortableContractDoesNotInventProviderCapabilities(t *testing.T) {
	m := New("demo", "dev", true, true, true)
	m = WithWorkload(m, "compose.yaml", "api")

	contract, err := PortableContractFromManifest(m)
	if err != nil {
		t.Fatal(err)
	}
	for _, requirement := range contract.Capabilities {
		switch requirement.Kind {
		case CapabilityMetrics, CapabilityLogs, CapabilityTelemetryOTLP, CapabilityExposureHTTP, CapabilityObjectStorageS3:
			t.Fatalf("undeclared provider capability invented: %#v", requirement)
		}
	}
}

func TestCapabilityBindingsIncludeLogsOnlyWhenDeclared(t *testing.T) {
	m := New("demo", "dev", false, false, false)
	m.Services.SQL = false
	m = WithWorkload(m, "compose.yaml", "api")

	bindings, err := CapabilityBindings(m)
	if err != nil {
		t.Fatal(err)
	}
	for _, binding := range bindings {
		if binding.Resource.Kind == CapabilityLogs || binding.Logs != nil {
			t.Fatalf("undeclared logs binding invented: %#v", binding)
		}
	}

	m = WithLogsCollection(m, "application")
	bindings, err = CapabilityBindings(m)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, binding := range bindings {
		if binding.Resource.Kind == CapabilityLogs {
			found = true
			if binding.Workload != "service/api" || binding.Logs == nil || binding.Logs.Service != "api" {
				t.Fatalf("unexpected logs binding: %#v", binding)
			}
		}
	}
	if !found {
		t.Fatal("declared logs capability missing from bindings")
	}
}
