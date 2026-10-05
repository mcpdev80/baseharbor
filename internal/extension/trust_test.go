package extension

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

type testVerifier struct {
	result Verification
	err    error
}

func (v testVerifier) Verify(context.Context, Metadata) (Verification, error) { return v.result, v.err }

func trustFixture() Metadata {
	return Metadata{SchemaVersion: DescriptorVersion, ID: "acme/sql", Version: "1.0.0", Family: FamilyProvider,
		Artifact:   Artifact{OCIReference: "ghcr.io/acme/sql:1.0.0", Digest: "sha256:" + strings.Repeat("a", 64)},
		Provenance: Provenance{Publisher: "acme", SignatureRef: "oci://public/signature"}}
}

func TestVerifiedArtifactCanStillBeDeniedByPublisherPolicy(t *testing.T) {
	metadata := trustFixture()
	verified := VerifyArtifact(context.Background(), metadata, testVerifier{result: Verification{
		Status: VerificationVerified, Digest: metadata.Artifact.Digest, Publisher: "acme", SignatureVerified: true}})
	if verified.Status != VerificationVerified {
		t.Fatalf("verification = %#v", verified)
	}
	policy := TrustPolicy{RequireSignature: true, AllowedPublishers: []string{"other"}, Reference: "prod/publishers"}
	decision := policy.Evaluate(metadata, verified)
	if decision.Status != TrustDenied || decision.Code != "publisher_denied" || decision.PolicyReference != policy.Reference {
		t.Fatalf("decision = %#v", decision)
	}
	policy.AllowedPublishers = []string{"acme"}
	if got := policy.Evaluate(metadata, verified); got.Status != TrustTrusted {
		t.Fatalf("decision = %#v", got)
	}
}

func TestArtifactVerificationRejectsMismatchedEvidence(t *testing.T) {
	metadata := trustFixture()
	for _, result := range []Verification{
		{Status: VerificationVerified, Digest: "sha256:" + strings.Repeat("b", 64), Publisher: "acme"},
		{Status: VerificationVerified, Digest: metadata.Artifact.Digest, Publisher: "impostor"},
		{Status: VerificationInvalid, Digest: metadata.Artifact.Digest},
		{Status: "future-status", Digest: metadata.Artifact.Digest},
	} {
		verified := VerifyArtifact(context.Background(), metadata, testVerifier{result: result})
		if verified.Status != VerificationInvalid {
			t.Fatalf("accepted evidence %#v", result)
		}
		if got := (TrustPolicy{}).Evaluate(metadata, verified); got.Status != TrustDenied {
			t.Fatalf("accepted invalid evidence %#v", got)
		}
	}
}

func TestVerifierUnavailableIsTypedAndSecretSafe(t *testing.T) {
	metadata := trustFixture()
	for _, verifier := range []Verifier{nil, testVerifier{err: errors.New("registry password=DO_NOT_LEAK")}} {
		verified := VerifyArtifact(context.Background(), metadata, verifier)
		decision := (TrustPolicy{RequireSignature: true}).Evaluate(metadata, verified)
		if verified.Status != VerificationUnverifiable || decision.Status != TrustDenied || decision.Code != "artifact_unverifiable" {
			t.Fatalf("unexpected unverifiable result: %#v %#v", verified, decision)
		}
		encoded, err := json.Marshal(verified)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(encoded), "DO_NOT_LEAK") {
			t.Fatal("backend error leaked")
		}
	}
}

func TestPolicyRejectsDigestReplayAndUnverifiedRequiredEvidence(t *testing.T) {
	metadata := trustFixture()
	verified := Verification{SchemaVersion: TrustVersion, Status: VerificationVerified, Digest: metadata.Artifact.Digest, Publisher: "acme"}
	if got := (TrustPolicy{RequireSignature: true}).Evaluate(metadata, verified); got.Code != "evidence_unverified" {
		t.Fatalf("decision = %#v", got)
	}
	verified.Digest = "sha256:" + strings.Repeat("b", 64)
	if got := (TrustPolicy{}).Evaluate(metadata, verified); got.Status != TrustDenied {
		t.Fatal("evidence replay across digests accepted")
	}
}

func TestTrustMetadataSupportsIndependentFamiliesAndRejectsCredentialReferences(t *testing.T) {
	for _, family := range []Family{FamilyProvider, FamilyDevelopment, FamilyRuntime, FamilyDelivery, FamilyWorkloadSource, FamilyTargetAccess} {
		metadata := trustFixture()
		metadata.Family = family
		if err := metadata.Validate(); err != nil {
			t.Fatal(err)
		}
	}
	metadata := trustFixture()
	metadata.Provenance.SignatureRef = "https://user:password@registry.example/signature"
	if err := metadata.Validate(); err == nil {
		t.Fatal("credential-bearing public reference accepted")
	}
	metadata = trustFixture()
	metadata.Artifact.OCIReference += "@sha256:" + strings.Repeat("b", 64)
	if err := metadata.Validate(); err == nil {
		t.Fatal("contradicting immutable digests accepted")
	}
}

func TestPermissivePolicyRejectsUnverifiableEvidenceClaims(t *testing.T) {
	metadata := trustFixture()
	for _, verification := range []Verification{
		{SchemaVersion: TrustVersion, Status: VerificationUnverifiable, SignatureVerified: true},
		{SchemaVersion: TrustVersion, Status: VerificationUnverifiable, SBOMVerified: true},
		{SchemaVersion: TrustVersion, Status: VerificationUnverifiable, AttestationVerified: true},
		{SchemaVersion: TrustVersion, Status: VerificationUnverifiable, Publisher: "unverified-claim"},
	} {
		decision := (TrustPolicy{}).Evaluate(metadata, verification)
		if decision.Status != TrustDenied || decision.Code != "verification_invalid" {
			t.Fatalf("inconsistent evidence accepted: %#v", decision)
		}
	}
}
