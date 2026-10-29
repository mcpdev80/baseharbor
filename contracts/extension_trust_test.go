package contracts_test

import (
	"net/url"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/mcpdev80/baseharbor/contracts"
)

// TestConnectionProfileSchemaRejectsAuthorityAndLocalState already exercises the
// connection-profile slice; this file qualifies the extension/v1 contract family,
// which ships two normative schemas but previously had zero behavioral coverage.
// The baseline descriptor is schema-valid; each negative case mutates exactly one
// required field and asserts that the schema rejects it for that reason.
func TestExtensionDescriptorSchemaRejectsGoValidatorDivergence(t *testing.T) {
	descriptor, err := contracts.ReadSchema("https://github.com/mcpdev80/baseharbor/blob/HEAD/contracts/extension/v1/extension-descriptor.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	schema, err := newSchema(descriptor)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := schema.Resolve(nil)
	if err != nil {
		t.Fatal(err)
	}

	base := map[string]any{
		"schema_version":       "baseharbor.extension/v1",
		"id":                   "example.com/org.example-extension",
		"family":               "provider",
		"version":              "0.1.0",
		"artifact":             map[string]any{"oci_reference": "ghcr.io/example/org.example-extension@sha256:0000000000000000000000000000000000000000000000000000000000000000"},
		"configuration_schema": map[string]any{"dialect": "https://json-schema.org/draft/2020-12/schema"},
	}
	if err := resolved.Validate(base); err != nil {
		t.Fatalf("schema-valid extension descriptor rejected by the packaged schema: %v", err)
	}

	// A descriptor valid under the Go metadata validator is still rejected by the
	// packaged schema when the required oci_reference and configuration_schema.dialect
	// fields are removed; the schema, not the Go validator, closes the boundary.
	missing := map[string]any{
		"schema_version":       "baseharbor.extension/v1",
		"id":                   "example.com/org.example-extension",
		"family":               "provider",
		"version":              "0.1.0",
		"configuration_schema": map[string]any{},
	}
	if err := resolved.Validate(missing); err == nil {
		t.Fatal("descriptor schema accepted a descriptor missing required artifact.oci_reference and configuration_schema.dialect")
	}
}

// TestExtensionDescriptorSchemaRejectsInvalidIdAndFamily closes the trust boundary
// at the public-contracts surface for the extension family by rejecting a valid
// descriptor that mutates exactly one constraint.
func TestExtensionDescriptorSchemaRejectsInvalidIdAndFamily(t *testing.T) {
	descriptor, err := contracts.ReadSchema("https://github.com/mcpdev80/baseharbor/blob/HEAD/contracts/extension/v1/extension-descriptor.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	schema, err := newSchema(descriptor)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := schema.Resolve(nil)
	if err != nil {
		t.Fatal(err)
	}
	base := map[string]any{
		"schema_version":       "baseharbor.extension/v1",
		"id":                   "example.com/org.example-extension",
		"family":               "provider",
		"version":              "0.1.0",
		"artifact":             map[string]any{"oci_reference": "ghcr.io/example/org.example-extension@sha256:0000000000000000000000000000000000000000000000000000000000000000"},
		"configuration_schema": map[string]any{"dialect": "https://json-schema.org/draft/2020-12/schema"},
	}
	for _, id := range []string{"UPPER/Name", "/leading-namespace", "x/-segment", "trailing-namespace/UPPER"} {
		got := clone(base)
		got["id"] = id
		if err := resolved.Validate(got); err == nil {
			t.Fatalf("descriptor schema accepted invalid id %q", id)
		}
	}
	for _, family := range []string{"unknown", "Provider", ""} {
		got := clone(base)
		got["family"] = family
		if err := resolved.Validate(got); err == nil {
			t.Fatalf("descriptor schema accepted out-of-enum family %q", family)
		}
	}
}

func newSchema(data []byte) (*jsonschema.Schema, error) {
	var s jsonschema.Schema
	if err := s.UnmarshalJSON(data); err != nil {
		return nil, err
	}
	return &s, nil
}

func extensionSchema(t *testing.T, path string) *jsonschema.Resolved {
	t.Helper()
	raw, err := contracts.ReadSchema(contracts.Namespace + path)
	if err != nil {
		t.Fatal(err)
	}
	schema, err := newSchema(raw)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := schema.Resolve(&jsonschema.ResolveOptions{Loader: func(uri *url.URL) (*jsonschema.Schema, error) {
		raw, err := contracts.ReadSchema(uri.String())
		if err != nil {
			return nil, err
		}
		s, err := newSchema(raw)
		if err != nil {
			return nil, err
		}
		return s, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

func clone(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
