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
			sub:  CandidateSubmission{ContractVersion: CandidateContractVersion, ApplicationID: "app-1", TargetName: "lab", ConsentBy: "operator", Acknowledged: true, SourceRepository: "example/app", SourceRevision: "abc123def456"},
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
			sub:  CandidateSubmission{ContractVersion: CandidateContractVersion, ApplicationID: "app-4", TargetName: "lab", ConsentBy: "operator", Acknowledged: true, SourceRepository: "example/app", SourceRevision: "abc123", ImageReference: "example/app", ImageDigest: "sha256:2222222222222222222222222222222222222222222222222222222222222222"},
			ok:   false,
		},
		{
			name: "movable branch alone",
			sub:  CandidateSubmission{ContractVersion: CandidateContractVersion, ApplicationID: "app-1", TargetName: "lab", ConsentBy: "operator", Acknowledged: true, SourceRepository: "example/app", SourceRevision: "main"},
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
			sub:  CandidateSubmission{ContractVersion: CandidateContractVersion, ApplicationID: "app-1", TargetName: "lab", ConsentBy: "operator", Acknowledged: false, SourceRepository: "example/app", SourceRevision: "abc123def456"},
			ok:   false,
		},
		{
			name: "bare secret reference",
			sub:  CandidateSubmission{ContractVersion: CandidateContractVersion, ApplicationID: "app-5", TargetName: "lab", ConsentBy: "operator", Acknowledged: true, SourceRepository: "example/app", SourceRevision: "abc123", SecretRef: "secrets/db-password"},
			ok:   false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.sub.Validate()
			if tc.ok && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !tc.ok && err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

// isPinnedRevision accepts a 64-hex commit SHA and a dotted/numeric tag label;
// it rejects a plain lowercase branch name.
func TestIsPinnedRevision(t *testing.T) {
	cases := map[string]bool{
		"abc123def456":                                              true, // carries digits: fixed revision label (golden fixture)
		"abc123def456789012345678901234567890123456789012345678901234abcd": true, // 64-hex SHA
		"v1.4.2":                                                    true, // dotted tag
		"release/main":                                              false, // branch path (movable)
		"main":                                                      false, // plain branch (movable)
		"":                                                          false, // empty
	}
	for revision, want := range cases {
		if got := isPinnedRevision(revision); got != want {
			t.Errorf("isPinnedRevision(%q) = %v, want %v", revision, got, want)
		}
	}
}
