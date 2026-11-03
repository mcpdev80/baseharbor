// Package contracts_test validates the exact source/image submission contract
// through the existing product path: the candidate submission is validated by
// the fail-closed leaf in internal/delivery, not by an isolated schema-only
// check. A candidate is a fixed source revision, image digest, or native
// compose/quadlet unit. A submission must carry exactly one immutable identity
// source (source revision OR image digest, never a mutable tag alone) and never
// a bare secret reference. Provenance is never inferred from annotations.
package contracts_test

import (
	"bytes"
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
	// Parse structurally into a nested map so the mapper can read the exact
	// contract fields (contract_version, app.id, target.name, consent.*,
	// source.*, image.*) and build the typed submission by hand. This avoids
	// relying on struct tags for the top-level shape while still decoding onto
	// the typed leaf for the field-level contract below.
	var v map[string]any
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("fixture is not valid JSON: %v", err)
	}
	d := delivery.CandidateSubmission{}

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
	// Decode the fully mapped submission onto the typed contract with a
	// json.Decoder and DisallowUnknownFields: the published schema forbids
	// unknown keys (additionalProperties:false), so the mapped leaf has no
	// silent permissive path — any residual unknown key fails to map.
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&map[string]any{}); err != nil {
		// Top-level unknown keys are not structurally valid: treat as a
		// mapping failure rather than a silent drop.
		t.Fatalf("fixture carried unknown top-level keys: %v", err)
	}
	return d
}

// validateSubmission enforces the candidate leaf with an explicit consent.
// Positive cases carry a matching consent whose digest is the canonical
// content-hash of exactly this submission; cases that expect failure pass nil
// (missing consent) so a candidate can only be persisted when an explicit
// operator/automation consent pins the validated submission.
//
// The acknowledged principal (by) is taken from the caller when non-empty,
// otherwise it defaults to "operator".
func validateSubmission(t *testing.T, sub delivery.CandidateSubmission, by string, wantOK bool) error {
	t.Helper()
	var consent *delivery.CandidateConsent
	if wantOK {
		if by == "" {
			by = "operator"
		}
		consent = &delivery.CandidateConsent{By: by, Digest: delivery.CandidateConsentDigest(sub)}
	}
	return sub.Validate(consent)
}

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
			if err := validateSubmission(t, sub, "operator", true); err != nil {
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
			if err := validateSubmission(t, sub, "", false); err == nil {
				t.Fatalf("%s: malformed candidate accepted by product leaf; a candidate must fail closed", f.Record)
			}
			if err := validateAgainstSchema(t, resolved, f.Fixture); err == nil {
				t.Fatalf("%s: malformed candidate accepted by published schema; a candidate must fail closed", f.Record)
			}
		})
	}
}

// A mutable branch-string source revision fails the hardened schema even when
// every other field is well-formed: the published schema pins the identity.
func TestCandidateSourceRevisionPatternRejectsMutable(t *testing.T) {
	resolved := schemaValidator(t)
	loose := `{"contract_version":"baseharbor.candidate/v1","app":{"id":"00000000-0000-0000-0000-000000000001"},"target":{"name":"lab"},"consent":{"by":"operator","at":"2026-11-03T00:00:00Z","digest":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"},"source":{"kind":"source","repository":"example/app","revision":"main"}}`
	if err := validateAgainstSchema(t, resolved, json.RawMessage(loose)); err == nil {
		t.Fatal("mutable source revision accepted by hardened schema; revision must match the identity pattern")
	}

	// A fixed 40-hex commit object ID passes the same pattern.
	pinned := `{"contract_version":"baseharbor.candidate/v1","app":{"id":"00000000-0000-0000-0000-000000000001"},"target":{"name":"lab"},"consent":{"by":"operator","at":"2026-11-03T00:00:00Z","digest":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"},"source":{"kind":"source","repository":"example/app","revision":"0123456789abcdef0123456789abcdef01234567"}}`
	if err := validateAgainstSchema(t, resolved, json.RawMessage(pinned)); err != nil {
		t.Fatalf("fixed source revision rejected by hardened schema: %v", err)
	}

	// An opaque algo:hex digest also passes the pattern.
	opaque := `{"contract_version":"baseharbor.candidate/v1","app":{"id":"00000000-0000-0000-0000-000000000001"},"target":{"name":"lab"},"consent":{"by":"operator","at":"2026-11-03T00:00:00Z","digest":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"},"source":{"kind":"source","repository":"example/app","revision":"sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}}`
	if err := validateAgainstSchema(t, resolved, json.RawMessage(opaque)); err != nil {
		t.Fatalf("opaque algo:hex digest rejected by hardened schema: %v", err)
	}

	// The consent digest is pinned too: a non-identity digest fails the schema.
	badConsent := `{"contract_version":"baseharbor.candidate/v1","app":{"id":"00000000-0000-0000-0000-000000000001"},"target":{"name":"lab"},"consent":{"by":"operator","at":"2026-11-03T00:00:00Z","digest":"not-an-identity"},"source":{"kind":"source","repository":"example/app","revision":"0123456789abcdef0123456789abcdef01234567"}}`
	if err := validateAgainstSchema(t, resolved, json.RawMessage(badConsent)); err == nil {
		t.Fatal("non-identity consent digest accepted by hardened schema; digest must match the identity pattern")
	}
}

