package contracts_test

import (
	"net/url"
	"strings"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/mcpdev80/baseharbor/contracts"
)

// TestConnectionProfileSchemaValidBaseline accepts a schema-valid
// connection-profile/v1 descriptor. The required top-level fields
// version/name/core/trust/targets and the nested required sets codex listed
// (core.installation_id/url, trust.ca_pem/sha256,
// targets[].name/tenant_id/runtime_provider) are all present here, so the
// descriptor resolves and validates. This is complementary to
// TestConnectionProfileSchemaRejectsAuthorityAndLocalState in
// contracts/v0425_contracts_test.go, which exercises disallowed extra fields,
// insecure management URLs and an unknown version on the same schema; it does
// not remove individual required fields, which the negatives below cover.
func TestConnectionProfileSchemaValidBaseline(t *testing.T) {
	resolved := connectionProfileSchema(t)
	descriptor := map[string]any{
		"version": "baseharbor.connection-profile/v1",
		"name":    "homelab",
		"core": map[string]any{
			"installation_id": "11111111-1111-4111-8111-111111111111",
			"url":             "https://core.example.test",
		},
		"trust": map[string]any{
			"ca_pem": "-----BEGIN CERTIFICATE-----\nPUBLIC-TRANSPORT-FIXTURE",
			"sha256": "0000000000000000000000000000000000000000000000000000000000000000",
		},
		"targets": []any{
			map[string]any{
				"name":             "lab-core",
				"tenant_id":        "11111111-1111-4111-8111-111111111111",
				"runtime_provider": "baseharbor",
				"scope":            "control-plane",
			},
		},
	}
	if err := resolved.Validate(descriptor); err != nil {
		t.Fatalf("schema-valid connection-profile/v1 descriptor rejected: %v", err)
	}
}

// TestConnectionProfileSchemaRejectsMissingTopLevelRequired removes each
// top-level required field from a schema-valid descriptor and asserts the
// schema rejects the descriptor for that reason. The jsonschema-go required
// constraint names the missing property (e.g. "required: missing properties:
// [\"name\"]"), so each case asserts the field name appears in the error.
func TestConnectionProfileSchemaRejectsMissingTopLevelRequired(t *testing.T) {
	resolved := connectionProfileSchema(t)
	base := connectionProfileBaseline(t, resolved)

	for _, field := range []string{"version", "name", "core", "trust", "targets"} {
		mutated := cloneObject(base)
		delete(mutated, field)
		err := resolved.Validate(mutated)
		if err == nil {
			t.Fatalf("descriptor schema accepted a descriptor missing required top-level field %q", field)
		}
		if !strings.Contains(err.Error(), field) {
			t.Fatalf("missing top-level %q: error %q does not name the required constraint", field, err.Error())
		}
	}
}

// TestConnectionProfileSchemaRejectsMissingNestedRequired removes one field
// from a required nested object and asserts the schema rejects the descriptor
// for that reason. It covers core.installation_id/url, trust.ca_pem/sha256 and
// the required field set of a single targets[] item (name/tenant_id/
// runtime_provider).
func TestConnectionProfileSchemaRejectsMissingNestedRequired(t *testing.T) {
	resolved := connectionProfileSchema(t)
	base := connectionProfileBaseline(t, resolved)

	nested := func(container map[string]any, field string) map[string]any {
		return map[string]any{
			"version": "baseharbor.connection-profile/v1",
			"name":    "homelab",
			"core":    cloneObject(base["core"].(map[string]any)),
			"trust":   cloneObject(base["trust"].(map[string]any)),
			"targets": []any{},
		}
	}
	coreTrustCases := []struct{ container, field string }{
		{"core", "installation_id"},
		{"core", "url"},
		{"trust", "ca_pem"},
		{"trust", "sha256"},
	}
	for _, c := range coreTrustCases {
		obj := cloneObject(base[c.container].(map[string]any))
		delete(obj, c.field)
		descriptor := nested(base[c.container].(map[string]any), c.field)
		descriptor[c.container] = obj
		err := resolved.Validate(descriptor)
		if err == nil {
			t.Fatalf("descriptor schema accepted a %s descriptor missing required field %q", c.container, c.field)
		}
		if !strings.Contains(err.Error(), c.field) {
			t.Fatalf("%s.%s: error %q does not name the required constraint", c.container, c.field, err.Error())
		}
	}

	targetItem := func(rem string) map[string]any {
		item := cloneObject(base["targets"].([]any)[0].(map[string]any))
		delete(item, rem)
		return map[string]any{
			"version": "baseharbor.connection-profile/v1",
			"name":    "homelab",
			"core":    cloneObject(base["core"].(map[string]any)),
			"trust":   cloneObject(base["trust"].(map[string]any)),
			"targets": []any{item},
		}
	}
	for _, field := range []string{"name", "tenant_id", "runtime_provider"} {
		err := resolved.Validate(targetItem(field))
		if err == nil {
			t.Fatalf("descriptor schema accepted a targets[] item missing required field %q", field)
		}
		if !strings.Contains(err.Error(), field) {
			t.Fatalf("targets[].%s: error %q does not name the required constraint", field, err.Error())
		}
	}
}

