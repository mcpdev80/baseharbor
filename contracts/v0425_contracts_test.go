package contracts_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/mcpdev80/baseharbor/contracts"
	"github.com/mcpdev80/baseharbor/internal/application"
)

func contractSchema(t *testing.T, path string) *jsonschema.Resolved {
	t.Helper()
	raw, err := contracts.ReadSchema(contracts.Namespace + path)
	if err != nil {
		t.Fatal(err)
	}
	var schema jsonschema.Schema
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatal(err)
	}
	resolved, err := schema.Resolve(nil)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

func TestFootprintSchemaMatchesDomainFixtures(t *testing.T) {
	resolved := contractSchema(t, "footprint/v1/footprint.schema.json")
	raw, err := os.ReadFile("../internal/application/testdata/v0425-resolution.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []struct {
		Name        string                           `json:"name"`
		Manifest    string                           `json:"manifest"`
		Snapshot    application.ResolutionSnapshot   `json:"snapshot"`
		Preferences []application.ProviderPreference `json:"preferences"`
		ErrorCode   string                           `json:"error_code"`
	}
	if err := json.Unmarshal(raw, &fixtures); err != nil {
		t.Fatal(err)
	}
	for _, f := range fixtures {
		if f.ErrorCode != "" {
			continue
		}
		t.Run(f.Name, func(t *testing.T) {
			m, err := application.ParseYAML(f.Manifest)
			if err != nil {
				t.Fatal(err)
			}
			fp, err := application.ResolveFootprint(m, f.Snapshot, f.Preferences)
			if err != nil {
				t.Fatal(err)
			}
			data, err := json.Marshal(fp)
			if err != nil {
				t.Fatal(err)
			}
			var value any
			if err := json.Unmarshal(data, &value); err != nil {
				t.Fatal(err)
			}
			if err := resolved.Validate(value); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestConnectionProfileSchemaRejectsAuthorityAndLocalState(t *testing.T) {
	resolved := contractSchema(t, "connection-profile/v1/profile.schema.json")
	input := map[string]any{
		"version": "baseharbor.connection-profile/v1", "name": "homelab",
		"core":    map[string]any{"installation_id": "11111111-1111-4111-8111-111111111111", "url": "https://core.example.test"},
		"trust":   map[string]any{"ca_pem": "-----BEGIN CERTIFICATE-----\nPUBLIC-TRANSPORT-FIXTURE", "sha256": "0000000000000000000000000000000000000000000000000000000000000000"},
		"targets": []any{},
	}
	// Schema acceptance is transport-shape only, not certificate/trust acceptance.
	if err := resolved.Validate(input); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"token", "password", "private_key", "docker_endpoint", "application_id", "roles"} {
		input[field] = "forbidden"
		if err := resolved.Validate(input); err == nil {
			t.Fatalf("accepted %s", field)
		}
		delete(input, field)
	}
	input["core"].(map[string]any)["url"] = "http://core.example.test"
	if err := resolved.Validate(input); err == nil {
		t.Fatal("accepted insecure management URL")
	}
	input["core"].(map[string]any)["url"] = "https://core.example.test"
	input["version"] = "baseharbor.connection-profile/v99"
	if err := resolved.Validate(input); err == nil {
		t.Fatal("accepted unknown transport version")
	}
}
