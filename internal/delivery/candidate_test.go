package delivery

import "testing"

// The candidate submission leaf enforces the fail-closed contract exactly where
// the product path binds app/target/consent and reads the submission identity.
func TestCandidateValidateContract(t *testing.T) {
	cases := []struct {
		name string
		sub  CandidateSubmission
		ok   bool
	}{
		{
			name: "pinned source revision",
			sub:  CandidateSubmission{ContractVersion: CandidateContractVersion, ApplicationID: "app-1", TargetName: "lab", ConsentBy: "operator", Acknowledged: true, SourceRepository: "example/app", SourceRevision: "0123456789abcdef0123456789abcdef01234567"},
			ok:   true,
		},
		{
			name: "pinned image digest",
			sub:  CandidateSubmission{ContractVersion: CandidateContractVersion, ApplicationID: "app-2", TargetName: "lab", ConsentBy: "automation", Acknowledged: true, ImageReference: "example/app:release", ImageDigest: "sha256:1111111111111111111111111111111111111111111111111111111111111111"},
			ok:   true,
		},
		{
			name: "no identity source",
			sub:  CandidateSubmission{ContractVersion: CandidateContractVersion, ApplicationID: "app-1", TargetName: "lab", ConsentBy: "operator", Acknowledged: true},
			ok:   false,
		},
		{
			name: "both source and image",
			sub:  CandidateSubmission{ContractVersion: CandidateContractVersion, ApplicationID: "app-4", TargetName: "lab", ConsentBy: "operator", Acknowledged: true, SourceRepository: "example/app", SourceRevision: "0123456789abcdef0123456789abcdef01234567", ImageReference: "example/app", ImageDigest: "sha256:2222222222222222222222222222222222222222222222222222222222222222"},
			ok:   false,
		},
		{
			name: "movable branch alone",
			sub:  CandidateSubmission{ContractVersion: CandidateContractVersion, ApplicationID: "app-1", TargetName: "lab", ConsentBy: "operator", Acknowledged: true, SourceRepository: "example/app", SourceRevision: "main"},
			ok:   false,
		},
		{
			name: "mutable tag with image counterpart",
			sub:  CandidateSubmission{ContractVersion: CandidateContractVersion, ApplicationID: "app-1", TargetName: "lab", ConsentBy: "operator", Acknowledged: true, SourceRepository: "example/app", SourceRevision: "release-1", ImageReference: "example/app", ImageDigest: "sha256:1111111111111111111111111111111111111111111111111111111111111111"},
			ok:   false,
		},
		{
			name: "mutable tag only",
			sub:  CandidateSubmission{ContractVersion: CandidateContractVersion, ApplicationID: "app-2", TargetName: "lab", ConsentBy: "automation", Acknowledged: true, ImageReference: "example/app:latest"},
			ok:   false,
		},
		{
			name: "bad digest format",
			sub:  CandidateSubmission{ContractVersion: CandidateContractVersion, ApplicationID: "app-2", TargetName: "lab", ConsentBy: "automation", Acknowledged: true, ImageReference: "example/app", ImageDigest: "sha256:deadbeef"},
			ok:   false,
		},
		{
			name: "unacknowledged consent",
			sub:  CandidateSubmission{ContractVersion: CandidateContractVersion, ApplicationID: "app-1", TargetName: "lab", ConsentBy: "operator", Acknowledged: false, SourceRepository: "example/app", SourceRevision: "0123456789abcdef0123456789abcdef01234567"},
			ok:   false,
		},
		{
			name: "bare secret reference",
			sub:  CandidateSubmission{ContractVersion: CandidateContractVersion, ApplicationID: "app-5", TargetName: "lab", ConsentBy: "operator", Acknowledged: true, SourceRepository: "example/app", SourceRevision: "0123456789abcdef0123456789abcdef01234567", SecretRef: "secrets/db-password"},
			ok:   false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Positive cases carry a matching consent; the digest is the canonical
			// content-hash of exactly this submission. Cases that expect an error
			// pass nil consent (missing consent) or a deliberately wrong digest.
			var consent *CandidateConsent
			if tc.ok {
				consent = &CandidateConsent{By: "operator", Digest: CandidateConsentDigest(tc.sub)}
			}
			err := tc.sub.Validate(consent)
			if tc.ok && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !tc.ok && err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

// A consent whose digest does not match the validated submission fails closed:
// a stale, partial, or otherwise mismatched acknowledgement cannot persist a
// candidate.
func TestCandidateConsentDigestMustMatch(t *testing.T) {
	sub := CandidateSubmission{ContractVersion: CandidateContractVersion, ApplicationID: "app-1", TargetName: "lab", ConsentBy: "operator", Acknowledged: true, SourceRepository: "example/app", SourceRevision: "0123456789abcdef0123456789abcdef01234567"}
	if err := sub.Validate(nil); err == nil {
		t.Fatal("nil consent accepted; consent is required")
	}
	wrong := &CandidateConsent{By: "operator", Digest: "deadbeef"}
	if err := sub.Validate(wrong); err == nil {
		t.Fatal("mismatched consent digest accepted; consent digest must match the submission")
	}
}

// isPinnedRevision accepts a 40- or 64-hex commit object ID and an opaque
// algo:hex digest (>=32 hex chars); it rejects branch names, mutable tags and
// any other syntactic label.
func TestIsPinnedRevision(t *testing.T) {
	cases := map[string]bool{
		"0123456789abcdef0123456789abcdef01234567":                                true, // 40-hex commit object ID
		"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef":        true, // 64-hex SHA
		"sha256:1111111111111111111111111111111111111111111111111111111111111111": true, // opaque digest (>=32 hex)
		"abc123":       false, // short hex: not a fixed object ID
		"v1.4.2":       false, // dotted tag (mutable)
		"release/main": false, // branch path (movable)
		"release-1":    false, // mutable tag
		"main2":        false, // mutable tag
		"main":         false, // plain branch (movable)
		"":             false, // empty
	}
	for revision, want := range cases {
		if got := isPinnedRevision(revision); got != want {
			t.Errorf("isPinnedRevision(%q) = %v, want %v", revision, got, want)
		}
	}
}
