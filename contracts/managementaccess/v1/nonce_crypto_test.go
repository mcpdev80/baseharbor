package managementaccess_test

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/contracts/managementaccess/v1"
)

// aes256GcmNonceGolden is the golden credential-export nonce from
// profile.golden.json: a canonical base64url (no padding) encoding of exactly 12
// bytes, the AES-256-GCM AEAD nonce size fixed for management-access/v1.
const aes256GcmNonceGolden = "4fIDBAUGBwgJCgsM"

// canonicalBase64urlDecodingToBytes reports whether s is canonical base64url
// without padding and decodes to exactly want bytes. Non-canonical trailing bits
// (a value whose re-encoding would differ) fail.
func canonicalBase64urlDecodingToBytes(s string, want int) bool {
	if len(s)%4 == 1 {
		return false // impossible base64url length
	}
	dec, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return false
	}
	if len(dec) != want {
		return false
	}
	return base64.RawURLEncoding.EncodeToString(dec) == s
}

// TestNonceGoldenIsCanonicalAes256GcmNonce proves the golden nonce is a
// canonical 12-byte AES-256-GCM AEAD nonce (16 base64url characters without
// padding, full octet count).
func TestNonceGoldenIsCanonicalAes256GcmNonce(t *testing.T) {
	if !canonicalBase64urlDecodingToBytes(aes256GcmNonceGolden, 12) {
		t.Fatalf("%q must be a canonical 12-byte AES-256-GCM nonce", aes256GcmNonceGolden)
	}
}

// TestNonceAes256GcmRoundTrip proves the documented AEAD nonce actually encrypts
// and decrypts with AES-256-GCM stdlib: seal with one nonce/open key pair,
// open with the same, and confirm the golden nonce decodes to the AEAD nonce
// size.
func TestNonceAes256GcmRoundTrip(t *testing.T) {
	key := make([]byte, 32) // 32-byte AES-256 key
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatalf("new cipher: %v", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatalf("new gcm: %v", err)
	}
	if gcm.NonceSize() != 12 {
		t.Fatalf("AES-256-GCM nonce size must be 12, got %d", gcm.NonceSize())
	}
	nonce := make([]byte, gcm.NonceSize()) // 12
	if _, err := rand.Read(nonce); err != nil {
		t.Fatalf("rand nonce: %v", err)
	}
	plaintext := []byte("credential-export-secret-blob")
	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)
	dec, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if string(dec) != string(plaintext) {
		t.Fatalf("round-trip mismatch: got %q", dec)
	}
}

// TestNonceWrongLengthFailsClosed rejects a non-16-character nonce.
func TestNonceWrongLengthFailsClosed(t *testing.T) {
	bad := "ABC" // 3 chars -> impossible length
	if canonicalBase64urlDecodingToBytes(bad, 12) {
		t.Fatal("wrong-length nonce must fail closed")
	}
	badLen := "ABCDEFGHIJKLMNOPQR" // 18 chars -> more than 12 bytes
	if canonicalBase64urlDecodingToBytes(badLen, 12) {
		t.Fatal("wrong-length (too long) nonce must fail closed")
	}
}

// TestNonceNonCanonicalTrailingBitsFailsClosed proves the canonical re-encode
// guard accepts every canonical 12-byte nonce and rejects any value that does
// not decode to exactly 12 bytes. For the fixed 12-byte AEAD nonce all 96 bits
// are significant, so every canonical 16-character base64url value IS a 12-byte
// nonce; the guard therefore protects against wrong-length / undecodable values
// (the real drift surface) rather than against non-canonical trailing bits, which
// cannot exist for an octet-aligned length.
func TestNonceNonCanonicalTrailingBitsFailsClosed(t *testing.T) {
	// Canonical golden and a couple of other canonical 16-char values all decode
	// to 12 bytes.
	for _, n := range []string{aes256GcmNonceGolden, "ABCDEFGHIJKLMNOP", "9Xy3Kp8Qr2Zt7Lm4"} {
		if !canonicalBase64urlDecodingToBytes(n, 12) {
			t.Fatalf("canonical nonce %q must pass the octet-count guard", n)
		}
	}
	// 15 chars -> 11 bytes (one short): fails the 12-byte requirement.
	if canonicalBase64urlDecodingToBytes("ABCDEFGHIJKLMNOPQ", 12) {
		t.Fatal("11-byte nonce must fail the 12-byte guard")
	}
	// 17 chars -> not a valid raw base64url length (17 % 4 == 1): rejected.
	if canonicalBase64urlDecodingToBytes("ABCDEFGHIJKLMNOPQRST", 12) {
		t.Fatal("18-char nonce must fail the length guard")
	}
}

