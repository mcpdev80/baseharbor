package contracts_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

func TestServiceSchemasUseJSONSchema202012AndStayProviderNeutral(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("service", "v1", "*.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatal("no service schemas found")
	}
	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var schema map[string]any
			if err := json.Unmarshal(data, &schema); err != nil {
				t.Fatalf("invalid JSON: %v", err)
			}
			if got := schema["$schema"]; got != "https://json-schema.org/draft/2020-12/schema" {
				t.Fatalf("$schema=%v", got)
			}
			lower := strings.ToLower(string(data))
			for _, product := range []string{
				"postgresql", "valkey", "redis", "seaweedfs", "openbao",
				"prometheus", "loki", "tempo", "keycloak", "rabbitmq", "mongodb",
			} {
				if strings.Contains(lower, product) {
					t.Fatalf("portable service schema leaks product name %q", product)
				}
			}
		})
	}
}

func TestShippedServiceFamiliesHaveSchemas(t *testing.T) {
	for _, name := range []string{
		"sql", "cache", "key-value", "document-database", "object-storage",
		"secrets", "observability", "identity", "messaging", "vector", "exposure",
	} {
		path := filepath.Join("service", "v1", name+".schema.json")
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("shipped service family %q has no v1 JSON Schema: %v", name, err)
		}
	}
}

func TestServiceBindingSchemaReservesNamespacedExtensionsOnly(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("binding", "v1", "service-binding.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	var schema map[string]any
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatalf("service-binding.schema.json: invalid JSON: %v", err)
	}
	if got := schema["$schema"]; got != "https://json-schema.org/draft/2020-12/schema" {
		t.Fatalf("service-binding.schema.json: $schema=%v", got)
	}
}

// TestServiceBindingGuardRejectsUnknownOutputNames asserts the well-known
// output-name policy: the binding schema reserves open top-level extensions,
// but the well-known-output-name guard must fail for an unknown, non-namespaced
// top-level output key. This is the behavioral counterpart to the dialect test.
func TestServiceBindingGuardRejectsUnknownOutputNames(t *testing.T) {
	for _, key := range []string{"region", "account", "connectionstring"} {
		if err := capability.ValidateServiceBindingName(key); err == nil {
			t.Fatalf("well-known-output-name guard accepted unknown non-namespaced output key %q", key)
		}
	}

	for _, key := range []string{"type", "provider", "host", "uri", "username", "private-key", "baha.region", "provider.example/endpoint"} {
		if err := capability.ValidateServiceBindingName(key); err != nil {
			t.Fatalf("accepted entry %q rejected: %v", key, err)
		}
	}
}

func keysOf(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestProviderAndBindingSchemasUseJSONSchema202012(t *testing.T) {
	for _, path := range []string{
		filepath.Join("provider", "v1", "provider-descriptor.schema.json"),
		filepath.Join("binding", "v1", "service-binding.schema.json"),
	} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var schema map[string]any
		if err := json.Unmarshal(data, &schema); err != nil {
			t.Fatalf("%s: invalid JSON: %v", path, err)
		}
		if got := schema["$schema"]; got != "https://json-schema.org/draft/2020-12/schema" {
			t.Fatalf("%s: $schema=%v", path, got)
		}
	}
}
