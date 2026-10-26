package extension

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	jose "github.com/go-jose/go-jose/v4"
	"io"
	"strings"
	"testing"
)

type fixtureReader map[string]string

func (r fixtureReader) Open(ctx context.Context, ref string) (io.ReadCloser, error) {
	data, ok := r[ref]
	if !ok {
		return nil, errors.New("private registry password=DO_NOT_LEAK")
	}
	return io.NopCloser(strings.NewReader(data)), nil
}
func signedFixture(t *testing.T, key ed25519.PrivateKey, digest, predicateType string) string {
	t.Helper()
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.EdDSA, Key: key}, (&jose.SignerOptions{}).WithHeader("kid", "publisher-key"))
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(map[string]any{"_type": InTotoStatementV1, "subject": []any{map[string]any{"name": "artifact", "digest": map[string]string{"sha256": strings.TrimPrefix(digest, "sha256:")}}}, "predicateType": predicateType, "predicate": map[string]any{"fixture": true}})
	if err != nil {
		t.Fatal(err)
	}
	signed, err := signer.Sign(payload)
	if err != nil {
		t.Fatal(err)
	}
	compact, err := signed.CompactSerialize()
	if err != nil {
		t.Fatal(err)
	}
	return compact
}
func TestJWSVerifierAuthenticatesBytesPublisherAndEvidenceForEveryFamily(t *testing.T) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte("immutable OCI manifest"))
	digest := "sha256:" + hex.EncodeToString(sum[:])
	for _, family := range []Family{FamilyProvider, FamilyDevelopment, FamilyRuntime, FamilyDelivery, FamilyWorkloadSource, FamilyTargetAccess} {
		t.Run(string(family), func(t *testing.T) {
			metadata := Metadata{SchemaVersion: DescriptorVersion, ID: "fixture/extension", Version: "1.0.0", Family: family, Artifact: Artifact{OCIReference: "registry.example/extension:stable", Digest: digest}, Provenance: Provenance{Publisher: "fixture-publisher", SignatureRef: "signature", SBOMRef: "sbom", AttestationRef: "provenance"}}
			reader := fixtureReader{"registry.example/extension:stable@" + digest: "immutable OCI manifest", "signature": signedFixture(t, private, digest, SLSAProvenanceV1), "sbom": signedFixture(t, private, digest, SPDXPredicateV2), "provenance": signedFixture(t, private, digest, SLSAProvenanceV1)}
			verifier := JWSVerifier{Reader: reader, Keys: map[string]PublisherKey{"publisher-key": {Publisher: "fixture-publisher", PublicKey: public}}}
			verification := VerifyArtifact(context.Background(), metadata, verifier)
			policy := TrustPolicy{RequireVerification: true, RequireSignature: true, RequireSBOM: true, RequireAttestation: true, AllowedPublishers: []string{"fixture-publisher"}}
			if verification.Status != VerificationVerified || policy.Evaluate(metadata, verification).Status != TrustTrusted {
				t.Fatalf("authentic artifact rejected: %#v", verification)
			}
			reader["signature"] = signedFixture(t, private, "sha256:"+strings.Repeat("a", 64), SLSAProvenanceV1)
			if got := VerifyArtifact(context.Background(), metadata, verifier); got.Status != VerificationInvalid {
				t.Fatal("replayed evidence accepted")
			}
			reader["signature"] = signedFixture(t, private, digest, SLSAProvenanceV1)
			reader["registry.example/extension:stable@"+digest] = "tampered manifest"
			if got := VerifyArtifact(context.Background(), metadata, verifier); got.Status != VerificationInvalid {
				t.Fatal("tampered content accepted")
			}
			reader["registry.example/extension:stable@"+digest] = "immutable OCI manifest"
			_, wrong, _ := ed25519.GenerateKey(rand.Reader)
			reader["signature"] = signedFixture(t, wrong, digest, SLSAProvenanceV1)
			if got := VerifyArtifact(context.Background(), metadata, verifier); got.Status != VerificationInvalid {
				t.Fatal("untrusted signature accepted")
			}
			delete(reader, "signature")
			got := VerifyArtifact(context.Background(), metadata, verifier)
			encoded, _ := json.Marshal(got)
			if got.Status != VerificationUnverifiable || policy.Evaluate(metadata, got).Status != TrustDenied || strings.Contains(string(encoded), "DO_NOT_LEAK") {
				t.Fatalf("unsafe fetch failure: %s", encoded)
			}
		})
	}
}
