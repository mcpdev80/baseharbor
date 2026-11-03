package delivery

import (
	"fmt"
	"regexp"
	"strings"
)

// CandidateContractVersion is the exact candidate submission contract version.
// It mirrors the schema const baseharbor.candidate/v1.
const CandidateContractVersion = "baseharbor.candidate/v1"

// CandidateSource is the immutable identity kind of a candidate submission.
type CandidateSource string

const (
	// CandidateIdentitySource pins an exact Git source revision.
	CandidateIdentitySource CandidateSource = "source"
	// CandidateIdentityImage pins an exact OCI image digest.
	CandidateIdentityImage CandidateSource = "image"
)

var sha256Digest = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// CandidateSubmission is the exact source/image submission identity bound to an
// application and a target through explicit consent. It is declarative intent
// only; it never builds, promotes or migrates anything.
type CandidateSubmission struct {
	ContractVersion string `json:"contract_version"`
	ApplicationID   string `json:"app.id"`
	TargetName      string `json:"target.name"`
	ConsentBy       string `json:"consent.by"`
	// Acknowledged MUST be true: a fixed submission is applied only on explicit
	// operator/automation acknowledgement.
	Acknowledged bool `json:"consent.acknowledged"`
	// SourceRevision pins a fixed Git repository+revision (branch, tag or
	// commit SHA). A loose branch-string alone is never a fixed identity.
	SourceRepository string `json:"source.repository,omitempty"`
	SourceRevision   string `json:"source.revision,omitempty"`
	// ImageReference/ImageDigest pin an exact OCI image digest.
	ImageReference string `json:"image.reference,omitempty"`
	ImageDigest    string `json:"image.digest,omitempty"`
	// SecretRef must be empty: a bare secret reference never enters identity.
	SecretRef string `json:"source.secretRef,omitempty"`
}

// CandidateIdentity reports which immutable source a submission carries. It is
// empty when neither, and ambiguous when both are present. A source identity is
// only reported when the revision is itself fixed (see isPinnedRevision); a
// mutable branch/tag never contributes an identity — including when paired with
// a valid image digest, because the immutable identity must be content-addressed
// and a movable source is never acceptable.
func (c CandidateSubmission) CandidateIdentity() CandidateSource {
	hasImage := c.ImageDigest != ""
	hasSource := c.SourceRepository != "" && c.SourceRevision != ""
	// A mutable source revision is itself a contract violation; it must not be
	// silently absorbed by the image-digest path.
	if hasSource && !isPinnedRevision(c.SourceRevision) {
		return ""
	}
	if hasImage && hasSource {
		// A fixed source AND an image present: ambiguous, reject.
		return ""
	}
	switch {
	case hasImage:
		return CandidateIdentityImage
	default:
		return CandidateIdentitySource
	}
}

// Validate enforces the fail-closed candidate submission contract:
//
//   - exactly one immutable identity source (source revision OR image digest),
//     never both, never neither;
//   - consent.acknowledged MUST be true;
//   - a source revision must be a fixed identity (a 40/64-hex Git commit object
//     ID or an opaque algo:hex digest of >=32 hex chars); a mutable branch or
//     tag is never accepted, even when paired with an image digest;
//   - an image must carry a sha256 digest, never a mutable tag alone;
//   - no bare secret reference may enter the submission.
func (c CandidateSubmission) Validate() error {
	c = c.normalize()
	if c.ContractVersion != CandidateContractVersion {
		return fmt.Errorf("candidate contract version must be %q", CandidateContractVersion)
	}
	if strings.TrimSpace(c.ApplicationID) == "" {
		return fmt.Errorf("candidate submission requires app.id")
	}
	if strings.TrimSpace(c.TargetName) == "" {
		return fmt.Errorf("candidate submission requires target.name")
	}
	if strings.TrimSpace(c.ConsentBy) == "" {
		return fmt.Errorf("candidate submission requires consent.by")
	}
	if !c.Acknowledged {
		return fmt.Errorf("candidate submission requires consent.acknowledged to be true")
	}
	if strings.TrimSpace(c.SecretRef) != "" {
		return fmt.Errorf("candidate submission must not carry a bare secret reference")
	}
	identity := c.CandidateIdentity()
	switch identity {
	case CandidateIdentitySource:
		if err := c.validatePinnedSource(); err != nil {
			return err
		}
	case CandidateIdentityImage:
		if err := c.validatePinnedImage(); err != nil {
			return err
		}
	default:
		return fmt.Errorf("candidate submission must carry exactly one immutable identity (source revision OR image digest)")
	}
	return nil
}

