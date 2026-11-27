package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"regexp"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/delivery"
	"github.com/mcpdev80/baseharbor/internal/deployment"
)

var candidateConsentDigestPattern = regexp.MustCompile(`(^|[[:space:]])--confirm-candidate[[:space:]]+([0-9a-f]{64})([[:space:]]|$)`)

// candidateConsentDigestFromError returns only the exact digest supplied by
// the candidate-consent UsageError. Other errors never trigger an interactive
// retry from the New wizard.
func candidateConsentDigestFromError(err error) string {
	var usage *cli.UsageError
	if !errors.As(err, &usage) {
		return ""
	}
	match := candidateConsentDigestPattern.FindStringSubmatch(usage.Hint)
	if len(match) != 4 {
		return ""
	}
	return match[2]
}

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

func withRuntimeUpCandidateConsent(ctx context.Context, opts runtimeUpOptions) context.Context {
	return withCandidateConsentDigest(ctx, opts.CandidateConsent)
}

func clearCandidateConsentDigest(ctx context.Context) context.Context {
	return context.WithValue(ctx, candidateConsentOverrideKey{}, "")
}

// candidateConsentOverride returns the raw confirm digest from the context, or
// an empty string when none was supplied.
func candidateConsentOverride(ctx context.Context) string {
	value, _ := ctx.Value(candidateConsentOverrideKey{}).(string)
	return strings.TrimSpace(value)
}

// promptConfirmCandidateSubmission shows the exact candidate identity that will
// be bound to this deployment before any state is written. For a
// repository-owned candidate submission it prints the canonical content hash.
// On an interactive terminal the operator confirms that exact digest inline: an
// explicit "y" answer carries the consent straight into the write (the displayed
// digest IS the digest the choke point binds), so no second command run with
// --confirm-candidate is required. Any other input, and every non-interactive
// path, keeps the original contract unchanged: the apply fails closed and asks
// for a re-run with --confirm-candidate <digest>. Non-candidate sources are
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
//
// promptConfirmCandidateSubmission asks the operator to confirm the EXACT
// candidate before the write. It is fail-closed: without a real terminal stdin,
// with --no-input, or on a threaded reader that is not the terminal stream, it
// prints the re-run-with-flag contract and binds nothing. The bind answer is
// read from the same stream whose terminal identity the gate checked (see
// interactiveCandidatePromptAllowed), never from a different context reader.
func promptConfirmCandidateSubmission(ctx context.Context, resolved resolvedApplication, out io.Writer) {
	promptConfirmCandidateSubmissionInput(ctx, resolved, os.Stdin, out)
}

// promptConfirmCandidateSubmissionInput is the leaf of the bind prompt with an
// explicit reader: the local apply and the remote lifecycle thread their
// interactive stdin (applicationInput(ctx, runtimeInput)) so the question and
// the answer share one stream; every other caller keeps the historical default.
func promptConfirmCandidateSubmissionInput(ctx context.Context, resolved resolvedApplication, in io.Reader, out io.Writer) {
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
	// The interactive operator reviews the EXACT digest printed above and
	// confirms it here; the confirmation is then carried into the same
	// invocation's write. The prompt is reached only while the Target lifecycle
	// lock is held and only on an interactive terminal stdin, and the
	// lifecycle's own earlier prompts run before this point on the same stream,
	// so an explicit "y" on the next line is the operator's answer to THIS bind
	// question. The interactive reachability check (interactiveCandidateStdin)
	// is production-pinned to a terminal os.Stdin; non-os.File readers (piped
	// stdin, and the internal-input-resolver seam's bytes.Buffer, which
	// readerIsTerminal alone would treat as a terminal) never get the question.
	// Every other input — EOF, refuse, or no interactive terminal at all — binds
	// nothing and keeps the re-run-with-flag contract below.
	if interactiveCandidatePromptAllowed(ctx, in) {
		fmt.Fprintf(out, "Confirm this exact candidate to apply it now (y), or refuse to bind it and finish with --confirm-candidate %s instead.\n", digest)
		fmt.Fprintf(out, "Bind this exact candidate (confirm-hash above)? [y/N]: ")
		if answer, err := bufio.NewReader(in).ReadString('\n'); err == nil && isAffirmativeAnswer(answer) {
			bindCandidateConsentForSubmission(submission)
		}
		return
	}
	fmt.Fprintf(out, "Re-run with --confirm-candidate %s to acknowledge this exact submission.\n", digest)
}

// isAffirmativeAnswer is the promptYesNo vocabulary (cmd/baha/app_init_prompts.go):
// the empty answer keeps the default No, so only an explicit yes binds.
func isAffirmativeAnswer(line string) bool {
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes", "j", "ja":
		return true
	default:
		return false
	}
}

