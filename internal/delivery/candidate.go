package delivery

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
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
	// Consent is the acknowledged identity bound to this submission. It is
	// required: a candidate is applied only on an explicit operator/automation
	// acknowledgement whose digest pins the exact validated submission. The
	// published schema carries the identity through consent (by/at/digest);
	// a nil consent never validates.
	Consent *CandidateConsent `json:"-"`
}

// CandidateConsent is the acknowledged identity bound to a candidate submission.
// It carries the who (operator/automation principal) and the when, and its
// Digest pins the exact validated submission it acknowledges: the canonical
// content-hash of the submission the candidate leaf accepted. A candidate may
// only be applied when the persisted consent digest matches the digest of the
// submission the leaf validated, so a submission can never be acknowledged by a
// stale, partial, or otherwise mismatched consent.
type CandidateConsent struct {
	By     string    `json:"by"`
	At     time.Time `json:"at"`
	Digest string    `json:"digest"`
}

// CandidateConsentDigest returns the canonical content-hash of a candidate
// submission: the lowercase hex sha256 of its canonical JSON encoding. The
// encoding pins every contract field in declaration order through json.Marshal
// over the typed struct, so the digest binds the exact submission contents and
// no incidental key ordering. It reuses the established delivery-paket digest
// convention (sha256 over a canonical json.Marshal), not a new hash scheme.
//
// Both the confirmation prompt and the persisted consent MUST derive the bound
// digest from this single helper with the exact same submission, so the digest
// an operator sees can never differ from the digest the write binds.
func CandidateConsentDigest(c CandidateSubmission) string {
	raw, err := json.Marshal(c)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// sourcePairValid reports the schema half-pair binding for a source identity:
// SourceRevision is only meaningful together with SourceRepository. A revision
// without its repository, or a repository without its revision, is neither an
// identity nor a partial field — it is a structural contract violation shared
// with the published schema.
func (c CandidateSubmission) sourcePairValid() bool {
	if strings.TrimSpace(c.SourceRepository) == "" || strings.TrimSpace(c.SourceRevision) == "" {
		return false
	}
	// A fixed source still requires a pinned revision (see validatePinnedSource).
	return isPinnedRevision(c.SourceRevision)
}

// imagePairValid reports the schema half-pair binding for an image identity:
// ImageDigest is only meaningful together with ImageReference. A digest without
// its reference, or a reference without its digest, is neither an identity nor
// a partial field — it is a structural contract violation shared with the
// published schema.
func (c CandidateSubmission) imagePairValid() bool {
	if strings.TrimSpace(c.ImageReference) == "" || strings.TrimSpace(c.ImageDigest) == "" {
		return false
	}
	return sha256Digest.MatchString(c.ImageDigest)
}

// CandidateIdentity reports which immutable source a submission carries. It is
// empty when neither, and ambiguous when both are present. A source identity is
// only reported when the revision is itself fixed (see isPinnedRevision); a
// mutable branch/tag never contributes an identity — including when paired with
// a valid image digest, because the immutable identity must be content-addressed
// and a movable source is never acceptable.
//
// The identity is reported only when the corresponding schema half-pair is
// complete and valid: a source identity additionally requires SourceRevision
// together with SourceRepository (see sourcePairValid), an image identity
// additionally requires ImageDigest together with ImageReference (see
// imagePairValid). An incomplete pair is never reported as an identity — it is
// a structural contract violation, not a partial field.
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
		// A digest without its reference is not an image identity.
		if !c.imagePairValid() {
			return ""
		}
		return CandidateIdentityImage
	case hasSource:
		// A revision without its repository is not a source identity.
		if !c.sourcePairValid() {
			return ""
		}
		return CandidateIdentitySource
	default:
		return ""
	}
}

