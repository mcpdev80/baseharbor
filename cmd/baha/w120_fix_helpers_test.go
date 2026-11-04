package main

import (
	"context"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/delivery"
)

// confirmDigestForResolved returns the canonical confirm digest the real apply
// choke point binds for resolved, computed exactly as finalizeCandidateConsent
// does (the canonical content-hash of the validated CandidateSubmission).
func confirmDigestForResolved(t *testing.T, resolved resolvedApplication) string {
	t.Helper()
	revision := ""
	if resolved.DeploymentRecord != nil {
		revision = resolved.DeploymentRecord.Source.Digest
	}
	if fingerprint, ferr := repositoryDesiredStateFingerprint(context.Background(), resolved); ferr == nil && fingerprint != "" {
		revision = fingerprint
	}
	submission := delivery.CandidateSubmission{
		ContractVersion:  delivery.CandidateContractVersion,
		ApplicationID:    resolved.DeploymentIdentity.ApplicationID,
		TargetName:       resolved.DeploymentIdentity.Target,
		SourceRepository: resolved.repositoryRoot(),
		SourceRevision:   revision,
		ImageDigest:      "",
	}
	digest := delivery.CandidateConsentDigest(submission)
	if digest == "" {
		t.Fatal("computed confirm digest is empty")
	}
	return digest
}
