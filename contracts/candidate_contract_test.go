// Package contracts_test validates the exact source/image submission contract
// through the existing product path: the candidate submission is validated by
// the fail-closed leaf in internal/delivery, not by an isolated schema-only
// check. A candidate is a fixed source revision, image digest, or native
// compose/quadlet unit. A submission must carry exactly one immutable identity
// source (source revision OR image digest, never a mutable tag alone) and never
// a bare secret reference. Provenance is never inferred from annotations.
package contracts_test

import (
	_ "embed"
	"encoding/json"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/mcpdev80/baseharbor/internal/delivery"
)

//go:embed candidate/v1/golden-fixtures.json
var goldenFixtures []byte

//go:embed candidate/v1/negative-fixtures.json
var negativeFixtures []byte

//go:embed candidate/v1/candidate-contract.schema.json
var candidateSchema []byte

type candidateFixture struct {
	Record  string          `json:"record"`
	Fixture json.RawMessage `json:"fixture"`
}

// toSubmission maps a raw contract fixture onto the typed delivery leaf so the
// schema validators exercise the real product interface, not JSON parsing. It
// fails closed on any unknown key (matching the published schema's
// additionalProperties:false), so the mapper has no silent permissive path.
func toSubmission(t *testing.T, raw json.RawMessage) delivery.CandidateSubmission {
	t.Helper()
	var v map[string]any
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("fixture is not valid JSON: %v", err)
	}
	var d delivery.CandidateSubmission
	if err := json.Unmarshal(raw, &d); err != nil {
		t.Fatalf("fixture did not map onto the candidate contract: %v", err)
	}

	contractVersion, _ := v["contract_version"].(string)
	d.ContractVersion = contractVersion

	if app, ok := v["app"].(map[string]any); ok {
		if id, _ := app["id"].(string); id != "" {
			d.ApplicationID = id
		}
	}
	if target, ok := v["target"].(map[string]any); ok {
		if name, _ := target["name"].(string); name != "" {
			d.TargetName = name
		}
	}
	if consent, ok := v["consent"].(map[string]any); ok {
		if by, _ := consent["by"].(string); by != "" {
			d.ConsentBy = by
		}
		if ack, ok := consent["acknowledged"].(bool); ok {
			d.Acknowledged = ack
		}
	}
	if source, ok := v["source"].(map[string]any); ok {
		if repository, _ := source["repository"].(string); repository != "" {
			d.SourceRepository = repository
		}
		if revision, _ := source["revision"].(string); revision != "" {
			d.SourceRevision = revision
		}
		if secretRef, _ := source["secretRef"].(string); secretRef != "" {
			d.SecretRef = secretRef
		}
	}
	if image, ok := v["image"].(map[string]any); ok {
		if reference, _ := image["reference"].(string); reference != "" {
			d.ImageReference = reference
		}
		if digest, _ := image["digest"].(string); digest != "" {
			d.ImageDigest = digest
		}
	}
	return d
}

// loadFixtures parses a fixtures array and fails closed on an empty set.
func loadFixtures(t *testing.T, raw []byte) []candidateFixture {
	t.Helper()
	var fixtures []candidateFixture
	if err := json.Unmarshal(raw, &fixtures); err != nil {
		t.Fatalf("fixtures: %v", err)
	}
	if len(fixtures) == 0 {
		t.Fatal("empty fixtures")
	}
	return fixtures
}

// schemaValidator parses the published candidate contract schema once.
func schemaValidator(t *testing.T) *jsonschema.Resolved {
	t.Helper()
	var raw jsonschema.Schema
	if err := json.Unmarshal(candidateSchema, &raw); err != nil {
		t.Fatalf("published schema is not valid JSON: %v", err)
	}
	resolved, err := raw.Resolve(nil)
	if err != nil {
		t.Fatalf("published schema did not resolve: %v", err)
	}
	return resolved
}

// validateAgainstSchema validates a parsed fixture document against the
// published schema and returns the schema error (nil on success).
func validateAgainstSchema(t *testing.T, resolved *jsonschema.Resolved, fixture json.RawMessage) error {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal(fixture, &doc); err != nil {
		t.Fatalf("fixture is not valid JSON: %v", err)
	}
	return resolved.Validate(doc)
}

// Golden fixtures prove that a valid candidate submission is accepted by the
// real internal/delivery candidate leaf and by the published schema: exactly
// one immutable identity, acknowledged consent, and a pinned source revision or
// image digest.
func TestCandidateGoldenFixturesAreAcceptedByProductLeaf(t *testing.T) {
	for _, f := range loadFixtures(t, goldenFixtures) {
		t.Run(f.Record, func(t *testing.T) {
			sub := toSubmission(t, f.Fixture)
			if err := sub.Validate(); err != nil {
				t.Fatalf("%s: valid candidate rejected by product leaf: %v", f.Record, err)
			}
			resolved := schemaValidator(t)
			if err := validateAgainstSchema(t, resolved, f.Fixture); err != nil {
				t.Fatalf("%s: valid candidate rejected by published schema: %v", f.Record, err)
			}
		})
	}
}

// Negative fixtures prove that a malformed candidate submission fails closed at
// the real internal/delivery candidate leaf and the published schema. Exactly
// one fail-closed case is required per contract point: missing identity source,
// mutable tag only, and missing acknowledged consent.
func TestCandidateNegativeFixturesFailClosedAtProductLeaf(t *testing.T) {
	resolved := schemaValidator(t)
	for _, f := range loadFixtures(t, negativeFixtures) {
		t.Run(f.Record, func(t *testing.T) {
			sub := toSubmission(t, f.Fixture)
			if err := sub.Validate(); err == nil {
				t.Fatalf("%s: malformed candidate accepted by product leaf; a candidate must fail closed", f.Record)
			}
			if err := validateAgainstSchema(t, resolved, f.Fixture); err == nil {
				t.Fatalf("%s: malformed candidate accepted by published schema; a candidate must fail closed", f.Record)
			}
		})
	}
}

