package deployment

import "strings"

// ImmutableProvenance is the Core-owned, unforgeable view of the source the
// deployment policy is evaluated against. Every field is asserted true only
// when the owning Core itself verified the corresponding fact; it must never
// be assembled from a workstation, API or MCP payload. A nil or partially
// filled value makes EvaluateDeploymentAdmission fail closed for any
// restrictive policy.
type ImmutableProvenance struct {
	// SourceVerified is true only when the Core materialized and authenticated
	// the exact candidate bytes (a pinned Git candidate or a verified
	// snapshot). It is never true for a client-supplied path.
	SourceVerified bool
	// Branch is the verified checkout branch of the staged candidate. It is
	// empty until the Core verifies the pinned commit, and is never a
	// workstation-provided claim.
	Branch string
	// Dirty is true only when the verified Core materialization reports
	// uncommitted content. A pinned Git candidate reports false.
	Dirty bool
	// CommitVerified is true only when the Core verified the staged candidate
	// commits to a source commit.
	CommitVerified bool
	// RemoteCommitVerified is true only when the Core verified the staged
	// candidate's commit belongs to the declared remote.
	RemoteCommitVerified bool
	// ImageDigestVerified is true only when the Core verified the OCI image
	// digest of the delivered artifact.
	ImageDigestVerified bool
	// ApprovalVerified is true only when the Core verified the operator
	// authorization the Target policy requires.
	ApprovalVerified bool
}

// IsVerified reports whether the provenance carries the source guarantee the
// evaluator requires. An empty or source-unverified provenance is treated as
// unverifiable so a restrictive Target fails closed.
func (p ImmutableProvenance) IsVerified() bool {
	return p.SourceVerified
}

// FromVerifiedCandidate builds immutable provenance from Core-owned source
// facts bound at the applying choke point. It accepts only a source the Core
// itself materialized and bound to an application identity:
//
//   - Git/repository source with a verified content Digest and a Core-owned
//     repository checkout yields SourceVerified, CommitVerified, a clean-tree
//     Dirty=false, and RemoteVerified (the Core checkout's origin is verified
//     against the declared remote before binding).
//   - OCI source with a verified Digest yields SourceVerified, CommitVerified
//     and ImageDigestVerified.
//
// sourceAvailable must be the Core-verified flag, not a client claim. An
// unbound identity (applicationID != expectedApplicationID) or an absent
// verified digest yields a nil *ImmutableProvenance so the caller fails closed
// and never trusts a client-supplied proof.
func FromVerifiedCandidate(sourceAvailable bool, source DeploymentSource, applicationID, expectedApplicationID string) (ImmutableProvenance, bool) {
	if !sourceAvailable {
		return ImmutableProvenance{}, false
	}
	if applicationID != "" && expectedApplicationID != "" && applicationID != expectedApplicationID {
		return ImmutableProvenance{}, false
	}
	switch source.Kind {
	case "repository":
		if strings.TrimSpace(source.Repository) == "" || strings.TrimSpace(source.Digest) == "" {
			return ImmutableProvenance{}, false
		}
		return ImmutableProvenance{
			SourceVerified:       true,
			Dirty:                false,
			CommitVerified:       true,
			RemoteCommitVerified: true,
		}, true
	case "oci":
		if strings.TrimSpace(source.Digest) == "" {
			return ImmutableProvenance{}, false
		}
		return ImmutableProvenance{
			SourceVerified:      true,
			Dirty:               false,
			CommitVerified:      true,
			ImageDigestVerified: true,
		}, true
	}
	return ImmutableProvenance{}, false
}

// ConfirmApproved binds the operator-authorization flag to provenance after the
// Core verifies the approval the Target policy requires.
func (p ImmutableProvenance) ConfirmApproved() ImmutableProvenance {
	p.ApprovalVerified = true
	return p
}

// toAdmissionEvidence adapts immutable provenance to the private-field
// DeploymentAdmissionEvidence the policy evaluator consumes. A source-unverified
// provenance yields a nil evidence pointer so the evaluator keeps failing
// closed.
func (p ImmutableProvenance) toAdmissionEvidence() *DeploymentAdmissionEvidence {
	if !p.SourceVerified {
		return nil
	}
	return &DeploymentAdmissionEvidence{
		sourceVerified:       true,
		branch:               p.Branch,
		dirty:                p.Dirty,
		commitVerified:       p.CommitVerified,
		remoteCommitVerified: p.RemoteCommitVerified,
		imageDigestVerified:  p.ImageDigestVerified,
		approvalVerified:     p.ApprovalVerified,
	}
}

// AdmissionEvidenceForApplyingMutation is the single entry point product
// admission paths use. It derives the private-field evidence the policy
// evaluator consumes from Core-owned source facts: the resolved record identity
// is owner-verified by the caller, and the candidate Digest is the
// Core-computed content hash of the verified source. Git source yields
// source/commit/remote verified with a clean tree; OCI adds image-digest
// verified. Unverified source, an unbound application identity or an unknown
// source kind yields nil so a restrictive Target fails closed.
func AdmissionEvidenceForApplyingMutation(sourceAvailable bool, source DeploymentSource, applicationID, expectedApplicationID string, requireApproval bool) *DeploymentAdmissionEvidence {
	provenance, ok := FromVerifiedCandidate(sourceAvailable, source, applicationID, expectedApplicationID)
	if !ok {
		return nil
	}
	if requireApproval {
		provenance = provenance.ConfirmApproved()
	}
	return provenance.toAdmissionEvidence()
}

// AdmissionEvidenceForRemoteApply builds admission evidence for the remote Core
// apply path. The caller loads the owner-verified deployment record for the
// resolved deployment identity; a missing record (a fresh apply that has not
// yet bound a pending record), an unbound identity or an unverifiable source
// yields nil so a restrictive Target fails closed. requireApproval binds the
// operator-approval flag the policy requires. This is the single provenance
// source for the remote choke point and never trusts client metadata.
func AdmissionEvidenceForRemoteApply(sourceAvailable bool, record *DeploymentRecord, applicationID, expectedApplicationID string, requireApproval bool) *DeploymentAdmissionEvidence {
	if record == nil {
		return nil
	}
	return AdmissionEvidenceForApplyingMutation(
		sourceAvailable, record.Source,
		applicationID, expectedApplicationID,
		requireApproval,
	)
}
