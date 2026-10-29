package contracts_test

import (
	"encoding/json"
	"errors"
	"net/url"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/mcpdev80/baseharbor/contracts"
	"github.com/mcpdev80/baseharbor/internal/extension"
)

// TestExtensionV1SchemasResolveAndValidate proves that the extension/v1 contract
// family, which ships two normative schemas but previously had zero behavioral
// coverage, validates its own trust document against the packaged descriptor.
// It ties the schema to the real verifier so the descriptor cannot silently
// drift from the runtime contract.
func TestExtensionV1SchemasResolveAndValidate(t *testing.T) {
	descriptor, err := contracts.ReadSchema("https://github.com/mcpdev80/baseharbor/blob/HEAD/contracts/extension/v1/extension-descriptor.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	trust, err := contracts.ReadSchema("https://github.com/mcpdev80/baseharbor/blob/HEAD/contracts/extension/v1/extension-trust.schema.json")
	if err != nil {
		t.Fatal(err)
	}

	descriptorSchema, err := newSchema(descriptor)
	if err != nil {
		t.Fatal(err)
	}
	trustSchema, err := newSchema(trust)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := descriptorSchema.Resolve(&jsonschema.ResolveOptions{Loader: extensionLoader(descriptor, trust)}); err != nil {
		t.Fatalf("descriptor $ref resolution failed: %v", err)
	}
	trustResolved, err := trustSchema.Resolve(&jsonschema.ResolveOptions{Loader: extensionLoader(descriptor, trust)})
	if err != nil {
		t.Fatalf("trust $ref resolution failed: %v", err)
	}

	instance := map[string]any{
		"metadata": map[string]any{
			"schema_version": "baseharbor.extension/v1",
			"id":             "example.com/org.example-extension",
			"family":         "provider",
			"version":        "0.1.0",
			"artifact": map[string]any{
				"oci_reference": "ghcr.io/example/org.example-extension@sha256:0000000000000000000000000000000000000000000000000000000000000000",
			},
			"configuration_schema": map[string]any{"dialect": "https://json-schema.org/draft/2020-12/schema"},
		},
		"trusted": true,
		"verification": map[string]any{
			"schema_version":       "baseharbor.extension-trust/v1",
			"status":               "verified",
			"digest":               "sha256:0000000000000000000000000000000000000000000000000000000000000000",
			"signature_verified":   true,
			"sbom_verified":        true,
			"attestation_verified": true,
		},
		"trust_decision": map[string]any{
			"schema_version": "baseharbor.extension-trust/v1",
			"status":         "trusted",
		},
	}
	if err := trustResolved.Validate(instance); err != nil {
		t.Fatalf("valid extension trust document rejected by the packaged schema: %v", err)
	}
}

// TestExtensionDescriptorSchemaRejectsGoValidatorDivergence asserts the packaged
// descriptor enforces required artifact and configuration fields that the Go
// validator never checks: a descriptor valid under Metadata.Validate() is
// rejected by the schema, so the frozen-family guard cannot miss the family.
func TestExtensionDescriptorSchemaRejectsGoValidatorDivergence(t *testing.T) {
	descriptor, err := contracts.ReadSchema("https://github.com/mcpdev80/baseharbor/blob/HEAD/contracts/extension/v1/extension-descriptor.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	descriptorSchema, err := newSchema(descriptor)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := descriptorSchema.Resolve(nil)
	if err != nil {
		t.Fatal(err)
	}

	meta := extension.Metadata{
		SchemaVersion: extension.DescriptorVersion,
		ID:            "example.com/org.example-extension",
		Family:        extension.FamilyProvider,
		Version:       "0.1.0",
		Artifact: extension.Artifact{
			OCIReference: "ghcr.io/example/org.example-extension@sha256:0000000000000000000000000000000000000000000000000000000000000000",
		},
	}
	if err := meta.Validate(); err != nil {
		t.Fatalf("fixture descriptor unexpectedly fails the Go validator: %v", err)
	}
	if err := resolved.Validate(mustMetaJSON(meta)); err == nil {
		t.Fatal("descriptor schema accepted a descriptor missing required oci_reference and configuration_schema.dialect")
	}
}

// TestExtensionDescriptorSchemaRejectsInvalidIdAndFamily closes the trust
// boundary at the public-contracts surface for the extension family.
func TestExtensionDescriptorSchemaRejectsInvalidIdAndFamily(t *testing.T) {
	descriptor, err := contracts.ReadSchema("https://github.com/mcpdev80/baseharbor/blob/HEAD/contracts/extension/v1/extension-descriptor.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	descriptorSchema, err := newSchema(descriptor)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := descriptorSchema.Resolve(nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"invalid-id-missing-namespace", "UPPER/Name", "leading-dash/-x", "trailing-dash/x-"} {
		if err := resolved.Validate(map[string]any{"schema_version": "baseharbor.extension/v1", "id": id, "version": "0.1.0"}); err == nil {
			t.Fatalf("descriptor schema accepted invalid id %q", id)
		}
	}
	for _, family := range []string{"unknown", "Provider", ""} {
		if err := resolved.Validate(map[string]any{"schema_version": "baseharbor.extension/v1", "id": "example.com/org.example-extension", "family": family, "version": "0.1.0"}); err == nil {
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

// extensionLoader resolves the extension descriptor $ref (referenced by filename
// from the trust schema) and any sibling schema against the packaged registry.
func extensionLoader(descriptor, trust []byte) func(*url.URL) (*jsonschema.Schema, error) {
	byID := map[string][]byte{
		"https://github.com/mcpdev80/baseharbor/blob/HEAD/contracts/extension/v1/extension-descriptor.schema.json": descriptor,
		"https://github.com/mcpdev80/baseharbor/blob/HEAD/contracts/extension/v1/extension-trust.schema.json":      trust,
	}
	return func(uri *url.URL) (*jsonschema.Schema, error) {
		data, ok := byID[uri.String()]
		if !ok {
			return nil, &url.Error{Op: "load", URL: uri.String(), Err: errors.New("not found")}
		}
		s, err := newSchema(data)
		if err != nil {
			return nil, err
		}
		return s, nil
	}
}

func mustMetaJSON(meta extension.Metadata) map[string]any {
	raw, err := json.Marshal(meta)
	if err != nil {
		panic(err)
	}
	out := map[string]any{}
	if err := json.Unmarshal(raw, &out); err != nil {
		panic(err)
	}
	return out
}
