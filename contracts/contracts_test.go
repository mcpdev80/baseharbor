package contracts_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const jsonSchema202012 = "https://json-schema.org/draft/2020-12/schema"

func TestContractSchemasUseJSONSchema202012(t *testing.T) {
	var files []string
	err := filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".schema.json") {
			return nil
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no contract schemas found")
	}
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var schema map[string]any
		if err := json.Unmarshal(data, &schema); err != nil {
			t.Fatalf("%s: invalid JSON: %v", path, err)
		}
		if schema["$schema"] != jsonSchema202012 {
			t.Fatalf("%s: $schema=%v, want %s", path, schema["$schema"], jsonSchema202012)
		}
		if strings.TrimSpace(stringValue(schema["$id"])) == "" {
			t.Fatalf("%s: missing $id", path)
		}
	}
}

func TestServiceSchemasStayProviderNeutral(t *testing.T) {
	forbidden := []string{
		"postgresql",
		"valkey",
		"redis",
		"seaweedfs",
		"openbao",
		"prometheus",
		"loki",
		"tempo",
		"grafana",
		"keycloak",
	}
	files, err := filepath.Glob("service/v1/*.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		lower := strings.ToLower(string(data))
		for _, product := range forbidden {
			if strings.Contains(lower, product) {
				t.Fatalf("%s leaks provider/product name %q", path, product)
			}
		}
	}
}

func TestServiceBindingUsesWellKnownNames(t *testing.T) {
	data, err := os.ReadFile("binding/v1/service-binding.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	var schema struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"type", "provider", "host", "port", "uri",
		"username", "password", "certificates", "private-key",
	} {
		if _, ok := schema.Properties[name]; !ok {
			t.Fatalf("Service Binding well-known property %q missing", name)
		}
	}
	for _, alias := range []string{"hostname", "connectionHost", "user", "pass", "connectionString"} {
		if _, ok := schema.Properties[alias]; ok {
			t.Fatalf("non-standard Service Binding alias %q present", alias)
		}
	}
}

func stringValue(v any) string {
	s, _ := v.(string)
	return s
}