// Validate enforces the fail-closed candidate submission contract. Consent is
// required and content-pinned: a candidate is applied only on an explicit
// operator/automation acknowledgement whose digest matches the exact validated
// submission.
//
//		consent: the acknowledged identity bound to this submission; must be
//		    non-nil and its Digest must equal the canonical content-hash of the
//		    submission the leaf validated (a stale, partial, or otherwise mismatched
//		    consent is rejected). A nil consent never validates.
//
//	  - exactly one immutable identity source (source revision OR image digest),
//	    never both, never neither;
//	  - a source revision must be a fixed identity (a 40/64-hex Git commit object
//	    ID or an opaque algo:hex digest of >=32 hex chars); a mutable branch or
//	    tag is never accepted, even when paired with an image digest;
//	  - a source revision must be bound to its repository (SourceRevision only
//	    valid with SourceRepository), an image digest must be bound to its
//	    reference (ImageDigest only valid with ImageReference) — an incomplete
//	    schema half-pair is a structural contract violation;
//	  - an image must carry a sha256 digest, never a mutable tag alone;
//	  - no bare secret reference may enter the submission.
func (c CandidateSubmission) Validate(consent *CandidateConsent) error {
	if consent == nil || strings.TrimSpace(consent.By) == "" || consent.Digest == "" {
		return fmt.Errorf("candidate submission requires an explicit operator/automation consent with a matching digest")
	}
	expectedDigest := CandidateConsentDigest(c)
	if !constantTimeEqual(consent.Digest, expectedDigest) {
		return fmt.Errorf("candidate consent digest does not match the validated submission (expected %s)", expectedDigest)
	}
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
	if strings.TrimSpace(c.SecretRef) != "" {
		return fmt.Errorf("candidate submission must not carry a bare secret reference")
	}
	// Pair binding: a source revision only counts together with its repository
	// and an image digest only counts together with its reference. An incomplete
	// schema half-pair is a structural contract violation, independent of the
	// identity the candidate reports — both shared vectors must fail here.
	if (c.SourceRevision != "" || c.SourceRepository != "") && !c.sourcePairValid() {
		return fmt.Errorf("source revision requires its source repository (source.revision only valid with source.repository)")
	}
	if (c.ImageDigest != "" || c.ImageReference != "") && !c.imagePairValid() {
		return fmt.Errorf("image digest requires its image reference (image.digest only valid with image.reference)")
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

// constantTimeEqual compares two digests in constant time to avoid leaking
// timing information about the expected submission digest. An empty expected
// digest never matches (the caller already rejects empty consent digests).
func constantTimeEqual(got, expected string) bool {
	if expected == "" || got == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(expected)) == 1
}

// ConstantTimeDigestMatch reports whether the operator-supplied confirm digest
// equals the digest the candidate leaf validated, in constant time so the
// comparison never leaks the expected digest through timing. An empty value
// never matches, so a missing or whitespace-only confirm can never bind.
func ConstantTimeDigestMatch(got, expected string) bool {
	return constantTimeEqual(got, expected)
}

// validatePinnedSource accepts only a fixed source revision. A mutable branch or
// tag — even when an image digest counterpart is present — is never accepted:
// the immutable identity is content-addressed, never a syntactic label. A source
// revision is only valid when bound to its owning source repository: the
// published schema's source pair (kind, repository, revision) is validated as a
// unit, so a revision without its repository can never carry an identity.
func (c CandidateSubmission) validatePinnedSource() error {
	if strings.TrimSpace(c.SourceRepository) == "" {
		return fmt.Errorf("source revision requires source.repository to be set; a source revision must be bound to its owning repository")
	}
	if isPinnedRevision(c.SourceRevision) {
		return nil
	}
	return fmt.Errorf("source revision %q must be a fixed identity (40/64-hex commit object ID or algo:hex digest of >=32 hex chars), not a mutable branch or tag", c.SourceRevision)
}

// validatePinnedImage requires a sha256 digest; a mutable tag alone is rejected.
// An image digest is only valid when bound to its owning image reference: the
// published schema's image pair (kind, reference, digest) is validated as a
// unit, so a digest without its reference can never carry an identity.
func (c CandidateSubmission) validatePinnedImage() error {
	if strings.TrimSpace(c.ImageReference) == "" {
		return fmt.Errorf("image digest requires image.reference to be set; an image digest must be bound to its owning reference")
	}
	if !sha256Digest.MatchString(c.ImageDigest) {
		return fmt.Errorf("image must carry a sha256 digest, never a mutable tag alone")
	}
	return nil
}

// digestPattern matches an opaque immutable digest of the form algo:hex, where
// the hex part is at least 32 characters. It is the second immutable identity
// kind alongside a fixed Git commit SHA.
var digestPattern = regexp.MustCompile(`^[a-z0-9]+:[0-9a-fA-F]{32,}$`)

// hexCommitPattern matches a Git commit object ID: a 40- or 64-character hex
// string. Hex digits are case-insensitive here and in the published schema
// (pattern [0-9a-fA-F]); Go accepts both upper- and lowercase and compares the
// identity canonically after lowercasing, so a fixed revision is accepted with
// the exact same result by the Go leaf and the schema.
var hexCommitPattern = regexp.MustCompile(`^[0-9a-fA-F]{40}$|^[0-9a-fA-F]{64}$`)

// isPinnedRevision reports whether a revision is a fixed identity. Only two
// syntactically immutable forms are accepted:
//
//   - a Git commit object ID: a 40- or 64-character hex string (the SHA-1 of a
//     commit), in either case, or
//   - an opaque digest in the form algo:hex with a hex part of at least 32
//     characters (e.g. an OCI sha256 digest), in either case.
//
// The Go leaf and the published schema share one case rule for hex: both accept
// [0-9a-fA-F] and treat the identity canonically (Go lowercases before the
// hex match). isPinnedRevision documents that equivalence so a revision that
// passes one validator cannot pass the other — the single source of truth for
// case parity is this helper plus the schema pattern, never a per-call lower().
//
// Branch names, mutable tags (release-1, main2, moving refs) and any other
// human-readable version string are never a fixed identity; the immutable
// identity is proven by content addressing, never by a syntactic label.
func isPinnedRevision(revision string) bool {
	trimmed := strings.TrimSpace(revision)
	if trimmed == "" {
		return false
	}
	// A Git commit object ID: 40 or 64 hex characters (either case, matched
	// canonically — the single source of truth shared with the schema).
	if hexCommitPattern.MatchString(strings.ToLower(trimmed)) {
		return true
	}
	// An opaque digest algo:hex (hex part >= 32 chars, either case).
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
