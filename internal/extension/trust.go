package extension

import (
	"context"
	"strings"
)

const TrustVersion = "baseharbor.extension-trust/v1"

type VerificationStatus string

const (
	VerificationVerified     VerificationStatus = "verified"
	VerificationUnverifiable VerificationStatus = "unverifiable"
	VerificationInvalid      VerificationStatus = "invalid"
)

type Verification struct {
	SchemaVersion       string             `json:"schema_version"`
	Status              VerificationStatus `json:"status"`
	Digest              string             `json:"digest,omitempty"`
	Publisher           string             `json:"publisher,omitempty"`
	SignatureVerified   bool               `json:"signature_verified"`
	SBOMVerified        bool               `json:"sbom_verified"`
	AttestationVerified bool               `json:"attestation_verified"`
	Code                string             `json:"code,omitempty"`
	Next                string             `json:"next,omitempty"`
}

// Verifier is an operator-configured verification backend, not descriptor data.
// Implementations verify fetched bytes/evidence against the immutable digest.
// A publisher returned here must come from verified evidence, never a claim.
type Verifier interface {
	Verify(context.Context, Metadata) (Verification, error)
}

type TrustStatus string

const (
	TrustTrusted TrustStatus = "trusted"
	TrustDenied  TrustStatus = "denied"
)

type TrustDecision struct {
	SchemaVersion   string      `json:"schema_version"`
	Status          TrustStatus `json:"status"`
	PolicyReference string      `json:"policy_reference,omitempty"`
	Code            string      `json:"code,omitempty"`
	Next            string      `json:"next,omitempty"`
}

type TrustError struct {
	Code string `json:"code"`
	Next string `json:"next"`
}

func (e *TrustError) Error() string { return "extension trust: " + e.Code }

func VerifyArtifact(ctx context.Context, metadata Metadata, verifier Verifier) Verification {
	result := Verification{SchemaVersion: TrustVersion, Status: VerificationUnverifiable,
		Code: "verifier_unavailable", Next: "Configure a verification backend for this artifact."}
	if metadata.Validate() != nil {
		result.Status, result.Code = VerificationInvalid, "invalid_metadata"
		result.Next = "Correct the extension descriptor before verification."
		return result
	}
	if metadata.Artifact.Digest == "" {
		result.Code, result.Next = "digest_missing", "Resolve an immutable artifact digest before verification."
		return result
	}
	if verifier == nil {
		return result
	}
	verified, err := verifier.Verify(ctx, metadata)
	// Backend errors can contain registry credentials. Never echo their text.
	if err != nil {
		result.Code, result.Next = "verification_unavailable", "Check the configured verification backend and retry."
		return result
	}
	if verified.Status == VerificationUnverifiable {
		return result
	}
	if verified.Status != VerificationVerified || verified.Digest != metadata.Artifact.Digest {
		result.Status, result.Code = VerificationInvalid, "artifact_verification_invalid"
		result.Next = "Reject the artifact and verify its immutable content and evidence."
		return result
	}
	if metadata.Provenance.Publisher != "" && verified.Publisher != metadata.Provenance.Publisher {
		result.Status, result.Code = VerificationInvalid, "publisher_claim_mismatch"
		result.Next = "Correct the publisher claim or reject the artifact."
		return result
	}
	// Only canonical public fields cross the boundary; backend messages do not.
	return Verification{SchemaVersion: TrustVersion, Status: VerificationVerified,
		Digest: verified.Digest, Publisher: verified.Publisher,
		SignatureVerified: verified.SignatureVerified, SBOMVerified: verified.SBOMVerified,
		AttestationVerified: verified.AttestationVerified}
}

func (p TrustPolicy) Evaluate(metadata Metadata, verification Verification) TrustDecision {
	decision := TrustDecision{SchemaVersion: TrustVersion, Status: TrustDenied, PolicyReference: p.Reference}
	deny := func(code, next string) TrustDecision {
		decision.Code, decision.Next = code, next
		return decision
	}
	if metadata.Validate() != nil {
		return deny("invalid_metadata", "Correct the extension descriptor.")
	}
	if verification.SchemaVersion != TrustVersion || verification.Status == VerificationInvalid {
		return deny("verification_invalid", "Reject invalid artifact evidence.")
	}
	if verification.Status != VerificationVerified && verification.Status != VerificationUnverifiable {
		return deny("verification_invalid", "Use a supported verification result.")
	}
	if verification.Status == VerificationVerified && (metadata.Artifact.Digest == "" || verification.Digest != metadata.Artifact.Digest) {
		return deny("verification_invalid", "Verify the selected immutable artifact digest.")
	}
	if p.Validate(metadata) != nil {
		return deny("trust_metadata_required", "Supply the immutable digest and public evidence references required by policy.")
	}
	requiresVerified := p.RequireVerification || p.RequireSignature || p.RequireSBOM || p.RequireAttestation || len(p.AllowedPublishers) > 0
	if requiresVerified && verification.Status != VerificationVerified {
		return deny("artifact_unverifiable", "Configure verification and provide verifiable artifact evidence.")
	}
	if p.RequireSignature && !verification.SignatureVerified || p.RequireSBOM && !verification.SBOMVerified || p.RequireAttestation && !verification.AttestationVerified {
		return deny("evidence_unverified", "Verify each evidence type required by trust policy.")
	}
	if len(p.AllowedPublishers) > 0 {
		allowed := false
		for _, publisher := range p.AllowedPublishers {
			if strings.TrimSpace(publisher) != "" && publisher == verification.Publisher {
				allowed = true
			}
		}
		if !allowed {
			return deny("publisher_denied", "Use an artifact from a publisher allowed by the effective policy.")
		}
	}
	decision.Status = TrustTrusted
	return decision
}
