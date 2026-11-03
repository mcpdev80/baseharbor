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
		name  string
		set   func(*managementaccess.ImportCapability)
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
func (s *fakeOneUseStore) RecordIssued(context.Context, targetenrollment.Scope, string, string, time.Time) error { return nil }

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
		Scope:   authorizedScope(),
		Token:   bootstrap.Token,
		Nonce:   bootstrap.Nonce,
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

func TestWireRecordJSONTagsMatchSchema(t *testing.T) {
	p := profileFor(t)
	p.Kind = managementaccess.CredentialExport
	p.Export = &managementaccess.EncryptionDescriptor{Algorithm: "aes-256-gcm", Encrypted: true, Nonce: "E1f2g3h4i5j6k7l8m9n0o1p2q3r4s5t6u7v8w9x0y_z"}

	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}

	// Required top-level keys present.
	for _, key := range []string{"version", "kind", "installation_id", "trust", "recipient", "scope", "not_before", "not_after", "import", "storage", "export"} {
		if _, ok := decoded[key]; !ok {
			t.Fatalf("wire record missing JSON key %q: %s", key, raw)
		}
	}

	// Nested trust object: ca_pem / sha256.
	trust, ok := decoded["trust"].(map[string]any)
	if !ok {
		t.Fatalf("trust must be a nested object: %s", raw)
	}
	if _, hasCA := trust["ca_pem"]; !hasCA {
		t.Fatalf("trust missing ca_pem: %s", raw)
	}
	if _, hasSHA := trust["sha256"]; !hasSHA {
		t.Fatalf("trust missing sha256: %s", raw)
	}

	// import flags all false.
	importCap := decoded["import"].(map[string]any)
	for _, flag := range []string{"rights_granting", "plugin_execution", "credential_redirect"} {
		if importCap[flag] != false {
			t.Fatalf("import flag %s must be false: %s", flag, raw)
		}
	}
}

// ---- Finding 5: golden fixtures still schema-validate (complementary) ----

func TestGoldenFixturesSchemaValidate(t *testing.T) {
	data := readGoldenFixtures(t)
	var fixtures []struct {
		Record  string         `json:"record"`
		Fixture map[string]any `json:"fixture"`
	}
	if err := json.Unmarshal(data, &fixtures); err != nil {
		t.Fatalf("golden fixtures not valid JSON: %v", err)
	}
	if len(fixtures) < 3 {
		t.Fatalf("expected at least 3 golden fixtures, got %d", len(fixtures))
	}
}