// TestConnectionProfileSchemaRejectsUnknownField keeps the field-presence
// baseline valid and asserts the schema rejects a disallowed additional
// property, complementing the extra-field coverage in
// TestConnectionProfileSchemaRejectsAuthorityAndLocalState.
func TestConnectionProfileSchemaRejectsUnknownField(t *testing.T) {
	resolved := connectionProfileSchema(t)
	base := connectionProfileBaseline(t, resolved)
	base["unknown"] = "forbidden"
	if err := resolved.Validate(base); err == nil {
		t.Fatal("connection-profile schema accepted a disallowed additional property")
	}
}

func connectionProfileSchema(t *testing.T) *jsonschema.Resolved {
	t.Helper()
	raw, err := contracts.ReadSchema(contracts.Namespace + "connection-profile/v1/profile.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := profileResolve(raw)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

func connectionProfileBaseline(t *testing.T, resolved *jsonschema.Resolved) map[string]any {
	t.Helper()
	descriptor := map[string]any{
		"version": "baseharbor.connection-profile/v1",
		"name":    "homelab",
		"core": map[string]any{
			"installation_id": "11111111-1111-4111-8111-111111111111",
			"url":             "https://core.example.test",
		},
		"trust": map[string]any{
			"ca_pem": "-----BEGIN CERTIFICATE-----\nPUBLIC-TRANSPORT-FIXTURE",
			"sha256": "0000000000000000000000000000000000000000000000000000000000000000",
		},
		"targets": []any{
			map[string]any{
				"name":             "lab-core",
				"tenant_id":        "11111111-1111-4111-8111-111111111111",
				"runtime_provider": "baseharbor",
			},
		},
	}
	if err := resolved.Validate(descriptor); err != nil {
		t.Fatalf("schema-valid connection-profile/v1 descriptor rejected by the packaged schema: %v", err)
	}
	return cloneObject(descriptor)
}

func cloneObject(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func newSchema(data []byte) (*jsonschema.Schema, error) {
	var s jsonschema.Schema
	if err := s.UnmarshalJSON(data); err != nil {
		return nil, err
	}
	return &s, nil
}

// TestExtensionDescriptorArtifactEmptyObjectRejectsOciReference qualifies the
// extension/v1 artifact contract: artifact is optional, but once present its
// oci_reference is required. A schema-valid baseline is asserted once, then the
// artifact is replaced by an empty object while keeping a valid configuration
// schema dialect; the packaged schema must reject it for the missing
// oci_reference.
func TestExtensionDescriptorArtifactEmptyObjectRejectsOciReference(t *testing.T) {
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

	// The artifact object is present but empty: with oci_reference absent the
	// packaged schema must reject it for that reason.
	emptyArtifact := map[string]any{
		"schema_version":       "baseharbor.extension/v1",
		"id":                   "example.com/org.example-extension",
		"family":               "provider",
		"version":              "0.1.0",
		"artifact":             map[string]any{},
		"configuration_schema": map[string]any{"dialect": "https://json-schema.org/draft/2020-12/schema"},
	}
	if err := resolved.Validate(emptyArtifact); err == nil {
		t.Fatal("descriptor schema accepted an artifact object without the required oci_reference")
	}
}

// TestExtensionDescriptorArtifactIsOptional qualifies the extension/v1 artifact
// contract in the positive direction: artifact is optional, so a schema-valid
// descriptor that omits it entirely must be accepted by the packaged schema.
func TestExtensionDescriptorArtifactIsOptional(t *testing.T) {
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
	omittedArtifact := map[string]any{
		"schema_version":       "baseharbor.extension/v1",
		"id":                   "example.com/org.example-extension",
		"family":               "provider",
		"version":              "0.1.0",
		"configuration_schema": map[string]any{"dialect": "https://json-schema.org/draft/2020-12/schema"},
	}
	if err := resolved.Validate(omittedArtifact); err != nil {
		t.Fatalf("schema rejected a descriptor missing the optional artifact: %v", err)
	}
}

func profileResolve(data []byte) (*jsonschema.Resolved, error) {
	var s jsonschema.Schema
	if err := s.UnmarshalJSON(data); err != nil {
		return nil, err
	}
	return s.Resolve(&jsonschema.ResolveOptions{Loader: func(uri *url.URL) (*jsonschema.Schema, error) {
		raw, err := contracts.ReadSchema(uri.String())
		if err != nil {
			return nil, err
		}
		var inner jsonschema.Schema
		if err := inner.UnmarshalJSON(raw); err != nil {
			return nil, err
		}
		return &inner, nil
	}})
}
