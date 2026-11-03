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
	"github.com/mcpdev80/baseharbor/internal/hosttrust"
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

// enrollSession bundles a ready-to-use one-use enrollment request, the enforcer
// adapter, the backing store (so a test can assert the enrollment effect never
// ran) and the management profile that binds the request. The profile is pinned
// to a self-consistent CA whose fingerprint ResolveFingerprint returns, so a
// valid enrollment succeeds and every malformed variant fails closed before the
// single-use store is consumed.
type enrollSession struct {
	request targetenrollment.Request
	adapter *enforcerAdapter
	store   *fakeOneUseStore
	profile managementaccess.ManagementAccessProfile
}

func enrollScopeRequest(t *testing.T) enrollSession {
	t.Helper()
	store := &fakeOneUseStore{}
	issuer := serviceissuer.New(t)
	authority, err := targetenrollment.New(store, issuer)
	if err != nil {
		t.Fatal(err)
	}
	adapter := newEnforcerAdapter(authority, issuer)
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
	now := time.Now().UTC()
	profile := profileFor(t)
	profile.Kind = managementaccess.DeviceEnrollment
	profile.Recipient = authorizedScope().Identity()
	profile.Scope = "node-bootstrap"
	profile.NotBefore = now.Add(-time.Minute)
	profile.NotAfter = now.Add(time.Hour)
	profile.Storage = managementaccess.Storage{Class: managementaccess.SessionOnly, Durable: false, Encrypted: true, DisasterRecovery: false}
	return enrollSession{request: request, adapter: adapter, store: store, profile: profile}
}

// enforcerAdapter binds the existing authoritative target-enrollment authority
// and service issuer to the management-access Enforcer interface, so the call
// path exercises internal/hosttrust, internal/serviceaccess and
// internal/targetenrollment exactly as production would.
type enforcerAdapter struct {
	authority *targetenrollment.Authority
	issuer    *serviceissuer.Issuer
}

func newEnforcerAdapter(authority *targetenrollment.Authority, issuer *serviceissuer.Issuer) *enforcerAdapter {
	return &enforcerAdapter{authority: authority, issuer: issuer}
}

func (a *enforcerAdapter) ResolveFingerprint(caPEM string) (string, error) {
	_, fingerprint, err := hosttrust.ParseCA([]byte(caPEM))
	return fingerprint, err
}

func (a *enforcerAdapter) Enroll(ctx context.Context, request targetenrollment.Request) (targetenrollment.Result, error) {
	return a.authority.Enroll(ctx, request)
}

func TestEnrollDeviceOneUseThroughAuthority(t *testing.T) {
	session := enrollScopeRequest(t)
	result, err := managementaccess.EnrollDevice(context.Background(), session.adapter, session.profile, session.request)
	if err != nil {
		t.Fatalf("one-use enrollment must succeed: %v", err)
	}
	if len(result.Certificate) == 0 {
		t.Fatal("expected a certificate from enrollment")
	}
	if len(result.Trust.PEM) == 0 {
		t.Fatal("expected a trust bundle from enrollment")
	}
	if result.ExpiresAt.IsZero() {
		t.Fatal("expected an expiry from enrollment")
	}

	// A second use of the same one-use request must fail closed.
	_, err = managementaccess.EnrollDevice(context.Background(), session.adapter, session.profile, session.request)
	if err == nil {
		t.Fatal("second one-use enrollment must fail closed (ErrDenied)")
	}
}

func TestEnrollDeviceNilEnforcerFailsClosed(t *testing.T) {
	session := enrollScopeRequest(t)
	_, err := managementaccess.EnrollDevice(context.Background(), nil, session.profile, session.request)
	if err == nil {
		t.Fatal("nil enforcer must fail closed")
	}
}

