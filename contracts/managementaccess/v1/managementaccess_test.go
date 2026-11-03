package managementaccess_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/url"
	"testing"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/mcpdev80/baseharbor/contracts/managementaccess/v1"
	"github.com/mcpdev80/baseharbor/internal/targetenrollment"
	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
)

// ---- Finding 1: kind-specific fail-closed enforcement ----

func TestValidateKindManagementProfileRequiresSafeTrustAndStorage(t *testing.T) {
	p := profileFor(t)
	if err := p.ValidateKindEnforcement(); err != nil {
		t.Fatalf("baseline management-profile should validate: %v", err)
	}

	// Missing trust fingerprint.
	p.Trust.SHA256 = ""
	if err := p.ValidateKindEnforcement(); err == nil {
		t.Fatal("management-profile with empty trust fingerprint must fail closed")
	}

	// Plaintext durable storage must fail closed.
	p = profileFor(t)
	p.Storage = managementaccess.Storage{Class: managementaccess.ProtectedDurable, Durable: true, Encrypted: false, DisasterRecovery: false}
	if err := p.ValidateKindEnforcement(); err == nil {
		t.Fatal("management-profile with plaintext durable storage must fail closed")
	}
}

func TestValidateKindDeviceEnrollmentMustBeSensitiveAndOneUse(t *testing.T) {
	p := profileFor(t)
	p.Kind = managementaccess.DeviceEnrollment
	p.Storage = managementaccess.Storage{Class: managementaccess.SessionOnly, Durable: false, Encrypted: true, DisasterRecovery: false}
	if err := p.ValidateKindEnforcement(); err != nil {
		t.Fatalf("sensitive session-only device-enrollment should validate: %v", err)
	}

	// Plaintext durable enrollment fails closed.
	p = profileFor(t)
	p.Kind = managementaccess.DeviceEnrollment
	p.Storage = managementaccess.Storage{Class: managementaccess.ProtectedDurable, Durable: true, Encrypted: false, DisasterRecovery: false}
	if err := p.ValidateKindEnforcement(); err == nil {
		t.Fatal("plaintext durable device-enrollment must fail closed")
	}
}

func TestValidateKindCredentialExportRequiresEncryption(t *testing.T) {
	p := profileFor(t)
	p.Kind = managementaccess.CredentialExport
	p.Export = &managementaccess.EncryptionDescriptor{Algorithm: "aes-256-gcm", Encrypted: true, Nonce: "E1f2g3h4i5j6k7l8m9n0o1p2q3r4s5t6u7v8w9x0y_z"}
	if err := p.ValidateKindEnforcement(); err != nil {
		t.Fatalf("encrypted credential-export should validate: %v", err)
	}

	// Missing export descriptor fails closed.
	p = profileFor(t)
	p.Kind = managementaccess.CredentialExport
	if err := p.ValidateKindEnforcement(); err == nil {
		t.Fatal("credential-export without export descriptor must fail closed")
	}

	// Unencrypted export fails closed.
	p = profileFor(t)
	p.Kind = managementaccess.CredentialExport
	p.Export = &managementaccess.EncryptionDescriptor{Algorithm: "aes-256-gcm", Encrypted: false, Nonce: "E1f2g3h4i5j6k7l8m9n0o1p2q3r4s5t6u7v8w9x0y_z"}
	if err := p.ValidateKindEnforcement(); err == nil {
		t.Fatal("unencrypted credential-export must fail closed")
	}

	// Missing nonce fails closed.
	p = profileFor(t)
	p.Kind = managementaccess.CredentialExport
	p.Export = &managementaccess.EncryptionDescriptor{Algorithm: "aes-256-gcm", Encrypted: true, Nonce: ""}
	if err := p.ValidateKindEnforcement(); err == nil {
		t.Fatal("credential-export with empty nonce must fail closed")
	}
}

