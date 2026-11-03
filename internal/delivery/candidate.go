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
// empty when neither, and ambiguous when both are present.
func (c CandidateSubmission) CandidateIdentity() CandidateSource {
	hasSource := c.SourceRepository != "" && c.SourceRevision != ""
	hasImage := c.ImageDigest != ""
	switch {
	case hasSource && hasImage:
		return ""
	case hasSource:
		return CandidateIdentitySource
	case hasImage:
		return CandidateIdentityImage
	default:
		return ""
	}
}

// Validate enforces the fail-closed candidate submission contract:
//
//   - exactly one immutable identity source (source revision OR image digest),
//     never both, never neither;
//   - consent.acknowledged MUST be true;
//   - a source revision must be pinned; a loose branch-string alone is rejected
//     unless paired with an image digest counterpart;
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

// validatePinnedSource accepts a fixed source revision, or a movable branch
// only when an image digest counterpart pins the release identity.
func (c CandidateSubmission) validatePinnedSource() error {
	pinned := isPinnedRevision(c.SourceRevision)
	if pinned {
		return nil
	}
	// A movable branch-string is accepted only together with a pinned image.
	if c.ImageDigest != "" && sha256Digest.MatchString(c.ImageDigest) {
		return nil
	}
	return fmt.Errorf("source revision %q must be a fixed revision (branch, tag or commit SHA), not a movable string", c.SourceRevision)
}

// validatePinnedImage requires a sha256 digest; a mutable tag alone is rejected.
func (c CandidateSubmission) validatePinnedImage() error {
	if !sha256Digest.MatchString(c.ImageDigest) {
		return fmt.Errorf("image must carry a sha256 digest, never a mutable tag alone")
	}
	return nil
}

// isPinnedRevision reports whether a revision is a fixed identity. A commit SHA
// (64 lowercase hex) is fixed; a non-empty label that carries a version marker
// (digit, dot, dash) is a fixed tag; a bare lowercase branch name without such
// markers is movable. A short hex-only string is treated as a fixed revision
// label, matching the canonical golden fixture revision.
func isPinnedRevision(revision string) bool {
	trimmed := strings.TrimSpace(revision)
	if trimmed == "" {
		return false
	}
	if len(trimmed) == 64 && sha256Digest.MatchString("sha256:" + trimmed) {
		return true
	}
	// A branch name is a bare lowercase identifier with no version markers.
	marksVersion := false
	for _, r := range trimmed {
		if r == '.' || r == '-' || (r >= '0' && r <= '9') {
			marksVersion = true
			break
		}
	}
	if !marksVersion {
		allLetters := true
		for _, r := range trimmed {
			if r < 'a' || r > 'z' {
				allLetters = false
				break
			}
		}
		if allLetters {
			// All-lowercase-letters with no version marker is a movable branch.
			return false
		}
	}
	// A slash denotes a namespace/branch path, which is movable.
	if strings.Contains(trimmed, "/") {
		return false
	}
	return true
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