// TestNonceUnknownAlgorithmFailsClosed proves the algorithm gate is independent
// of the nonce: a canonical 12-byte nonce paired with an unknown algorithm must
// fail closed on the ALGORITHM, using a fully valid baseline profile so no other
// field (trust, validity window, storage) masks the algorithm as the rejection
// reason.
func TestNonceUnknownAlgorithmFailsClosed(t *testing.T) {
	nonce := "ABCDEFGHIJKLMNOP" // 16 chars, 12 bytes, canonical
	if !canonicalBase64urlDecodingToBytes(nonce, 12) {
		t.Fatalf("sanity: %q must be canonical 12 bytes", nonce)
	}
	// profileFor() is a fully valid baseline profile (trust + validity window
	// set), so the only way a credential-export variant fails is the field under
	// test. This avoids masking the unknown-algorithm reason with a missing
	// trust or validity window.
	badAlgoP := profileFor(t)
	badAlgoP.Kind = managementaccess.CredentialExport
	badAlgoP.Export = &managementaccess.EncryptionDescriptor{Algorithm: "xor-1", Encrypted: true, Nonce: nonce}
	if err := badAlgoP.ValidateKindEnforcement(); err == nil {
		t.Fatal("unknown algorithm must fail closed")
	}
	// A profile identical except for the supported algorithm must validate, so
	// the only reason the profile above fails is the unknown algorithm.
	goodP := profileFor(t)
	goodP.Kind = managementaccess.CredentialExport
	goodP.Export = &managementaccess.EncryptionDescriptor{Algorithm: "aes-256-gcm", Encrypted: true, Nonce: nonce}
	if err := goodP.ValidateKindEnforcement(); err != nil {
		t.Fatalf("fully valid baseline must validate, got %v", err)
	}
}

// TestNonceWrongKeyFailsClosed proves the AEAD authenticates: a different
// AES-256 key must fail to open the ciphertext (GCM tag mismatch).
func TestNonceWrongKeyFailsClosed(t *testing.T) {
	key := make([]byte, 32) // 32-byte AES-256 key
	other := make([]byte, 32)
	other[0] = 0xff
	block, _ := aes.NewCipher(key)
	otherBlock, _ := aes.NewCipher(other)
	gcm, _ := cipher.NewGCM(block)
	otherGcm, _ := cipher.NewGCM(otherBlock)
	nonce := make([]byte, gcm.NonceSize())
	plaintext := []byte("secret")
	ct := gcm.Seal(nil, nonce, plaintext, nil)
	if _, err := otherGcm.Open(nil, nonce, ct, nil); err == nil {
		t.Fatal("decryption with the wrong key must fail closed")
	}
}

