package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/delivery"
	"github.com/mcpdev80/baseharbor/internal/deployment"
)

// candidateConsentOverrideKey carries the operator-provided candidate digest
// (--confirm-candidate) through the apply lifecycle. The zero value means no
// explicit consent was supplied; the repository apply path then fails closed
// before writing any deployment state.
type candidateConsentOverrideKey struct{}

// withCandidateConsentDigest stores a raw confirm digest on the context. An
// empty or whitespace-only digest is a no-op so callers can always pass the
// possibly-unparsed flag value.
func withCandidateConsentDigest(ctx context.Context, digest string) context.Context {
	if strings.TrimSpace(digest) == "" {
		return ctx
	}
	return context.WithValue(ctx, candidateConsentOverrideKey{}, strings.TrimSpace(digest))
}

// candidateConsentOverride returns the raw confirm digest from the context, or
// an empty string when none was supplied.
func candidateConsentOverride(ctx context.Context) string {
	value, _ := ctx.Value(candidateConsentOverrideKey{}).(string)
	return strings.TrimSpace(value)
}

// promptConfirmCandidateSubmission shows the exact candidate identity that will
// be bound to this deployment before any state is written. For a
// repository-owned candidate submission it prints the canonical content hash so
// the operator can confirm it and pass --confirm-candidate with that digest;
// the apply then fails closed without that flag. Non-candidate sources are
// unaffected.
//
// The displayed digest is derived from the SAME desired-state input the write
// binds: repositoryDesiredStateFingerprint(ctx, resolved) — the one helper both
// recordDeploymentBeforeMutation (the candidate choke point) and this prompt run
// over the same resolved application. The prompt never reads the persisted
// DeploymentRecord.Source.Digest, so an already-recorded pending record can no
// longer freeze the shown identity at an old digest: every re-run recomputes
// from the current source. A nil record still fingerprints the freshly resolved
// source, so a nil-record prompt shows the digest of the new submission, not
// an empty value.
func promptConfirmCandidateSubmission(ctx context.Context, resolved resolvedApplication, out io.Writer) {
	submission, current := candidateSubmissionForResolvedApp(ctx, resolved)
	if submission.SourceRevision == "" {
		return
	}
	// Derive the bound digest from the same helper finalizeCandidateConsent
	// uses for the same submission, so the digest shown here is exactly the
	// digest the write binds.
	digest := delivery.CandidateConsentDigest(submission)
	if digest == "" {
		return
	}
	fmt.Fprintf(out, "\nCandidate to bind for confirmation:\n")
	fmt.Fprintf(out, "  app.id      %s\n", submission.ApplicationID)
	fmt.Fprintf(out, "  target.name %s\n", submission.TargetName)
	fmt.Fprintf(out, "  source %s\n", current)
	fmt.Fprintf(out, "  confirm-hash %s\n", digest)
	fmt.Fprintf(out, "Re-run with --confirm-candidate %s to acknowledge this exact submission.\n", digest)
}

// candidateSubmissionForResolved builds the repository-owned CandidateSubmission
// for a resolved application the SAME way recordDeploymentBeforeMutation binds
// it: the source revision is repositoryDesiredStateFingerprint(ctx, resolved),
// not the persisted DeploymentRecord.Source.Digest. It returns the submission
// together with the exact revision string that is shown to the operator, so the
// prompt and the write fingerprint the same input over the same resolved
// application. A nil record is handled transparently — the fingerprint is still
// computed from the freshly resolved source, so a nil-record prompt shows the
// new submission's digest rather than an empty value.
func candidateSubmissionForResolvedApp(ctx context.Context, resolved resolvedApplication) (delivery.CandidateSubmission, string) {
	identity := resolved.DeploymentRecord
	if identity == nil {
		identity = &deployment.DeploymentRecord{
			Identity: resolved.DeploymentIdentity,
			Source:   deployment.DeploymentSource{Kind: "repository", Repository: resolved.repositoryRoot()},
		}
	}
	revision := identity.Source.Digest
	fingerprint, fingerprintErr := repositoryDesiredStateFingerprint(ctx, resolved)
	if fingerprintErr == nil && fingerprint != "" {
		// The write binds this fingerprint; the prompt must show the same digest,
		// even when the persisted record already carries a different (older) digest.
		revision = fingerprint
	}
	submission := delivery.CandidateSubmission{
		ContractVersion:  delivery.CandidateContractVersion,
		ApplicationID:    identity.Identity.ApplicationID,
		TargetName:       identity.Identity.Target,
		SourceRepository: identity.Source.Repository,
		SourceRevision:   revision,
		ImageDigest:      "",
	}
	return submission, revision
}

// finalizeCandidateConsent turns a non-empty operator confirm into an explicit
// operator/automation consent whose Digest is the canonical content-hash of the
// validated submission. It binds the computed digest (not the raw value the
// operator supplied), so the persisted consent always carries exactly the
// submission identity that was validated — a wrong or absent digest can never be
// persisted. An empty confirm returns nil, which the apply path rejects before
// persisting.
//
// The digest the operator confirmed is never trusted as-is: the helper computes
// the digest of the exact submission being bound and compares it against the
// supplied confirm in constant time. A mismatch is a contract violation and is
// rejected before any state is written, so a stale, partial, or forged confirm
// can never bind a submission. The prompt (promptConfirmCandidateSubmission)
// derives the displayed digest from this same helper with this same submission,
// so the shown digest always equals the bound digest.
func finalizeCandidateConsent(rawDigest, by string, submission delivery.CandidateSubmission) (*delivery.CandidateConsent, error) {
	if strings.TrimSpace(rawDigest) == "" {
		return nil, nil
	}
	if strings.TrimSpace(by) == "" {
		by = "operator"
	}
	// Single source of truth: the digest bound to the submission this leaf
	// validated, derived from the same helper the prompt prints.
	computed := delivery.CandidateConsentDigest(submission)
	if computed == "" {
		return nil, errConsentRequiredForCandidateSubmission
	}
	// Fail closed: the operator's confirm must match the submission identity
	// exactly (constant-time to avoid leaking the digest via timing).
	if !delivery.ConstantTimeDigestMatch(strings.TrimSpace(rawDigest), computed) {
		return nil, errCandidateConsentDigestMismatch
	}
	return &delivery.CandidateConsent{
		By:     by,
		At:     time.Now().UTC(),
		Digest: computed,
	}, nil
}