// bindCandidateConsentForSubmission is the single place the interactive bind
// prompt records the operator's confirmation of an EXACT candidate submission:
// the write boundary (recordDeploymentBeforeMutation) consumes this one-shot
// binding for the same deployment identity, re-derives the digest from the
// CURRENT source and constant-time-compares it, so a confirmation is only ever
// carried into the invocation that asked for it. An already-supplied
// --confirm-candidate flag always wins and is never replaced here.
func bindCandidateConsentForSubmission(submission delivery.CandidateSubmission) {
	digest := delivery.CandidateConsentDigest(submission)
	if digest == "" {
		return
	}
	setInteractiveCandidateConsentBinding(submission.ApplicationID, submission.TargetName, digest)
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
		SourceRepository: resolved.repositoryRoot(),
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
// missingCandidateConsentError builds the clean usage error returned when a
// repository-owned apply has no explicit --confirm-candidate. The message
// prints the EXACT confirm-hash the candidate would bind, so automation can
// parse the value and re-run with that exact digest. It is a *cli.UsageError
// (exit 2), never a runtime failure, and never a stack trace.
func missingCandidateConsentError(submission delivery.CandidateSubmission) error {
	digest := delivery.CandidateConsentDigest(submission)
	message := "candidate submission requires an explicit operator/automation consent (--confirm-candidate); refusing to record deployment state"
	hint := "Re-run with --confirm-candidate " + digest + " to acknowledge this exact submission."
	return fmt.Errorf("%w: %w", errConsentRequiredForCandidateSubmission, usageError(message, hint))
}

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

// interactiveCandidateStdin reports whether this invocation runs against a
// terminal stdin. Production pins the check to os.Stdin — the CLI's real
// interactive stream: the bind question must never be asked when stdin is
// piped, regardless of which reader a caller threaded through the lifecycle
// (an internal seam reader may be a bytes.Buffer that readerIsTerminal treats
// as a terminal). Interactive tests replace this check wholesale.
var interactiveCandidateStdin = func() bool {
	return os.Stdin != nil && readerIsTerminal(os.Stdin)
}

// interactiveCandidatePromptAllowed reports whether the bind prompt may run for
// this invocation. The decision AND the consumed answer must concern the SAME
// stream (CODEX-static finding (1) on PR #1884): deciding on os.Stdin while the
// prompt consumes a different threaded reader would let a queued 'y' on an
// internal seam reader pass as operator consent nobody typed.
//
// Shapes: (a) the canonical process stdin — an *os.File equal to os.Stdin — is
// interactive iff the production-pinned terminal check says so; (b) a threaded
// *os.File (the same-terminal wrapper, or a test-threaded stdin file): the
// reader must BE a terminal file (term.IsTerminal via the pinned os.Stdin check
// only for the os.Stdin identity, otherwise the reader's own terminal state);
// (c) any other stream (pipe, bytes.Buffer seam, file) is non-interactive and
// keeps the --confirm-candidate re-run contract. --no-input always refuses.
func interactiveCandidatePromptAllowed(ctx context.Context, in io.Reader) bool {
	if in == nil || noInput(ctx) {
		return false
	}
	// The terminal decision and the consumed stream must be the SAME stream
	// (CODEX-static finding (1) on PR #1884): deciding on os.Stdin while the
	// prompt consumes `in` would let an internal context reader (a
	// bytes.Buffer seam) whose queue holds a 'y' nobody typed pass as operator
	// consent. Production therefore decides on the very reader it hands to the
	// prompt. The interactiveCandidateStdin seam stays the deterministic test
	// switch for the terminal property of the canonical *os.File stdin shape.
	if file, ok := in.(*os.File); ok {
		if file != os.Stdin {
			// A threaded stdin file: interactive only when that very file is a
			// terminal (the pinned os.Stdin check says nothing about it).
			return terminalFileReader(file)
		}
		return interactiveCandidateStdin()
	}
	if wrapped, ok := in.(*terminalStdinReader); ok {
		if wrapped.File != os.Stdin {
			return terminalFileReader(wrapped.File)
		}
		return interactiveCandidateStdin()
	}
	return false
}

// terminalFileReader reports whether an *os.File is a terminal, through the
// same IsTerminal call readerIsTerminal uses for the *os.File shape.
func terminalFileReader(file *os.File) bool {
	return readerIsTerminal(file)
}

// sameTerminalStream reports whether the reader the prompt would consume is the
// very terminal file the interactive decision was made on. A buffered wrapper
// (bufio.Reader) over the same *os.File qualifies — withApplicationInput reuses
// it so identity survives — but any other stream (an internal bytes.Buffer
// seam, a pipe, a file) does not, even when the process runs on a terminal.
func sameTerminalStream(in io.Reader, tty *os.File) bool {
	if tty == nil {
		return false
	}
	switch r := in.(type) {
	case *os.File:
		return r == tty
	case *terminalStdinReader:
		return r.File == tty && r.Reader != nil
	case *bufio.Reader:
		// Unwrap exactly one buffering layer (the *os.File / wrapper / default
		// arms terminate this single step — no unbounded recursion): a pipe or
		// file stream directly under the buffer qualifies only when it IS the
		// terminal file; a bytes.Buffer seam or anything else fails closed.
		return sameTerminalStream(underlyingSource(r), tty)
	default:
		return false
	}
}

// underlyingSource discloses the immediate source of a bufio.Reader without
// consuming a byte: the prompt's own Read path is io.Reader-agnostic, and the
// identity question only has a meaningful answer for readers built over
// streams. Reflection is limited to bufio's unexported 'rd' field, which is
// stable across Go versions (documented in the test akte); a missing or
// unreadable field means "foreign stream".
func underlyingSource(r *bufio.Reader) io.Reader {
	v := reflect.Indirect(reflect.ValueOf(r)).FieldByName("rd")
	if !v.IsValid() || v.IsZero() || !v.CanInterface() {
		return nil
	}
	src, ok := v.Interface().(io.Reader)
	if !ok {
		return nil
	}
	return src
}

// terminalStdinReader is the typed CLI-boundary wrapper (0428-292 finding 1/2):
// withApplicationInput buffers the terminal stdin for the lifecycle while
// keeping the *os.File identity explicit, so sameTerminalStream can prove the
// prompt consumes exactly the stream the terminal decision was made on.
type terminalStdinReader struct {
	*bufio.Reader
	File *os.File
}
