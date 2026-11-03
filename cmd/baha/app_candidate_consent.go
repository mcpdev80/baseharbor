package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/delivery"
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
func promptConfirmCandidateSubmission(resolved resolvedApplication, out io.Writer) {
	if resolved.DeploymentRecord == nil || resolved.DeploymentRecord.Source.Kind != "repository" {
		return
	}
	submission := delivery.CandidateSubmission{
		ContractVersion:  delivery.CandidateContractVersion,
		ApplicationID:    resolved.DeploymentRecord.Identity.ApplicationID,
		TargetName:       resolved.DeploymentRecord.Identity.Target,
		SourceRepository: resolved.DeploymentRecord.Source.Repository,
		SourceRevision:   resolved.DeploymentRecord.Source.Digest,
		ImageDigest:      "",
	}
	// Derive the bound digest from the same helper finalizeCandidateConsent
	// uses for the same submission, so the digest shown here is exactly the
	// digest the write binds.
	digest := delivery.CandidateConsentDigest(submission)
	if digest == "" {
		return
	}
	fmt.Fprintf(out, "\nCandidate to bind for confirmation:\n")
	fmt.Fprintf(out, "  app.id      %s\n", resolved.DeploymentRecord.Identity.ApplicationID)
	fmt.Fprintf(out, "  target.name %s\n", resolved.DeploymentRecord.Identity.Target)
	fmt.Fprintf(out, "  source %s\n", resolved.DeploymentRecord.Source.Digest)
	fmt.Fprintf(out, "  confirm-hash %s\n", digest)
	fmt.Fprintf(out, "Re-run with --confirm-candidate %s to acknowledge this exact submission.\n", digest)
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
