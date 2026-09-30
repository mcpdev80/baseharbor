package application

import (
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

func TestDurableKeyValueContractIsDistinctFromCache(t *testing.T) {
	if capability.KeyValue == capability.DurableKeyValue {
		t.Fatal("cache and durable key-value capability identities must differ")
	}
	if capability.KeyValueV1.ID != "cache.key-value/v1" {
		t.Fatalf("cache spec = %q", capability.KeyValueV1.ID)
	}
	if capability.DurableKeyValueV1.ID != "database.key-value/v1" {
		t.Fatalf("durable key-value spec = %q", capability.DurableKeyValueV1.ID)
	}
	if !capability.Valkey.Supports(capability.KeyValue) || !capability.Valkey.Supports(capability.DurableKeyValue) {
		t.Fatalf("Valkey capabilities = %#v", capability.Valkey.Capabilities)
	}
	cacheService, err := capability.ServiceKindForCapability(capability.KeyValue)
	if err != nil {
		t.Fatal(err)
	}
	durableService, err := capability.ServiceKindForCapability(capability.DurableKeyValue)
	if err != nil {
		t.Fatal(err)
	}
	if cacheService == durableService {
		t.Fatalf("cache and durable key-value share service kind %q", cacheService)
	}
}

func TestDurableKeyValueManifestRoundTripAndPortableContract(t *testing.T) {
	m := Manifest{
		Version:       CurrentVersion,
		ApplicationID: MustNewApplicationID(),
		Name:          "ledger",
		Environment:   "dev",
		Services: Services{
			KeyValue:          true,
			KeyValueInstances: map[string]ServiceInstance{"sessions": {}},
		},
	}
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
	rendered := m.YAML()
	if !strings.Contains(rendered, "  key_value:\n") || !strings.Contains(rendered, "      sessions: {}\n") {
		t.Fatalf("durable key-value service missing from manifest:\n%s", rendered)
	}
	got, err := ParseYAML(rendered)
	if err != nil {
		t.Fatal(err)
	}
	if names := KeyValueInstanceNames(got); len(names) != 1 || names[0] != "sessions" {
		t.Fatalf("durable key-value instances = %#v", names)
	}

	contract, err := PortableContractFromManifest(got)
	if err != nil {
		t.Fatal(err)
	}
	var durable, cache bool
	for _, requirement := range contract.Capabilities {
		switch requirement.Kind {
		case capability.DurableKeyValue:
			durable = requirement.Name == "sessions"
		case capability.KeyValue:
			cache = true
		}
	}
	if !durable {
		t.Fatalf("portable contract has no durable key-value requirement: %#v", contract.Capabilities)
	}
	if cache {
		t.Fatalf("durable key-value intent was collapsed into cache: %#v", contract.Capabilities)
	}

	resources, err := ResolveCapabilityResources(contract)
	if err != nil {
		t.Fatal(err)
	}
	if len(resources) != 1 || resources[0].Provider != capability.ProviderValkey || resources[0].Kind != capability.DurableKeyValue {
		t.Fatalf("resolved resources = %#v", resources)
	}

	plan, err := BuildPlan(got)
	if err != nil {
		t.Fatal(err)
	}
	var ensure, verify bool
	for _, action := range plan.Actions {
		if action.Resource != "database.key-value:sessions" {
			continue
		}
		switch action.Kind {
		case "ensure":
			ensure = strings.Contains(action.Description, "durable")
		case "verify":
			verify = strings.Contains(action.Description, "write/read")
		}
	}
	if !ensure || !verify {
		t.Fatalf("durable key-value plan = %#v", plan.Actions)
	}
}
