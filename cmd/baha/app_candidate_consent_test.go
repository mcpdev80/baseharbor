package main

import (
	"context"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/delivery"
	"github.com/mcpdev80/baseharbor/internal/deployment"
)

// repoCandidateFixture builds a repository-owned application on disk and
// resolves it through the real apply lifecycle, returning the resolved
// application.
func repoCandidateFixture(t *testing.T) resolvedApplication {
	t.Helper()
	configureTestTarget(t)
	t.Chdir(httpsAdoptionFixture(t, false))
	m := application.WithWorkloadComponents(application.New("demo", "dev", false, false, false), "demo-app")
	mustWriteInitializedAuthoredManifest(t, application.RepositoryManifestName, m)
	r, err := resolveApplication(context.Background(), application.DefaultStore(), nil, "apply")
	if err != nil {
		t.Fatal(err)
	}
	// Record an applied deployment so the repository source (repository root +
	// fingerprint digest) is populated exactly as the real apply choke point
	// reads it. The applied apply path then binds this identity on confirm.
	if err := recordAppliedDeployment(context.Background(), r, application.RuntimeFiles{}); err != nil {
		t.Fatal(err)
	}
	if err := application.ReconcileReferenceProviderRegistryAt(r.TargetStateRoot, r.Manifest); err != nil {
		t.Fatal(err)
	}
	record, err := deployment.LoadDeploymentRecord(r.DeploymentIdentity)
	if err != nil {
		t.Fatal(err)
	}
	r.DeploymentRecord = &record
	return r
}

// candidateSubmissionForResolved mirrors the exact apply-path choke point
// (cmd/baha/deployment_record.go): it builds the CandidateSubmission from the
// resolved record's repository source and fingerprints the source the same way
// recordAppliedDeployment does, so the computed digest is the digest the real
// write will bind.
func candidateSubmissionForResolved(t *testing.T, r resolvedApplication) delivery.CandidateSubmission {
	t.Helper()
	fingerprint, err := repositoryDesiredStateFingerprint(context.Background(), r)
	if err != nil || fingerprint == "" {
		t.Fatalf("source fingerprint: %v", err)
	}
	submission := delivery.CandidateSubmission{
		ContractVersion:  delivery.CandidateContractVersion,
		ApplicationID:    r.DeploymentRecord.Identity.ApplicationID,
		TargetName:       r.DeploymentRecord.Identity.Target,
		SourceRepository: r.DeploymentRecord.Source.Repository,
		SourceRevision:   fingerprint,
		ImageDigest:      "",
	}
	return submission
}

// TestConsentRejectsWrongDigestWriteNothing is a real apply-path test of Punkt 1:
// when the operator confirm digest does not match the canonical content-hash of
// the validated submission, the candidate choke point rejects it and persists NO
// deployment state, and the persisted digest (when a correct confirm is used) is
// exactly the digest finalizeCandidateConsent computed for the submission.
func TestConsentRejectsWrongDigestWriteNothing(t *testing.T) {
	r := repoCandidateFixture(t)
	submission := candidateSubmissionForResolved(t, r)
	want := delivery.CandidateConsentDigest(submission)
	if want == "" {
		t.Fatal("computed candidate digest is empty")
	}

	// A wrong confirm digest must fail closed and write nothing.
	wrong := "0000000000000000000000000000000000000000000000000000000000000000"
	ctxWrong := withCandidateConsentDigest(context.Background(), wrong)
	if _, err := recordDeploymentBeforeMutation(ctxWrong, r, "applying", wrong); err == nil {
		t.Fatal("wrong confirm digest accepted; candidate must fail closed")
	}
	// The applying mutation must have persisted NO consent: the candidate is
	// only bound on a matching digest, and a wrong digest writes nothing new.
	record, err := deployment.LoadDeploymentRecord(r.DeploymentIdentity)
	if err != nil {
		t.Fatal(err)
	}
	if record.Consent != nil {
		t.Fatalf("wrong digest persisted a consent: %+v", record.Consent)
	}

	// A correct confirm digest must succeed and bind exactly want.
	ctxOK := withCandidateConsentDigest(context.Background(), want)
	record2, err2 := recordDeploymentBeforeMutation(ctxOK, r, "applying", want)
	if err2 != nil {
		t.Fatalf("correct digest rejected: %v", err2)
	}
	if record2.Consent == nil {
		t.Fatal("correct digest produced no consent")
	}
	if record2.Consent.Digest != want {
		t.Fatalf("bound digest %q != computed %q: digest and binding diverged", record2.Consent.Digest, want)
	}
}

// TestPromptDigestEqualsBindDigest is a real apply-path test of Punkt 3: the
// digest the confirmation prompt shows for a submission is exactly the digest
// finalizeCandidateConsent binds for that same submission, derived from one
// shared helper with the same submission.
func TestPromptDigestEqualsBindDigest(t *testing.T) {
	r := repoCandidateFixture(t)
	submission := candidateSubmissionForResolved(t, r)
	bound := delivery.CandidateConsentDigest(submission)

	var prompt strings.Builder
	promptConfirmCandidateSubmission(r, &prompt)
	if !strings.Contains(prompt.String(), bound) {
		t.Fatalf("prompt does not contain the bound digest %q; prompt=%q", bound, prompt.String())
	}

	// The write binds the same digest that was shown.
	ctxOK := withCandidateConsentDigest(context.Background(), bound)
	record, err := recordDeploymentBeforeMutation(ctxOK, r, "applying", bound)
	if err != nil {
		t.Fatalf("correct digest rejected: %v", err)
	}
	if record.Consent.Digest != bound {
		t.Fatalf("persisted digest %q != shown digest %q", record.Consent.Digest, bound)
	}
}
