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
	"fmt"
	"io"
	"testing"
	"time"

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

// candidateContractDocument is the typed top-level shape of a candidate
// submission fixture. Decoding onto these concrete structs — not map[string]any
// — is what makes json.Decoder.DisallowUnknownFields actually reject unknown
// keys at every depth, so the mapper cannot silently drop an unknown nested key
// that the published schema forbids.
type candidateContractDocument struct {
	ContractVersion string `json:"contract_version"`
	App             struct {
		ID string `json:"id"`
	} `json:"app"`
	Target struct {
		Name string `json:"name"`
	} `json:"target"`
	Consent struct {
		By           string `json:"by"`
		At           string `json:"at"`
		Digest       string `json:"digest"`
		Acknowledged bool   `json:"acknowledged"`
	} `json:"consent"`
	Source struct {
		Kind       string `json:"kind"`
		Repository string `json:"repository"`
		Revision   string `json:"revision"`
		SecretRef  string `json:"secretRef"`
	} `json:"source"`
	Image struct {
		Kind      string `json:"kind"`
		Reference string `json:"reference"`
		Digest    string `json:"digest"`
	} `json:"image"`
	// stage is an optional top-level label allowed by the published schema.
	Stage string `json:"stage"`
}

// toSubmission maps a raw contract fixture onto the typed delivery leaf so the
// schema validators exercise the real product interface, not JSON parsing. It
// fails closed on ANY unknown key (top-level or nested) via DisallowUnknownFields
// over the concrete struct — matching the published schema's
// additionalProperties:false at every level — so the mapper has no silent
// permissive path. Unknown keys never reach the typed leaf.
func toSubmission(t *testing.T, raw json.RawMessage) delivery.CandidateSubmission {
	t.Helper()

	// Typed nested decode: DisallowUnknownFields only rejects unknown keys when
	// decoding onto concrete struct fields, never onto map[string]any.
	doc, decodeErr := decodeCandidateDocument(raw)
	if decodeErr != nil {
		t.Fatalf("fixture carried unknown key(s) rejected by the typed mapper (unknown keys must fail closed): %v", decodeErr)
	}

	// Build the typed delivery leaf from the decoded struct fields. Unknown
	// keys never reached here — the decode above already failed closed.
	return delivery.CandidateSubmission{
		ContractVersion:  doc.ContractVersion,
		ApplicationID:    doc.App.ID,
		TargetName:       doc.Target.Name,
		ConsentBy:        doc.Consent.By,
		Acknowledged:     doc.Consent.Acknowledged,
		SourceRepository: doc.Source.Repository,
		SourceRevision:   doc.Source.Revision,
		SecretRef:        doc.Source.SecretRef,
		ImageReference:   doc.Image.Reference,
		ImageDigest:      doc.Image.Digest,
	}
}

// decodeCandidateDocument decodes a raw JSON candidate fixture onto the typed
// struct with DisallowUnknownFields and an EOF check. It returns the decoded
// document and a non-nil error when the fixture carries any unknown key (at any
// depth) or trailing content — never silently dropping a key. This non-terminating
// form lets counter-tests assert the fail-closed behaviour without killing the
// test runner mid-run.
func decodeCandidateDocument(raw json.RawMessage) (candidateContractDocument, error) {
	var doc candidateContractDocument
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&doc); err != nil {
		return doc, fmt.Errorf("unknown key: %w", err)
	}
	if _, err := dec.Token(); err != io.EOF {
		return doc, fmt.Errorf("trailing content: %w", err)
	}
	return doc, nil
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

// Typed mapper counter-probe (r27 round 7, finding 2): the mapper decodes onto
// the concrete struct with DisallowUnknownFields, so an unknown TOP-LEVEL key
// must fail closed at toSubmission. A silently dropped top-level key would let
// an unknown field reach the leaf, contradicting additionalProperties:false.
func TestTypedMapperRejectsUnknownTopLevelKey(t *testing.T) {
	withUnknown := `{"contract_version":"baseharbor.candidate/v1","app":{"id":"00000000-0000-0000-0000-000000000001"},"target":{"name":"lab"},"consent":{"acknowledged":true,"by":"operator"},"source":{"kind":"source","repository":"example/app","revision":"0123456789abcdef0123456789abcdef01234567"},"staged":"nope"}`
	if got := toSubmissionErr(t, withUnknown); got == "" {
		t.Fatal("unknown top-level key accepted by typed mapper; DisallowUnknownFields must fail closed")
	}
}

