package contracts_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
				"prometheus", "loki", "tempo", "keycloak",
			} {
				if strings.Contains(lower, product) {
					t.Fatalf("portable service schema leaks product name %q", product)
				}
			}
		})
	}
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
