package main

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/delivery"
	"github.com/mcpdev80/baseharbor/internal/deployment"
)

// withConfirmCandidateDigestForResolved returns a context carrying the operator
// confirmation and the canonical confirm digest for the repository candidate
// bound to resolved, so the real apply choke point can record a validated ECHT
// consent in tests. It computes the digest exactly as the production consumer
// does (repository desired-state fingerprint over the resolved source), so it
// matches what recordDeploymentBeforeMutation binds. Non-candidate sources are
// unaffected.
func withConfirmCandidateDigestForResolved(ctx context.Context, resolved resolvedApplication) (context.Context, string) {
	record := resolved.DeploymentRecord
	if record == nil {
		if loaded, err := deployment.LoadDeploymentRecord(resolved.DeploymentIdentity); err == nil {
			record = &loaded
		}
	}
	if record == nil || record.Source.Kind != "repository" || !resolved.FromRepository {
		return ctx, ""
	}
	fingerprint, err := repositoryDesiredStateFingerprint(ctx, resolved)
	if err != nil || fingerprint == "" {
		return ctx, ""
	}
	submission := delivery.CandidateSubmission{
		ContractVersion:  delivery.CandidateContractVersion,
		ApplicationID:    record.Identity.ApplicationID,
		TargetName:       record.Identity.Target,
		SourceRepository: record.Source.Repository,
		SourceRevision:   fingerprint,
	}
	return withCandidateConsentDigest(ctx, delivery.CandidateConsentDigest(submission)), delivery.CandidateConsentDigest(submission)
}

// withConfirmCandidateForSetup returns a context carrying the confirm digest for
// the repository candidate that a baseline apply (identified by target and NAME)
// will bind, so the real apply lifecycle can record a validated ECHT consent. It
// resolves the application the same way the apply path does and pins the
// canonical confirm digest; when resolution fails or the source is not a
// repository candidate it returns the context unchanged.
func withConfirmCandidateForSetup(ctx context.Context, target deployment.ResolvedTarget, name, root string) context.Context {
	resolved, err := resolveApplication(ctx, application.DefaultStore(), []string{name}, "apply")
	if err != nil || !resolved.FromRepository {
		return ctx
	}
	fingerprint, err := repositoryDesiredStateFingerprint(ctx, resolved)
	if err != nil || fingerprint == "" {
		return ctx
	}
	submission := delivery.CandidateSubmission{
		ContractVersion:  delivery.CandidateContractVersion,
		ApplicationID:    resolved.DeploymentRecord.Identity.ApplicationID,
		TargetName:       resolved.DeploymentIdentity.Target,
		SourceRepository: resolved.DeploymentRecord.Source.Repository,
		SourceRevision:   fingerprint,
	}
	return withCandidateConsentDigest(ctx, delivery.CandidateConsentDigest(submission))
}

func TestInterruptedApplyRetainsOriginalIntentAcrossRetries(t *testing.T) {
	configureTestTarget(t)
	t.Chdir(httpsAdoptionFixture(t, false))
	m := application.WithWorkloadComponents(application.New("demo", "dev", false, false, false), "demo-app")
	m.Services.Identity = true
	mustWriteInitializedAuthoredManifest(t, application.RepositoryManifestName, m)
	resolved, err := resolveApplication(context.Background(), application.DefaultStore(), nil, "apply")
	if err != nil {
		t.Fatal(err)
	}
	if err := recordAppliedDeployment(context.Background(), resolved, application.RuntimeFiles{}); err != nil {
		t.Fatal(err)
	}
	original, err := deployment.LoadDeploymentRecord(resolved.DeploymentIdentity)
	if err != nil {
		t.Fatal(err)
	}
	// The resolver snapshot predates the last successful record. Mutation must
	// reload the authoritative record, not discard the prior Identity consumer.
	resolved.DeploymentRecord = nil
	resolved.Manifest.Services.Identity = false
	for attempt := 0; attempt < 2; attempt++ {
		confirmCtx, confirm := withConfirmCandidateDigestForResolved(context.Background(), resolved)
		pending, err := recordDeploymentBeforeMutation(confirmCtx, resolved, "applying", confirm)
		if err != nil {
			t.Fatal(err)
		}
		var prior application.Manifest
		if err := json.Unmarshal([]byte(pending.Applied.GeneratedState["previous_intent"]), &prior); err != nil || !reflect.DeepEqual(prior, m) {
			t.Fatalf("retry %d lost original intent: %#v %v", attempt, prior, err)
		}
		var desired application.Manifest
		if err := json.Unmarshal(pending.Applied.Intent, &desired); err != nil || desired.Services.Identity {
			t.Fatalf("retry %d lost desired intent: %#v %v", attempt, desired, err)
		}
	}
	if err := recordAppliedDeployment(context.Background(), resolved, application.RuntimeFiles{}); err != nil {
		t.Fatal(err)
	}
	verified, err := deployment.LoadDeploymentRecord(original.Identity)
	if err != nil || verified.Applied.GeneratedState["previous_intent"] != "" {
		t.Fatalf("verified convergence retained stale previous intent: %#v %v", verified, err)
	}
}