// TestNonceContractRejectsInvalidAlphabetPaddingWrongLength drives the exported
// ValidateKindEnforcement boundary to prove the contract rejects a nonce whose
// alphabet is outside base64url, a padded nonce, and a nonce of the wrong
// (non-16) length for a supported algorithm. At 12 bytes there are no unused
// trailing bits, so a valid first-character substitution is a DIFFERENT valid
// 12-byte nonce and MUST be accepted — there is no canonical/non-canonical
// rejection at 16 characters.
func TestNonceContractRejectsInvalidAlphabetPaddingWrongLength(t *testing.T) {
	now := time.Now().UTC()
	caPEM, caSHA := generateTestCA(t, "BaseHarbor Management Test CA")
	baseP := managementaccess.ManagementAccessProfile{
		Kind:      managementaccess.CredentialExport,
		Version:   "baseharbor.management-access/v1",
		Trust:     managementaccess.Trust{CAPEM: caPEM, SHA256: caSHA},
		NotBefore: now.Add(-time.Minute),
		NotAfter:  now.Add(time.Hour),
		Export:    &managementaccess.EncryptionDescriptor{Algorithm: "aes-256-gcm", Encrypted: true, Nonce: aes256GcmNonceGolden},
	}
	// A fully valid baseline profile with the golden nonce validates.
	if err := baseP.ValidateKindEnforcement(); err != nil {
		t.Fatalf("fully valid baseline profile must validate, got %v", err)
	}

	// Invalid alphabet: '!' is not a base64url character.
	badAlphabet := baseP
	badAlphabet.Export.Nonce = "4!IDBAUGBwgJCgsM"
	if err := badAlphabet.ValidateKindEnforcement(); err == nil {
		t.Fatal("nonce with invalid alphabet must fail the contract boundary")
	}
	// Padding character: base64url padding '=' is outside the 16-char domain.
	padded := baseP
	padded.Export.Nonce = "4fIDBAUGBwgJCgsM="
	if err := padded.ValidateKindEnforcement(); err == nil {
		t.Fatal("nonce with padding character must fail the contract boundary")
	}
	// Wrong length 15 -> 11 bytes: one short of the 12-byte AEAD nonce.
	short15 := baseP
	short15.Export.Nonce = "4fIDBAUGBwgJCGs"
	if err := short15.ValidateKindEnforcement(); err == nil {
		t.Fatal("15-character nonce (11 bytes) must fail the contract boundary")
	}
	// Wrong length 17 -> not a valid raw base64url length (17 % 4 == 1).
	long17 := baseP
	long17.Export.Nonce = "4fIDBAUGBwgJCgsMx"
	if err := long17.ValidateKindEnforcement(); err == nil {
		t.Fatal("17-character nonce must fail the contract boundary")
	}
}

// TestNonceContractAcceptsValid16CharVariants proves that a valid 16-character
// base64url nonce is accepted regardless of its specific bytes: at 12 bytes all
// octets are significant, so changing the first character yields a DIFFERENT
// valid 12-byte nonce (not a non-canonical value), and the contract accepts it.
func TestNonceContractAcceptsValid16CharVariants(t *testing.T) {
	now := time.Now().UTC()
	caPEM, caSHA := generateTestCA(t, "BaseHarbor Management Test CA")
	mk := func(nonce string) managementaccess.ManagementAccessProfile {
		return managementaccess.ManagementAccessProfile{
			Kind:      managementaccess.CredentialExport,
			Version:   "baseharbor.management-access/v1",
			Trust:     managementaccess.Trust{CAPEM: caPEM, SHA256: caSHA},
			NotBefore: now.Add(-time.Minute),
			NotAfter:  now.Add(time.Hour),
			Export:    &managementaccess.EncryptionDescriptor{Algorithm: "aes-256-gcm", Encrypted: true, Nonce: nonce},
		}
	}
	for _, nonce := range []string{
		aes256GcmNonceGolden, // canonical golden
		"5fIDBAUGBwgJCgsM",   // first char changed -> different 12 bytes, still valid
		"ABCDEFGHIJKLMNOP",   // fully different canonical 16 chars
		"9Xy3Kp8Qr2Zt7Lm4",   // fully different canonical 16 chars
	} {
		if err := mk(nonce).ValidateKindEnforcement(); err != nil {
			t.Fatalf("valid 16-character nonce %q must be accepted, got %v", nonce, err)
		}
	}
}

