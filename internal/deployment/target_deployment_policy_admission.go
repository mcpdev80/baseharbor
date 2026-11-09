package deployment

import (
	"errors"
	"fmt"
	"strings"
)

// DeploymentAdmissionEvidence is a Core-owned assertion. Its fields are private
// deliberately: a CLI/API/MCP JSON payload must never manufacture this proof.
// The verified candidate-ingress and authorization paths will populate it.
type DeploymentAdmissionEvidence struct {
	sourceVerified       bool
	branch               string
	dirty                bool
	commitVerified       bool
	remoteCommitVerified bool
	imageDigestVerified  bool
	approvalVerified     bool
}

// EvaluateDeploymentAdmission is fail-closed for configured restrictions when
// trusted Core evidence is unavailable. An absent policy preserves the legacy
// behavior; a development preset deliberately has no restrictive rules.
func EvaluateDeploymentAdmission(policy *TargetDeploymentPolicy, proof *DeploymentAdmissionEvidence) error {
	if policy == nil {
		return nil
	}
	if err := policy.Validate(); err != nil {
		return fmt.Errorf("invalid target deployment policy: %w", err)
	}
	requiresProof := !policy.AllowDirty || len(policy.AllowedBranches) != 0 ||
		policy.RequireCommit || policy.RequireRemoteCommit ||
		policy.RequireApproval || policy.RequireImageDigest
	if !requiresProof {
		return nil
	}
	if proof == nil || !proof.sourceVerified {
		return errors.New("target deployment policy requires Core-verified source provenance")
	}
	if !policy.AllowDirty && proof.dirty {
		return errors.New("target deployment policy denies dirty source")
	}
	if len(policy.AllowedBranches) != 0 {
		if strings.TrimSpace(proof.branch) == "" || !policy.AllowsBranch(proof.branch) {
			return fmt.Errorf("target deployment policy denies unverified or disallowed branch %q", proof.branch)
		}
	}
	if policy.RequireCommit && !proof.commitVerified {
		return errors.New("target deployment policy requires a verified source commit")
	}
	if policy.RequireRemoteCommit && !proof.remoteCommitVerified {
		return errors.New("target deployment policy requires a verified remote commit")
	}
	if policy.RequireImageDigest && !proof.imageDigestVerified {
		return errors.New("target deployment policy requires a verified OCI image digest")
	}
	if policy.RequireApproval && !proof.approvalVerified {
		return errors.New("target deployment policy requires Core-authorized approval")
	}
	return nil
}