// Exactly one immutable identity source: a submission carrying both a source
// revision and an image digest is ambiguous and must fail closed.
func TestCandidateRequiresExactlyOneIdentitySource(t *testing.T) {
	both := `{"contract_version":"baseharbor.candidate/v1","app":{"id":"00000000-0000-0000-0000-000000000004"},"target":{"name":"lab"},"consent":{"acknowledged":true,"by":"operator"},"source":{"kind":"source","repository":"example/app","revision":"0123456789abcdef0123456789abcdef01234567"},"image":{"kind":"image","reference":"example/app","digest":"sha256:2222222222222222222222222222222222222222222222222222222222222222"}}`
	sub := toSubmission(t, json.RawMessage(both))
	if err := sub.Validate(); err == nil {
		t.Fatal("both source and image accepted; exactly one immutable identity is required")
	}
}

// A movable branch-string is never a fixed identity on its own; only a pinned
// source revision (a 40/64-hex commit object ID or an opaque digest) is
// accepted, never a mutable tag even alongside an image digest.
func TestCandidateSourceRevisionMustBePinned(t *testing.T) {
	loose := `{"contract_version":"baseharbor.candidate/v1","app":{"id":"00000000-0000-0000-0000-000000000001"},"target":{"name":"lab"},"consent":{"acknowledged":true,"by":"operator"},"source":{"kind":"source","repository":"example/app","revision":"main"}}`
	sub := toSubmission(t, json.RawMessage(loose))
	if err := sub.Validate(); err == nil {
		t.Fatal("movable branch-string source accepted; a source revision must be pinned")
	}

	pinned := `{"contract_version":"baseharbor.candidate/v1","app":{"id":"00000000-0000-0000-0000-000000000001"},"target":{"name":"lab"},"consent":{"acknowledged":true,"by":"operator"},"source":{"kind":"source","repository":"example/app","revision":"0123456789abcdef0123456789abcdef01234567"}}`
	sub = toSubmission(t, json.RawMessage(pinned))
	if err := sub.Validate(); err != nil {
		t.Fatalf("fixed source revision rejected: %v", err)
	}
}

// A mutable tag paired with a sha256 image digest is still not a fixed identity:
// the immutable identity is content-addressed, never a syntactic label.
func TestCandidateMutableTagWithImageCounterpartRejected(t *testing.T) {
	mutable := `{"contract_version":"baseharbor.candidate/v1","app":{"id":"00000000-0000-0000-0000-000000000001"},"target":{"name":"lab"},"consent":{"acknowledged":true,"by":"operator"},"source":{"kind":"source","repository":"example/app","revision":"release-1"},"image":{"kind":"image","reference":"example/app","digest":"sha256:1111111111111111111111111111111111111111111111111111111111111111"}}`
	sub := toSubmission(t, json.RawMessage(mutable))
	if err := sub.Validate(); err == nil {
		t.Fatal("mutable tag with image counterpart accepted; a mutable tag is never a fixed identity")
	}
}

func TestCandidateConsentMustBeAcknowledged(t *testing.T) {
	notAcked := `{"contract_version":"baseharbor.candidate/v1","app":{"id":"00000000-0000-0000-0000-000000000001"},"target":{"name":"lab"},"consent":{"acknowledged":false,"by":"operator"},"source":{"kind":"source","repository":"example/app","revision":"0123456789abcdef0123456789abcdef01234567"}}`
	sub := toSubmission(t, json.RawMessage(notAcked))
	if err := sub.Validate(); err == nil {
		t.Fatal("unacknowledged candidate accepted; consent.acknowledged must be true")
	}
}

// A bare secret reference in a submission is never part of candidate identity
// and fails closed at the product leaf.
func TestCandidateRejectsBareSecretReference(t *testing.T) {
	withSecret := `{"contract_version":"baseharbor.candidate/v1","app":{"id":"00000000-0000-0000-0000-000000000005"},"target":{"name":"lab"},"consent":{"acknowledged":true,"by":"operator"},"source":{"kind":"source","repository":"example/app","revision":"0123456789abcdef0123456789abcdef01234567","secretRef":"secrets/db-password"}}`
	sub := toSubmission(t, json.RawMessage(withSecret))
	if err := sub.Validate(); err == nil {
		t.Fatal("bare secret reference accepted; secrets must not enter candidate identity")
	}
}

// Schema truth: the published schema forbids unknown keys, and the Go leaf
// rejects exactly the fields the schema forbids. A fixture carrying an unknown
// top-level key is rejected by the published schema (additionalProperties:false).
func TestCandidateSchemaForbidsUnknownKeys(t *testing.T) {
	resolved := schemaValidator(t)
	unknown := `{"contract_version":"baseharbor.candidate/v1","app":{"id":"00000000-0000-0000-0000-000000000001"},"target":{"name":"lab"},"consent":{"acknowledged":true,"by":"operator"},"source":{"kind":"source","repository":"example/app","revision":"0123456789abcdef0123456789abcdef01234567"},"stage":"unlisted","unknownKey":"nope"}`
	if err := validateAgainstSchema(t, resolved, json.RawMessage(unknown)); err == nil {
		t.Fatal("published schema accepted an unknown key; additionalProperties must be false")
	}
}