func TestImportCapabilityFailClosedForEachGuarantee(t *testing.T) {
	for _, flag := range []struct {
		name string
		set  func(*managementaccess.ImportCapability)
	}{
		{"rights_granting", func(c *managementaccess.ImportCapability) { c.RightsGranting = true }},
		{"plugin_execution", func(c *managementaccess.ImportCapability) { c.PluginExecution = true }},
		{"credential_redirect", func(c *managementaccess.ImportCapability) { c.CredentialRedirect = true }},
	} {
		c := managementaccess.ImportCapability{}
		flag.set(&c)
		if !c.FailClosedImport() {
			t.Fatalf("%s=true must fail closed import", flag.name)
		}
	}
}

// ---- Finding 2: semantic trust / recipient / validity validation ----

func TestValidateTrustMatchesCertificate(t *testing.T) {
	p := profileFor(t)
	if err := p.ValidateTrust(); err != nil {
		t.Fatalf("matching trust must validate: %v", err)
	}

	// Tampered fingerprint fails closed.
	p.Trust.SHA256 = "0000000000000000000000000000000000000000000000000000000000000000"
	if err := p.ValidateTrust(); err == nil {
		t.Fatal("trust fingerprint that does not match the certificate must fail closed")
	}
}

func TestValidityWindowElapsedFutureMalformed(t *testing.T) {
	now := time.Now().UTC()
	future := now.Add(48 * time.Hour)
	past := now.Add(-48 * time.Hour)

	// Elapsed window fails closed.
	p := profileFor(t)
	p.NotBefore = past.Add(-time.Hour)
	p.NotAfter = past.Add(-time.Minute)
	if err := p.ValidityWindow(now); err == nil {
		t.Fatal("elapsed validity window must fail closed")
	}

	// Future (not yet valid) window fails closed.
	p = profileFor(t)
	p.NotBefore = future.Add(time.Hour)
	p.NotAfter = future.Add(2 * time.Hour)
	if err := p.ValidityWindow(now); err == nil {
		t.Fatal("future validity window must fail closed")
	}

	// not_after before not_before fails closed.
	p = profileFor(t)
	p.NotBefore = future.Add(2 * time.Hour)
	p.NotAfter = future.Add(time.Hour)
	if err := p.ValidityWindow(future); err == nil {
		t.Fatal("inverted validity window must fail closed")
	}
}

func TestValidateRecipientBindForeignPin(t *testing.T) {
	scope := authorizedScope()
	if err := managementaccess.ValidateRecipientBind(scope.Identity(), scope); err != nil {
		t.Fatalf("matching recipient must validate: %v", err)
	}
	// Foreign pin: recipient not bound to the authorized scope.
	foreign := "spiffe://baseharbor/platform/core/22222222-2222-4222-8222-222222222222/attacker"
	if err := managementaccess.ValidateRecipientBind(foreign, scope); err == nil {
		t.Fatal("foreign recipient must fail closed (ErrForeignPin)")
	}
}

// ---- Finding 3 & 5: callable path + real enrollment through authoritative interface ----

// fakeOneUseStore implements targetenrollment.Store as a single-use memory store
// that returns a valid grant on first consume and denies every later use.
type fakeOneUseStore struct {
	consumed bool
	grant    targetenrollment.Grant
}

func (s *fakeOneUseStore) Create(_ context.Context, grant targetenrollment.Grant) error {
	s.grant = grant
	return nil
}
func (s *fakeOneUseStore) Consume(_ context.Context, scope targetenrollment.Scope, _, _, csr string, now time.Time) (targetenrollment.Grant, error) {
	if s.consumed {
		return targetenrollment.Grant{}, targetenrollment.ErrDenied
	}
	s.consumed = true
	return targetenrollment.Grant{Scope: scope, CertificateTTL: time.Hour}, nil
}
func (s *fakeOneUseStore) RecordIssued(context.Context, targetenrollment.Scope, string, string, time.Time) error {
	return nil
}