// validatePinnedSource accepts only a fixed source revision. A mutable branch or
// tag — even when an image digest counterpart is present — is never accepted:
// the immutable identity is content-addressed, never a syntactic label.
func (c CandidateSubmission) validatePinnedSource() error {
	if isPinnedRevision(c.SourceRevision) {
		return nil
	}
	return fmt.Errorf("source revision %q must be a fixed identity (40/64-hex commit object ID or algo:hex digest of >=32 hex chars), not a mutable branch or tag", c.SourceRevision)
}

// validatePinnedImage requires a sha256 digest; a mutable tag alone is rejected.
func (c CandidateSubmission) validatePinnedImage() error {
	if !sha256Digest.MatchString(c.ImageDigest) {
		return fmt.Errorf("image must carry a sha256 digest, never a mutable tag alone")
	}
	return nil
}

// digestPattern matches an opaque immutable digest of the form algo:hex, where
// the hex part is at least 32 characters. It is the second immutable identity
// kind alongside a fixed Git commit SHA.
var digestPattern = regexp.MustCompile(`^[a-z0-9]+:[0-9a-f]{32,}$`)

// hexCommitPattern matches a Git commit object ID: 40 or 64 lowercase hex
// characters.
var hexCommitPattern = regexp.MustCompile(`^[0-9a-f]{40}$|^[0-9a-f]{64}$`)

// isPinnedRevision reports whether a revision is a fixed identity. Only two
// syntactically immutable forms are accepted:
//
//   - a Git commit object ID: a 40- or 64-character lowercase hex string (the
//     SHA-1 of a commit), or
//   - an opaque digest in the form algo:hex with a hex part of at least 32
//     characters (e.g. an OCI sha256 digest).
//
// Branch names, mutable tags (release-1, main2, moving refs) and any other
// human-readable version string are never a fixed identity; the immutable
// identity is proven by content addressing, never by a syntactic label.
func isPinnedRevision(revision string) bool {
	trimmed := strings.TrimSpace(revision)
	if trimmed == "" {
		return false
	}
	// A Git commit object ID: 40 or 64 lowercase hex characters.
	if hexCommitPattern.MatchString(strings.ToLower(trimmed)) {
		return true
	}
	// An opaque digest algo:hex (hex part >= 32 chars).
	if digestPattern.MatchString(trimmed) {
		return true
	}
	// Everything else (branch names, mutable tags, moving refs, human-readable
	// version strings) is movable and rejected.
	return false
}

func (c CandidateSubmission) normalize() CandidateSubmission {
	out := c
	out.ContractVersion = strings.TrimSpace(out.ContractVersion)
	out.ApplicationID = strings.TrimSpace(out.ApplicationID)
	out.TargetName = strings.TrimSpace(out.TargetName)
	out.ConsentBy = strings.TrimSpace(out.ConsentBy)
	out.SourceRepository = strings.TrimSpace(out.SourceRepository)
	out.SourceRevision = strings.TrimSpace(out.SourceRevision)
	out.ImageReference = strings.TrimSpace(out.ImageReference)
	out.ImageDigest = strings.TrimSpace(out.ImageDigest)
	out.SecretRef = strings.TrimSpace(out.SecretRef)
	return out
}