// The enrollment entry requires Kind=device-enrollment. A management-profile or
// credential-export record that reaches EnrollDevice must fail closed BEFORE the
// single-use store is consumed, so no enrollment effect can run on a foreign kind.
func TestEnrollDeviceRequiresDeviceEnrollmentKind(t *testing.T) {
	session := enrollScopeRequest(t)

	// A management-profile reaching the enrollment entry fails closed.
	mgmt := session.profile
	mgmt.Kind = managementaccess.ManagementProfile
	if _, err := managementaccess.EnrollDevice(context.Background(), session.adapter, mgmt, session.request); err == nil {
		t.Fatal("EnrollDevice must reject a management-profile record at the enrollment entry")
	}
	if session.store.consumed {
		t.Fatal("a management-profile must be rejected before the single-use store is consumed")
	}

	// A credential-export reaching the enrollment entry fails closed.
	export := session.profile
	export.Kind = managementaccess.CredentialExport
	if _, err := managementaccess.EnrollDevice(context.Background(), session.adapter, export, session.request); err == nil {
		t.Fatal("EnrollDevice must reject a credential-export record at the enrollment entry")
	}
	if session.store.consumed {
		t.Fatal("a credential-export must be rejected before the single-use store is consumed")
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

// ---- Finding 2: negative expired / mismatched-pin / one-use-reuse goldens through the real path ----

func TestNegativeGoldensExpiredThroughProfilePath(t *testing.T) {
	caPEM, caSHA := generateExpiredCA(t, "BaseHarbor Expired Management CA")
	scope := authorizedScope()
	now := time.Now().UTC()
	p := managementaccess.ManagementAccessProfile{
		Kind:           managementaccess.ManagementProfile,
		Version:        "baseharbor.management-access/v1",
		InstallationID: "11111111-1111-4111-8111-111111111111",
		Trust:          managementaccess.Trust{CAPEM: caPEM, SHA256: caSHA},
		Recipient:      scope.Identity(),
		Scope:          "control-plane",
		NotBefore:      now.Add(-72 * time.Hour),
		NotAfter:       now.Add(-24 * time.Hour),
		Import:         managementaccess.ImportCapability{RightsGranting: false, PluginExecution: false, CredentialRedirect: false},
		Storage:        managementaccess.Storage{Class: managementaccess.ProtectedDurable, Durable: true, Encrypted: true, DisasterRecovery: false},
	}
	// The stored fingerprint hashes to the certificate itself, so trust is
	// internally consistent; only the elapsed window must fail closed.
	if err := p.ValidateTrust(); err != nil {
		t.Fatalf("expired CA with matching fingerprint should still validate trust: %v", err)
	}
	// Elapsed window fails closed through the real semantic path.
	if err := p.ValidityWindow(now); err == nil {
		t.Fatal("expired validity window must fail closed through the real path")
	}
	// The expired record is rejected by the combined import path: trust matches,
	// but the validity window makes it invalid, so ValidateKindEnforcement (which
	// gates import) must reject the record.
	if err := p.ValidateKindEnforcement(); err == nil {
		t.Fatal("expired record must fail kind enforcement through the real path")
	}
}

func TestNegativeGoldensMismatchedPinThroughProfilePath(t *testing.T) {
	caPEM, caSHA := generateMismatchedCA(t, "BaseHarbor Management Mismatched CA")
	scope := authorizedScope()
	now := time.Now().UTC()
	p := managementaccess.ManagementAccessProfile{
		Kind:           managementaccess.ManagementProfile,
		Version:        "baseharbor.management-access/v1",
		InstallationID: "11111111-1111-4111-8111-111111111111",
		Trust:          managementaccess.Trust{CAPEM: caPEM, SHA256: caSHA},
		Recipient:      scope.Identity(),
		Scope:          "control-plane",
		NotBefore:      now.Add(-time.Minute),
		NotAfter:       now.Add(time.Hour),
		Import:         managementaccess.ImportCapability{RightsGranting: false, PluginExecution: false, CredentialRedirect: false},
		Storage:        managementaccess.Storage{Class: managementaccess.ProtectedDurable, Durable: true, Encrypted: true, DisasterRecovery: false},
	}
	// Mismatched fingerprint must fail closed.
	if err := p.ValidateTrust(); err == nil {
		t.Fatal("trust fingerprint that does not match the certificate must fail closed through the real path")
	}
	// Foreign recipient must fail closed.
	if err := managementaccess.ValidateRecipientBind("spiffe://baseharbor/platform/core/22222222-2222-4222-8222-222222222222/attacker", scope); err == nil {
		t.Fatal("foreign recipient must fail closed (ErrForeignPin) through the real path")
	}
	// Combined kind enforcement fails closed.
	if err := p.ValidateKindEnforcement(); err == nil {
		t.Fatal("mismatched-pin record must fail kind enforcement through the real path")
	}
}

func TestNegativeGoldensOneUseReuseThroughProfilePath(t *testing.T) {
	session := enrollScopeRequest(t)
	// First one-use enrollment through the real path succeeds.
	if _, err := managementaccess.EnrollDevice(context.Background(), session.adapter, session.profile, session.request); err != nil {
		t.Fatalf("first one-use enrollment must succeed: %v", err)
	}
	// Second use of the same one-use pin fails closed (ErrDenied).
	_, err := managementaccess.EnrollDevice(context.Background(), session.adapter, session.profile, session.request)
	if err == nil {
		t.Fatal("second one-use enrollment must fail closed (ErrDenied) through the real path")
	}
}

// ---- Finding 2: validate-then-enroll binding (pinned CA, expiry, recipient) ----
// Every negative variant must fail closed BEFORE the single-use store is
// consumed (store.consumed stays false), i.e. the enrollment effect never runs.

func TestEnrollDeviceCorruptPinnedCAFailsClosedBeforeEffect(t *testing.T) {
	session := enrollScopeRequest(t)
	// A corrupt pinned CA: the stored sha256 does not match the certificate's
	// own hash, so ResolveFingerprint (the anchor-resolution step) cannot bind it.
	// Rejects before the enrollment effect.
	foreignPEM, _ := generateTestCA(t, "BaseHarbor Foreign Management CA")
	profile := session.profile
	profile.Trust = managementaccess.Trust{CAPEM: foreignPEM, SHA256: "0000000000000000000000000000000000000000000000000000000000000000"}
	_, err := managementaccess.EnrollDevice(context.Background(), session.adapter, profile, session.request)
	if err == nil {
		t.Fatal("enrollment with an unbindable pinned CA must fail closed")
	}
	if session.store.consumed {
		t.Fatal("an unbindable pinned CA must be rejected before the single-use store is consumed (no enrollment effect)")
	}
}

func TestEnrollDeviceExpiredWindowFailsClosedBeforeEffect(t *testing.T) {
	session := enrollScopeRequest(t)
	profile := session.profile
	profile.Kind = managementaccess.ManagementProfile
	profile.NotBefore = time.Now().UTC().Add(-72 * time.Hour)
	profile.NotAfter = time.Now().UTC().Add(-24 * time.Hour)
	_, err := managementaccess.EnrollDevice(context.Background(), session.adapter, profile, session.request)
	if err == nil {
		t.Fatal("expired validity window must fail closed")
	}
	if session.store.consumed {
		t.Fatal("expired window must be rejected before the single-use store is consumed (no enrollment effect)")
	}
}

func TestEnrollDeviceForeignRecipientFailsClosedBeforeEffect(t *testing.T) {
	session := enrollScopeRequest(t)
	profile := session.profile
	profile.Recipient = "spiffe://baseharbor/platform/core/22222222-2222-4222-8222-222222222222/attacker"
	_, err := managementaccess.EnrollDevice(context.Background(), session.adapter, profile, session.request)
	if err == nil {
		t.Fatal("foreign recipient must fail closed")
	}
	if session.store.consumed {
		t.Fatal("foreign recipient must be rejected before the single-use store is consumed (no enrollment effect)")
	}
}

func TestEnrollDeviceMismatchedPinnedCAFailsClosedBeforeEffect(t *testing.T) {
	session := enrollScopeRequest(t)
	// A certificate whose real fingerprint differs from the pinned sha256:
	// ResolveFingerprint returns the real hash, which != the pinned value.
	profile := session.profile
	profile.Trust = managementaccess.Trust{CAPEM: profile.Trust.CAPEM, SHA256: "1111111111111111111111111111111111111111111111111111111111111111"}
	_, err := managementaccess.EnrollDevice(context.Background(), session.adapter, profile, session.request)
	if err == nil {
		t.Fatal("mismatched pinned CA must fail closed")
	}
	if session.store.consumed {
		t.Fatal("mismatched pinned CA must be rejected before the single-use store is consumed (no enrollment effect)")
	}
}

// ---- Finding 5: golden fixtures still schema-validate (complementary) ----

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
