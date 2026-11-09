package deployment

import (
	"strings"
	"testing"
)

func TestDeploymentAdmissionLegacyAndDevelopmentWithoutProvenance(t *testing.T) {
	if err := EvaluateDeploymentAdmission(nil, nil); err != nil {
		t.Fatalf("legacy Target unexpectedly restricted: %v", err)
	}
	development, err := DeploymentPolicyPreset("development")
	if err != nil {
		t.Fatal(err)
	}
	if err := EvaluateDeploymentAdmission(&development, nil); err != nil {
		t.Fatalf("development must allow uncommitted local work: %v", err)
	}
}

func TestDeploymentAdmissionConfiguredRestrictionsFailClosed(t *testing.T) {
	policy, err := DeploymentPolicyPreset("production")
	if err != nil {
		t.Fatal(err)
	}
	if err := EvaluateDeploymentAdmission(&policy, nil); err == nil {
		t.Fatal("protected Target accepted absent Core provenance")
	}
	unverified := &DeploymentAdmissionEvidence{
		branch: "main", commitVerified: true, remoteCommitVerified: true,
		imageDigestVerified: true, approvalVerified: true,
	}
	if err := EvaluateDeploymentAdmission(&policy, unverified); err == nil {
		t.Fatal("untrusted client-like flags accepted")
	}
	valid := *unverified
	valid.sourceVerified = true
	if err := EvaluateDeploymentAdmission(&policy, &valid); err != nil {
		t.Fatalf("verified clean production admission denied: %v", err)
	}
	cases := []struct {
		name string
		edit func(*DeploymentAdmissionEvidence)
		want string
	}{
		{"dirty", func(p *DeploymentAdmissionEvidence) { p.dirty = true }, "dirty"},
		{"branch", func(p *DeploymentAdmissionEvidence) { p.branch = "feature/unsafe" }, "branch"},
		{"commit", func(p *DeploymentAdmissionEvidence) { p.commitVerified = false }, "commit"},
		{"remote", func(p *DeploymentAdmissionEvidence) { p.remoteCommitVerified = false }, "remote commit"},
		{"digest", func(p *DeploymentAdmissionEvidence) { p.imageDigestVerified = false }, "digest"},
		{"approval", func(p *DeploymentAdmissionEvidence) { p.approvalVerified = false }, "approval"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			proof := valid
			tc.edit(&proof)
			if err := EvaluateDeploymentAdmission(&policy, &proof); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %s rejection, got %v", tc.want, err)
			}
		})
	}
}

func TestDeploymentAdmissionOperatorOverridesDoNotInheritPresetRestrictions(t *testing.T) {
	policy, err := DeploymentPolicyPreset("production")
	if err != nil {
		t.Fatal(err)
	}
	policy.AllowedBranches = nil
	policy.AllowDirty = true
	policy.RequireCommit = false
	policy.RequireRemoteCommit = false
	policy.RequireApproval = false
	policy.RequireImageDigest = false
	if err := EvaluateDeploymentAdmission(&policy, nil); err != nil {
		t.Fatalf("explicitly unrestricted production Target rejected: %v", err)
	}
	// A custom policy with AllowDirty=false is still restrictive even without
	// a named preset. The Target name never grants implicit privileges.
	custom := TargetDeploymentPolicy{Preset: "custom"}
	if err := EvaluateDeploymentAdmission(&custom, nil); err == nil {
		t.Fatal("custom clean-tree restriction accepted unverified input")
	}
}