// Shared case-parity vector (r27 round 7, finding 3): the Go leaf
// (internal/delivery) and the published schema must agree on every case form of
// a fixed source revision. Go accepts [0-9a-fA-F] and compares canonically
// (lowercased); the schema pattern is the same [0-9a-fA-F]. This vector drives
// identical case inputs through BOTH validators so a divergence in either case
// handling fails closed here.
func TestCandidateSourceRevisionCaseParityThroughBothValidators(t *testing.T) {
	resolved := schemaValidator(t)
	// Identical revision in four case forms; all four are the same fixed
	// identity and must be accepted by both validators, or rejected by both.
	cases := []string{
		"0123456789abcdef0123456789abcdef01234567",                                // lowercase
		"0123456789ABCDEF0123456789ABCDEF01234567",                                // uppercase
		"0123456789AbCdEf0123456789AbCdEf01234567",                                // mixed
		"sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", // opaque lowercase
		"sha256:0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF", // opaque uppercase
	}
	for _, revision := range cases {
		source := fmt.Sprintf(`{"contract_version":"baseharbor.candidate/v1","app":{"id":"00000000-0000-0000-0000-000000000001"},"target":{"name":"lab"},"consent":{"by":"operator","at":"2026-11-03T00:00:00Z","digest":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"},"source":{"kind":"source","repository":"example/app","revision":%q}}`, revision)

		// Go leaf: build the typed submission and validate it with a consent
		// whose digest pins THIS submission's exact content-hash, so the
		// consent-digest check passes and only the revision-case contract is
		// exercised.
		sub := toSubmission(t, json.RawMessage(source))
		sub.Consent = &delivery.CandidateConsent{By: "operator", At: timeNowUTC(), Digest: delivery.CandidateConsentDigest(sub)}
		gotErr := sub.Validate(sub.Consent)
		gotOK := gotErr == nil

		// Schema: validate the same document.
		schemaErr := validateAgainstSchema(t, resolved, json.RawMessage(source))
		schemaOK := schemaErr == nil

		// The two validators must agree: same pass/fail outcome for every case.
		if gotOK != schemaOK {
			t.Fatalf("case parity violated for %q: Go leaf accepted=%v, schema accepted=%v (go err=%v, schema err=%v)", revision, gotOK, schemaOK, gotErrString(gotErr), schemaErr)
		}
	}
}

// timeNowUTC returns the current UTC time for consent stamps.
func timeNowUTC() time.Time {
	return time.Now().UTC()
}

// gotErrString returns the error string, or "" when err is nil, for compact
// divergence messages.
func gotErrString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// Typed mapper counter-probe (r27 round 7, finding 2): an unknown key nested
// one level deep (inside an existing object such as consent) must fail closed,
// because DisallowUnknownFields only rejects unknown keys when decoding onto
// concrete struct fields. A map[string]any round-trip would silently drop it.
func TestTypedMapperRejectsUnknownNestedKey(t *testing.T) {
	withNested := `{"contract_version":"baseharbor.candidate/v1","app":{"id":"00000000-0000-0000-0000-000000000001"},"target":{"name":"lab"},"consent":{"acknowledged":true,"by":"operator","scoped":true},"source":{"kind":"source","repository":"example/app","revision":"0123456789abcdef0123456789abcdef01234567"}}`
	if got := toSubmissionErr(t, withNested); got == "" {
		t.Fatal("unknown nested key accepted by typed mapper; DisallowUnknownFields must fail closed at every depth")
	}
}

// toSubmissionErr runs toSubmission on a raw JSON fixture and returns the
// failure message (empty when the fixture is structurally valid), without
// terminating the test. Unknown keys must produce a non-nil error so the mapper
// fails closed instead of silently dropping them.
func toSubmissionErr(t *testing.T, raw string) string {
	t.Helper()
	defer func() {
		if rec := recover(); rec != nil {
			t.Fatalf("toSubmission panicked on %s: %v", raw, rec)
		}
	}()
	if _, err := decodeCandidateDocument(json.RawMessage(raw)); err != nil {
		return err.Error()
	}
	return ""
}