// TestNonceGoldenAEADRoundtripWithDecodedGoldenNonce drives a real AES-256-GCM
// AEAD encrypt/decrypt using the bytes of the DECODED golden nonce (not a newly
// generated nonce): the golden base64url nonce decodes to exactly 12 bytes, the
// AEAD nonce size, and those exact bytes must seal and then open a blob. This
// proves the documented golden nonce is a real AEAD nonce end to end.
func TestNonceGoldenAEADRoundtripWithDecodedGoldenNonce(t *testing.T) {
	goldenBytes, err := base64.RawURLEncoding.DecodeString(aes256GcmNonceGolden)
	if err != nil {
		t.Fatalf("golden nonce must decode: %v", err)
	}
	if len(goldenBytes) != 12 {
		t.Fatalf("decoded golden nonce must be 12 bytes, got %d", len(goldenBytes))
	}
	key := make([]byte, 32) // 32-byte AES-256 key
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatalf("new cipher: %v", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatalf("new gcm: %v", err)
	}
	if gcm.NonceSize() != 12 {
		t.Fatalf("AES-256-GCM nonce size must be 12, got %d", gcm.NonceSize())
	}
	plaintext := []byte("credential-export-secret-blob")
	ciphertext := gcm.Seal(nil, goldenBytes, plaintext, nil)
	dec, err := gcm.Open(nil, goldenBytes, ciphertext, nil)
	if err != nil {
		t.Fatalf("AEAD open with decoded golden nonce must succeed: %v", err)
	}
	if !bytes.Equal(dec, plaintext) {
		t.Fatalf("round-trip mismatch: got %q", dec)
	}
}

// TestNonceContractRejectsNegativeVariantsWithFreshBaselineProfiles drives the
// exported ValidateKindEnforcement boundary for negative nonce variants. Each
// negative variant starts from its OWN fresh, fully valid baseline profile (a
// deep copy taken before mutation) so that no variant shares the exported
// EncryptionDescriptor pointer of a shared baseline and one variant's mutation
// cannot mask another's failure. The golden baseline validates first.
func TestNonceContractRejectsNegativeVariantsWithFreshBaselineProfiles(t *testing.T) {
	now := time.Now().UTC()
	caPEM, caSHA := generateTestCA(t, "BaseHarbor Management Test CA")
	// freshBaseline returns a brand-new valid profile per call (no shared Export pointer).
	freshBaseline := func() managementaccess.ManagementAccessProfile {
		return managementaccess.ManagementAccessProfile{
			Kind:      managementaccess.CredentialExport,
			Version:   "baseharbor.management-access/v1",
			Trust:     managementaccess.Trust{CAPEM: caPEM, SHA256: caSHA},
			NotBefore: now.Add(-time.Minute),
			NotAfter:  now.Add(time.Hour),
			Export:    &managementaccess.EncryptionDescriptor{Algorithm: "aes-256-gcm", Encrypted: true, Nonce: aes256GcmNonceGolden},
		}
	}
	base := freshBaseline()
	if err := base.ValidateKindEnforcement(); err != nil {
		t.Fatalf("fully valid baseline profile must validate, got %v", err)
	}

	negatives := []string{
		"4!IDBAUGBwgJCgsM",  // invalid alphabet: '!' is not base64url
		"4fIDBAUGBwgJCgsM=", // padding character '=' outside the 16-char domain
		"4fIDBAUGBwgJCGs",   // 15 chars -> 11 bytes: short of the 12-byte AEAD nonce
		"4fIDBAUGBwgJCgsMx", // 17 chars -> not a valid raw base64url length
	}
	for _, bad := range negatives {
		// Deep-copy the fresh baseline before mutating its Nonce.
		p := freshBaseline()
		pp := &p
		pp.Export = &managementaccess.EncryptionDescriptor{
			Algorithm: "aes-256-gcm",
			Encrypted: true,
			Nonce:     bad,
		}
		if err := pp.ValidateKindEnforcement(); err == nil {
			t.Fatalf("nonce %q must fail the contract boundary", bad)
		}
	}
}
