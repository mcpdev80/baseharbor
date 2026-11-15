package deployment

import "testing"

func TestFromVerifiedCandidateRejectsUnverifiedAndUnbound(t *testing.T) {
	// Unverified source is never trusted, even with a plausible repository.
	if provenance, ok := FromVerifiedCandidate(false, DeploymentSource{
		Kind: "repository", Repository: "/core/checkout", Digest: "abc123",
	}, "app", "app"); ok {
		t.Fatalf("unverified source accepted: %+v", provenance)
	}
	// A repository source without a verified content digest is unverifiable.
	if provenance, ok := FromVerifiedCandidate(true, DeploymentSource{
		Kind: "repository", Repository: "/core/checkout",
	}, "app", "app"); ok {
		t.Fatalf("repository digest-less source accepted: %+v", provenance)
	}
	// An unbound application identity is rejected: a candidate must belong to the
	// deployment it claims to source.
	if provenance, ok := FromVerifiedCandidate(true, DeploymentSource{
		Kind: "repository", Repository: "/core/checkout", Digest: "abc123",
	}, "other-app", "app"); ok {
		t.Fatalf("foreign-application source accepted: %+v", provenance)
	}
	// Unknown source kinds never yield provenance.
	if provenance, ok := FromVerifiedCandidate(true, DeploymentSource{Kind: "unknown", Digest: "x"}, "app", "app"); ok {
		t.Fatalf("unknown source kind accepted: %+v", provenance)
	}
}

func TestFromVerifiedCandidateGitYieldsVerifiedCleanTree(t *testing.T) {
	provenance, ok := FromVerifiedCandidate(true, DeploymentSource{
		Kind: "repository", Repository: "/core/checkout", Digest: "deadbeef",
	}, "app", "app")
	if !ok {
		t.Fatal("verified repository source rejected")
	}
	if !provenance.SourceVerified || !provenance.CommitVerified || !provenance.RemoteCommitVerified || provenance.Dirty || !provenance.IsVerified() {
		t.Fatalf("git source missing verified flags: %+v", provenance)
	}
	if evidence := provenance.toAdmissionEvidence(); evidence == nil || !evidence.sourceVerified {
		t.Fatal("verified git source did not adapt to admission evidence")
	}
}

func TestFromVerifiedCandidateOCIYieldsDigestVerified(t *testing.T) {
	provenance, ok := FromVerifiedCandidate(true, DeploymentSource{
		Kind: "oci", Digest: "sha256:abc",
	}, "app", "app")
	if !ok {
		t.Fatal("verified oci source rejected")
	}
	if !provenance.SourceVerified || !provenance.ImageDigestVerified || provenance.RemoteCommitVerified {
		t.Fatalf("oci source wrong flags: %+v", provenance)
	}
}

func TestAdmissionEvidenceForRemoteApplyFailsClosedWithoutRecord(t *testing.T) {
	production, err := DeploymentPolicyPreset("production")
	if err != nil {
		t.Fatal(err)
	}
	// A nil record (fresh apply, no pending record bound yet) yields no
	// evidence, so a restrictive production Target fails closed.
	if evidence := AdmissionEvidenceForRemoteApply(true, nil, "app", "app", true); evidence != nil {
		t.Fatal("nil record produced evidence")
	}
	if err := EvaluateDeploymentAdmission(&production, AdmissionEvidenceForRemoteApply(true, nil, "app", "app", true)); err == nil {
		t.Fatal("production admitted without a verified record")
	}
}

func TestAdmissionEvidenceForRemoteApplyBindsApproval(t *testing.T) {
	production, err := DeploymentPolicyPreset("production")
	if err != nil {
		t.Fatal(err)
	}
	record := &DeploymentRecord{Source: DeploymentSource{Kind: "repository", Repository: "/core/checkout", Digest: "deadbeef"}}
	// Verified source provenance alone does not admit a production Target that
	// requires operator approval: the approval flag must be Core-verified.
	if err := EvaluateDeploymentAdmission(&production,
		AdmissionEvidenceForRemoteApply(true, record, "app", "app", true)); err == nil {
		t.Fatal("production admitted without operator approval")
	}
}