func enrollScopeRequest(t *testing.T) (targetenrollment.Request, managementaccess.Enroller) {
	t.Helper()
	store := &fakeOneUseStore{}
	issuer := serviceissuer.New(t)
	authority, err := targetenrollment.New(store, issuer)
	if err != nil {
		t.Fatal(err)
	}
	// Create a one-use bootstrap grant, then build an enrollment request.
	bootstrap, err := authority.Create(context.Background(), authorizedScope(), time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	uri, _ := url.Parse(authorizedScope().Identity())
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{URIs: []*url.URL{uri}}, key)
	if err != nil {
		t.Fatal(err)
	}
	request := targetenrollment.Request{
		Scope:  authorizedScope(),
		Token:  bootstrap.Token,
		Nonce:  bootstrap.Nonce,
		CSRPEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: raw}),
	}
	return request, authority
}

func TestEnrollDeviceOneUseThroughAuthority(t *testing.T) {
	request, authority := enrollScopeRequest(t)
	result, err := managementaccess.EnrollDevice(context.Background(), authority, request)
	if err != nil {
		t.Fatalf("one-use enrollment must succeed: %v", err)
	}
	if len(result.Certificate) == 0 {
		t.Fatal("expected a certificate from enrollment")
	}
	if len(result.Trust.PEM) == 0 {
		t.Fatal("expected a trust bundle from enrollment")
	}

	// A second use of the same one-use request must fail closed.
	_, err = managementaccess.EnrollDevice(context.Background(), authority, request)
	if err == nil {
		t.Fatal("second one-use enrollment must fail closed (ErrDenied)")
	}
}

func TestEnrollDeviceNilAuthorityFailsClosed(t *testing.T) {
	request, _ := enrollScopeRequest(t)
	_, err := managementaccess.EnrollDevice(context.Background(), nil, request)
	if err == nil {
		t.Fatal("nil authority must fail closed")
	}
}

// ---- Finding 3: wire contract JSON tags match schema (round trip) ----

// wireRoundTripAgainstSchema marshals a profile, unmarshals it back, re-marshals
// and validates BOTH directions against the packaged schema with real semantic
// validation (not just key presence).
func wireRoundTripAgainstSchema(t *testing.T, resolved *jsonschema.Resolved, p managementaccess.ManagementAccessProfile) {
	t.Helper()

	// Encode: marshal the Go value, unmarshal to a map, validate against schema.
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal profile: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal marshaled profile: %v", err)
	}
	if err := resolved.Validate(decoded); err != nil {
		t.Fatalf("encoded wire record rejected by packaged schema: %v", err)
	}

	// Decode->encode: unmarshal the map into a profile and re-marshal; the
	// round-tripped bytes must still validate (tags preserve the contract).
	var roundTripped managementaccess.ManagementAccessProfile
	if err := json.Unmarshal(raw, &roundTripped); err != nil {
		t.Fatalf("unmarshal into profile: %v", err)
	}
	raw2, err := json.Marshal(roundTripped)
	if err != nil {
		t.Fatalf("re-marshal round-tripped profile: %v", err)
	}
	var decoded2 map[string]any
	if err := json.Unmarshal(raw2, &decoded2); err != nil {
		t.Fatalf("unmarshal round-tripped: %v", err)
	}
	if err := resolved.Validate(decoded2); err != nil {
		t.Fatalf("round-tripped wire record rejected by packaged schema: %v", err)
	}

	// import flags are all false after round trip.
	importCap := decoded["import"].(map[string]any)
	for _, flag := range []string{"rights_granting", "plugin_execution", "credential_redirect"} {
		if importCap[flag] != false {
			t.Fatalf("import flag %s must be false after round trip: %s", flag, raw)
		}
	}
}

func TestWireRecordJSONTagsRoundTrip(t *testing.T) {
	resolved := managementAccessResolve(t)

	// management-profile: real CA + matching fingerprint, validates both ways.
	mgmt := profileFor(t)
	wireRoundTripAgainstSchema(t, resolved, mgmt)

	// credential-export with an encryption descriptor.
	export := profileFor(t)
	export.Kind = managementaccess.CredentialExport
	export.Export = &managementaccess.EncryptionDescriptor{Algorithm: "aes-256-gcm", Encrypted: true, Nonce: "E1f2g3h4i5j6k7l8m9n0o1p2q3r4s5t6u7v8w9x0y_z"}
	wireRoundTripAgainstSchema(t, resolved, export)
}

// ---- Finding 5: golden fixtures still schema-validate (complementary) ----
