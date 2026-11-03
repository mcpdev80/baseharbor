// Package contracts_test validates the exact source/image submission contract:
// a candidate is a fixed source revision, image digest, or native compose/
// quadlet unit. A submission must carry exactly one immutable identity source
// (source revision OR image digest, never a mutable tag alone) and never a bare
// secret reference. Provenance is never inferred from annotations.
package contracts_test

import (
	_ "embed"
	"encoding/json"
	"net/url"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/mcpdev80/baseharbor/contracts"
)

const candidateSchemaPath = "candidate/v1/candidate-contract.schema.json"

//go:embed candidate/v1/golden-fixtures.json
var goldenFixtures []byte

//go:embed candidate/v1/negative-fixtures.json
var negativeFixtures []byte

type candidateFixture struct {
	Record  string          `json:"record"`
	Fixture json.RawMessage `json:"fixture"`
}

func candidateResolvedSchema(t *testing.T) *jsonschema.Resolved {
	t.Helper()
	raw, err := contracts.ReadSchema(contracts.Namespace + candidateSchemaPath)
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	var schema jsonschema.Schema
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("unmarshal schema: %v", err)
	}
	resolved, err := schema.Resolve(&jsonschema.ResolveOptions{Loader: func(uri *url.URL) (*jsonschema.Schema, error) {
		raw, err := contracts.ReadSchema(uri.String())
		if err != nil {
			return nil, err
		}
		var loaded jsonschema.Schema
		if err := json.Unmarshal(raw, &loaded); err != nil {
			return nil, err
		}
		return &loaded, nil
	}})
	if err != nil {
		t.Fatalf("resolve schema: %v", err)
	}
	return resolved
}

func TestCandidateGoldenFixturesAreExactAndValid(t *testing.T) {
	var fixtures []candidateFixture
	if err := json.Unmarshal(goldenFixtures, &fixtures); err != nil {
		t.Fatalf("golden fixtures: %v", err)
	}
	if len(fixtures) == 0 {
		t.Fatal("empty golden fixtures")
	}
	resolved := candidateResolvedSchema(t)
	for _, f := range fixtures {
		t.Run(f.Record, func(t *testing.T) {
			var value any
			if err := json.Unmarshal(f.Fixture, &value); err != nil {
				t.Fatalf("fixture is not valid JSON: %v", err)
			}
			if _, ok := value.(map[string]any); !ok {
				t.Fatalf("fixture is not a JSON object: %v", f.Record)
			}
			if err := resolved.Validate(value); err != nil {
				t.Fatalf("%s: valid candidate rejected: %v", f.Record, err)
			}
		})
	}
}

func TestCandidateNegativeFixturesAreRejected(t *testing.T) {
	var fixtures []candidateFixture
	if err := json.Unmarshal(negativeFixtures, &fixtures); err != nil {
		t.Fatalf("negative fixtures: %v", err)
	}
	resolved := candidateResolvedSchema(t)
	for _, f := range fixtures {
		t.Run(f.Record, func(t *testing.T) {
			var value any
			if err := json.Unmarshal(f.Fixture, &value); err != nil {
				t.Fatalf("fixture is not valid JSON: %v", err)
			}
			if _, ok := value.(map[string]any); !ok {
				t.Fatalf("fixture is not a JSON object: %v", f.Record)
			}
			if err := resolved.Validate(value); err == nil {
				t.Fatalf("%s: malformed candidate submission was accepted; a candidate must fail closed", f.Record)
			}
		})
	}
}

// The digest-only negative fixture proves a mutable tag alone is never a valid
// immutable identity; only a sha256 digest pins an image.
func TestCandidateDigestMustBePinnedNotTagOnly(t *testing.T) {
	resolved := candidateResolvedSchema(t)
	tagOnly := `{"contract_version":"baseharbor.candidate/v1","app":{"id":"00000000-0000-0000-0000-000000000002"},"target":{"name":"lab"},"consent":{"acknowledged":true,"by":"automation"},"image":{"kind":"image","reference":"example/app:latest"}}`
	var value any
	if err := json.Unmarshal([]byte(tagOnly), &value); err != nil {
		t.Fatal(err)
	}
	if err := resolved.Validate(value); err == nil {
		t.Fatal("mutable tag-only submission accepted; a digest is required for image identity")
	}
}

// A bare secret reference in a submission is never part of candidate identity:
// the schema disallows unknown keys, so a stray secretRef must fail.
func TestCandidateRejectsBareSecretReference(t *testing.T) {
	resolved := candidateResolvedSchema(t)
	withSecret := `{"contract_version":"baseharbor.candidate/v1","app":{"id":"00000000-0000-0000-0000-000000000005"},"target":{"name":"lab"},"consent":{"acknowledged":true,"by":"operator"},"source":{"kind":"source","repository":"example/app","revision":"abc123","secretRef":"secrets/db-password"}}`
	var value any
	if err := json.Unmarshal([]byte(withSecret), &value); err != nil {
		t.Fatal(err)
	}
	if err := resolved.Validate(value); err == nil {
		t.Fatal("bare secret reference accepted in submission; secrets must not enter candidate identity")
	}
}