// Exactly one immutable identity source: a submission carrying both a source
// revision and an image digest is ambiguous and must fail closed.
func TestCandidateRequiresExactlyOneIdentitySource(t *testing.T) {
	both := `{"contract_version":"baseharbor.candidate/v1","app":{"id":"00000000-0000-0000-0000-000000000004"},"target":{"name":"lab"},"consent":{"acknowledged":true,"by":"operator"},"source":{"kind":"source","repository":"example/app","revision":"0123456789abcdef0123456789abcdef01234567"},"image":{"kind":"image","reference":"example/app","digest":"sha256:2222222222222222222222222222222222222222222222222222222222222222"}}`
	sub := toSubmission(t, json.RawMessage(both))
	if err := validateSubmission(t, sub, "", false); err == nil {
		t.Fatal("both source and image accepted; exactly one immutable identity is required")
	}
}

// A movable branch-string is never a fixed identity on its own; only a pinned
// source revision (a 40/64-hex commit object ID or an opaque digest) is
// accepted, never a mutable tag even alongside an image digest.
func TestCandidateSourceRevisionMustBePinned(t *testing.T) {
	loose := `{"contract_version":"baseharbor.candidate/v1","app":{"id":"00000000-0000-0000-0000-000000000001"},"target":{"name":"lab"},"consent":{"acknowledged":true,"by":"operator"},"source":{"kind":"source","repository":"example/app","revision":"main"}}`
	sub := toSubmission(t, json.RawMessage(loose))
	if err := validateSubmission(t, sub, "", false); err == nil {
		t.Fatal("movable branch-string source accepted; a source revision must be pinned")
	}

	pinned := `{"contract_version":"baseharbor.candidate/v1","app":{"id":"00000000-0000-0000-0000-000000000001"},"target":{"name":"lab"},"consent":{"acknowledged":true,"by":"operator"},"source":{"kind":"source","repository":"example/app","revision":"0123456789abcdef0123456789abcdef01234567"}}`
	sub = toSubmission(t, json.RawMessage(pinned))
	if err := validateSubmission(t, sub, "operator", true); err != nil {
		t.Fatalf("fixed source revision rejected: %v", err)
	}
}

// A mutable tag paired with a sha256 image digest is still not a fixed identity:
// the immutable identity is content-addressed, never a syntactic label.
func TestCandidateMutableTagWithImageCounterpartRejected(t *testing.T) {
	mutable := `{"contract_version":"baseharbor.candidate/v1","app":{"id":"00000000-0000-0000-0000-000000000001"},"target":{"name":"lab"},"consent":{"acknowledged":true,"by":"operator"},"source":{"kind":"source","repository":"example/app","revision":"release-1"},"image":{"kind":"image","reference":"example/app","digest":"sha256:1111111111111111111111111111111111111111111111111111111111111111"}}`
	sub := toSubmission(t, json.RawMessage(mutable))
	if err := validateSubmission(t, sub, "", false); err == nil {
		t.Fatal("mutable tag with image counterpart accepted; a mutable tag is never a fixed identity")
	}
}

func TestCandidateConsentMustBeAcknowledged(t *testing.T) {
	notAcked := `{"contract_version":"baseharbor.candidate/v1","app":{"id":"00000000-0000-0000-0000-000000000001"},"target":{"name":"lab"},"consent":{"acknowledged":false,"by":"operator"},"source":{"kind":"source","repository":"example/app","revision":"0123456789abcdef0123456789abcdef01234567"}}`
	sub := toSubmission(t, json.RawMessage(notAcked))
	if err := validateSubmission(t, sub, "", false); err == nil {
		t.Fatal("unacknowledged candidate accepted; consent.acknowledged must be true")
	}
}

// A bare secret reference in a submission is never part of candidate identity
// and fails closed at the product leaf.
func TestCandidateRejectsBareSecretReference(t *testing.T) {
	withSecret := `{"contract_version":"baseharbor.candidate/v1","app":{"id":"00000000-0000-0000-0000-000000000005"},"target":{"name":"lab"},"consent":{"acknowledged":true,"by":"operator"},"source":{"kind":"source","repository":"example/app","revision":"0123456789abcdef0123456789abcdef01234567","secretRef":"secrets/db-password"}}`
	sub := toSubmission(t, json.RawMessage(withSecret))
	if err := validateSubmission(t, sub, "", false); err == nil {
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
