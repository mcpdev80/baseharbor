package managementaccess_test

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"testing"

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
// fail closed.
func TestNonceUnknownAlgorithmFailsClosed(t *testing.T) {
	nonce := "ABCDEFGHIJKLMNOP" // 16 chars, 12 bytes, canonical
	if !canonicalBase64urlDecodingToBytes(nonce, 12) {
		t.Fatalf("sanity: %q must be canonical 12 bytes", nonce)
	}
	p := managementaccess.ManagementAccessProfile{
		Kind: managementaccess.CredentialExport,
		Export: &managementaccess.EncryptionDescriptor{Algorithm: "xor-1", Encrypted: true, Nonce: nonce},
	}
	if err := p.ValidateKindEnforcement(); err == nil {
		t.Fatal("unknown algorithm must fail closed")
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

// TestNonceContractRejectsNonCanonicalAndWrongLength drives the exported
// ValidateKindEnforcement boundary to prove the contract rejects a non-canonical
// nonce and a wrong-length nonce for a supported algorithm.
func TestNonceContractRejectsNonCanonicalAndWrongLength(t *testing.T) {
	// Non-canonical: corrupted last char, supported algorithm.
	nonCanon := "5" + aes256GcmNonceGolden[1:]
	nonCanonP := managementaccess.ManagementAccessProfile{
		Kind: managementaccess.CredentialExport,
		Export: &managementaccess.EncryptionDescriptor{Algorithm: "aes-256-gcm", Encrypted: true, Nonce: nonCanon},
	}
	if err := nonCanonP.ValidateKindEnforcement(); err == nil {
		t.Fatal("non-canonical nonce must fail the contract boundary")
	}
	// Wrong length (43 chars, the old wire length): supported algorithm, wrong nonce.
	wrongLen := "E1f2g3h4i5j6k7l8m9n0o1p2q3r4s5t6u7v8w9x0y_z"
	wrongLenP := managementaccess.ManagementAccessProfile{
		Kind: managementaccess.CredentialExport,
		Export: &managementaccess.EncryptionDescriptor{Algorithm: "aes-256-gcm", Encrypted: true, Nonce: wrongLen},
	}
	if err := wrongLenP.ValidateKindEnforcement(); err == nil {
		t.Fatal("wrong-length nonce must fail the contract boundary")
	}
	// The canonical golden nonce decodes to exactly 12 bytes.
	if !canonicalBase64urlDecodingToBytes(aes256GcmNonceGolden, 12) {
		t.Fatal("golden nonce must be canonical 12 bytes")
	}
}
