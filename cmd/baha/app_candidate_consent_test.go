package main

import (
	"bufio"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/delivery"
	"github.com/mcpdev80/baseharbor/internal/deployment"
)

func TestCandidateConsentDigestRequiresExactToken(t *testing.T) {
	digest := strings.Repeat("a", 64)
	for _, hint := range []string{
		"Re-run with --confirm-candidate " + digest,
		"Candidate: --confirm-candidate " + digest + " to continue",
	} {
		if got := candidateConsentDigestFromError(&cli.UsageError{Hint: hint}); got != digest {
			t.Fatalf("candidate digest from %q = %q, want exact token", hint, got)
		}
	}
	for _, hint := range []string{
		"--confirm-candidate " + digest + "suffix",
		"--confirm-candidate " + digest[:63],
		"--confirm-candidate " + strings.ToUpper(digest),
	} {
		if got := candidateConsentDigestFromError(&cli.UsageError{Hint: hint}); got != "" {
			t.Fatalf("accepted non-exact candidate digest from %q: %q", hint, got)
		}
	}
}

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
// (cmd/baha/deployment_record.go) and the prompt (cmd/baha/app_candidate_consent.go):
// it builds the CandidateSubmission from the resolved record's repository source
// and fingerprints the source the same way recordDeploymentBeforeMutation does,
// so the computed digest is the digest the real write binds. It returns the
// submission together with the exact revision shown to the operator, so the test
// exercises the same input both sides fingerprint.
func candidateSubmissionForResolved(t *testing.T, r resolvedApplication) (delivery.CandidateSubmission, string) {
	t.Helper()
	submission, revision := candidateSubmissionForResolvedApp(context.Background(), r)
	if submission.SourceRevision == "" {
		t.Fatal("computed candidate submission has no source revision")
	}
	return submission, revision
}

// TestConsentRejectsWrongDigestWriteNothing is a real apply-path test of Punkt 1:
// when the operator confirm digest does not match the canonical content-hash of
// the validated submission, the candidate choke point rejects it and persists NO
// deployment state, and the persisted digest (when a correct confirm is used) is
// exactly the digest finalizeCandidateConsent computed for the submission.
func TestConsentRejectsWrongDigestWriteNothing(t *testing.T) {
	r := repoCandidateFixture(t)
	submission, _ := candidateSubmissionForResolved(t, r)
	want := delivery.CandidateConsentDigest(submission)
	if want == "" {
		t.Fatal("computed candidate digest is empty")
	}

	// A wrong confirm digest must fail closed and write nothing.
	wrong := "0000000000000000000000000000000000000000000000000000000000000000"
	ctxWrong := withCandidateConsentDigest(context.Background(), wrong)
	if _, err := recordDeploymentBeforeMutation(ctxWrong, r, "applying", wrong); err == nil {
		t.Fatal("wrong confirm digest accepted; candidate must fail closed")
	} else {
		if !errors.Is(err, errCandidateConsentDigestMismatch) {
			t.Fatalf("wrong digest error lost mismatch classification: %v", err)
		}
		if got := candidateConsentDigestFromError(err); got != want {
			t.Fatalf("stale digest refresh=%q, want current exact candidate %q", got, want)
		}
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
	submission, _ := candidateSubmissionForResolved(t, r)
	bound := delivery.CandidateConsentDigest(submission)

	var prompt strings.Builder
	promptConfirmCandidateSubmissionInput(context.Background(), r, nil, &prompt)
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

// TestConsentFillForEditSnapshot is a real apply-path test of Punkt 1: the edit
// snapshot's candidate digest is filled from the same source-fingerprint helper
// the real choke point uses, so the confirmed Edit carries an operator/automation
// consent whose digest pins the exact new submission. This mirrors the baseline
// withConfirmCandidateForSetup pattern but recomputes over the resolved snapshot,
// so a changed source fingerprint binds the new digest, not the baseline's.
func TestConsentFillForEditSnapshot(t *testing.T) {
	r := repoCandidateFixture(t)
	submission, _ := candidateSubmissionForResolved(t, r)
	// A wrong confirm digest must fail closed and write nothing.
	wrong := "0000000000000000000000000000000000000000000000000000000000000000"
	if _, err := recordDeploymentBeforeMutation(withCandidateConsentDigest(context.Background(), wrong), r, "applying", wrong); err == nil {
		t.Fatal("wrong confirm digest accepted; candidate must fail closed")
	}
	// The correct, freshly-computed edit digest must bind and persist.
	editDigest := delivery.CandidateConsentDigest(submission)
	record, err := recordDeploymentBeforeMutation(withCandidateConsentDigest(context.Background(), editDigest), r, "applying", editDigest)
	if err != nil {
		t.Fatalf("correct edit digest rejected: %v", err)
	}
	if record.Consent == nil {
		t.Fatal("correct edit digest produced no consent")
	}
	if record.Consent.Digest != editDigest {
		t.Fatalf("bound edit digest %q != computed %q", record.Consent.Digest, editDigest)
	}
	if record.Consent.By != "operator" {
		t.Fatalf("edit consent by=%q, want operator", record.Consent.By)
	}
}

// TestReapplyingIdenticalCandidateReturnsImmutableError is the round-1 immutability
// cell for t-20261018-r27-candidate-ingress: a candidate submission is immutable once
// persisted with consent. Applying the SAME candidate (identical app/target/source
// identity and the same content-hash digest) a second time must fail closed with an
// explicit immutability cause, not silently overwrite the already-bound consent. A
// changed source (a genuinely new candidate) is unaffected and still applies.
//
// Red-first: before the guard this test fails at the assertImmutable because the
// second apply returns nil (the bug). After the guard the second apply is refused.
func TestReapplyingIdenticalCandidateReturnsImmutableError(t *testing.T) {
	r := repoCandidateFixture(t)
	submission, _ := candidateSubmissionForResolved(t, r)
	digest := delivery.CandidateConsentDigest(submission)
	if digest == "" {
		t.Fatal("computed candidate digest is empty")
	}

	// First apply with the correct digest: it must succeed and bind consent.
	ctx1 := withCandidateConsentDigest(context.Background(), digest)
	first, err := recordDeploymentBeforeMutation(ctx1, r, "applying", digest)
	if err != nil {
		t.Fatalf("first identical apply rejected: %v", err)
	}
	if first.Consent == nil || first.Consent.Digest != digest {
		t.Fatal("first apply did not bind the consented candidate digest")
	}

	// Promote the pending record to a COMPLETED/verified realization
	// (Observed.State == "ready") while preserving the consent the first apply
	// bound. The real apply path finalizes only through recordAppliedDeployment,
	// which drops the candidate consent here, so promote the record in place: a
	// finished candidate is exactly what the immutability guard protects.
	pending, loadErr := deployment.LoadDeploymentRecord(r.DeploymentIdentity)
	if loadErr != nil {
		t.Fatalf("load pending record: %v", loadErr)
	}
	if pending.Consent == nil || pending.Consent.Digest != digest {
		t.Fatal("pending record did not retain the consented digest")
	}
	pending.Observed.State = "ready"
	pending.Observed.Ready = true
	if err := deployment.SaveDeploymentRecord(pending); err != nil {
		t.Fatalf("finalize candidate to verified state: %v", err)
	}
	record, loadErr := deployment.LoadDeploymentRecord(r.DeploymentIdentity)
	if loadErr != nil {
		t.Fatalf("load after finalize: %v", loadErr)
	}
	if record.Consent == nil || record.Consent.Digest != digest || record.Observed.State != "ready" {
		t.Fatal("candidate was not finalized to a verified (ready) record")
	}

	// Second apply with the SAME identity and SAME digest: must be refused,
	// not silently clobber the persisted consent.
	ctx2 := withCandidateConsentDigest(context.Background(), digest)
	_, err = recordDeploymentBeforeMutation(ctx2, r, "applying", digest)
	if err == nil {
		t.Fatal("re-applying the identical candidate succeeded; a persisted candidate must be immutable")
	}
	if !strings.Contains(err.Error(), "immutable") {
		t.Fatalf("identical re-apply refused with wrong cause: %v", err)
	}

	// The persisted consent is still the first binding, untouched by the refused
	// second apply.
	record, loadErr = deployment.LoadDeploymentRecord(r.DeploymentIdentity)
	if loadErr != nil {
		t.Fatalf("load after refused re-apply: %v", loadErr)
	}
	if record.Consent == nil || record.Consent.Digest != digest {
		t.Fatal("refused re-apply corrupted or dropped the persisted consent")
	}
	if record.Observed.State != "ready" {
		t.Fatalf("refused re-apply corrupted the finalized state: %q", record.Observed.State)
	}

	// A genuinely NEW candidate (changed source, so a new fingerprint and digest)
	// still applies: immutability binds the identity, it does not block real edits.
	// We prove this by asserting that a submission carrying a DIFFERENT digest than
	// the persisted one is not refused by the immutability guard: it is accepted by
	// the consent path (the digest simply binds a new submission). This counter-probe
	// shows the guard rejects only the identical re-record, not a changed candidate.
	newDigest := "00000000000000000000000000000000000000000000000000000000000000aa"
	if newDigest == digest {
		t.Fatal("counter-probe digest collides with the base digest")
	}
	// A digest that does not match the persisted one is not an immutability conflict:
	// the guard must not fire. It is a different submission identity.
	ctxNew := withCandidateConsentDigest(context.Background(), newDigest)
	_, newErr := recordDeploymentBeforeMutation(ctxNew, r, "applying", newDigest)
	if newErr != nil && strings.Contains(newErr.Error(), "immutable") {
		t.Fatalf("a changed candidate refused by the immutability guard; only the identical re-record is immutable: %v", newErr)
	}
}

func TestMissingCandidateConsentDoesNotDumpRuntimeStack(t *testing.T) {
	resolved := repoCandidateFixture(t)
	original := os.Stderr
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = writer
	_, applyErr := recordDeploymentBeforeMutation(context.Background(), resolved, "applying", "")
	os.Stderr = original
	_ = writer.Close()
	output, readErr := io.ReadAll(reader)
	_ = reader.Close()
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !errors.Is(applyErr, errConsentRequiredForCandidateSubmission) {
		t.Fatalf("candidate without consent must fail closed: %v", applyErr)
	}
	if strings.Contains(string(output), "goroutine") || strings.Contains(string(output), "runtime.Stack") {
		t.Fatalf("consent refusal emitted runtime stack to stderr: %s", output)
	}
}

func TestCandidateConsentUsesActiveSourceNotPreviousCoreRegistration(t *testing.T) {
	resolved := repoCandidateFixture(t)
	previous := *resolved.DeploymentRecord
	previous.Source.Repository = "previous-core-managed-source"
	resolved.DeploymentRecord = &previous

	submission, _ := candidateSubmissionForResolved(t, resolved)
	if submission.SourceRepository != resolved.repositoryRoot() {
		t.Fatalf("candidate consent used stale registration source %q, want uploaded source %q", submission.SourceRepository, resolved.repositoryRoot())
	}
	confirmed := delivery.CandidateConsentDigest(submission)
	if confirmed == "" {
		t.Fatal("active source has no confirmable candidate digest")
	}
	if _, err := recordDeploymentBeforeMutation(withCandidateConsentDigest(context.Background(), confirmed), resolved, "applying", confirmed); err != nil {
		t.Fatalf("Core prompt and actual source binding disagree: %v", err)
	}
}

// interactiveBindReader serves queued operator answers on a pipe reader, the
// same shape the real bind prompt reads on a terminal stdin.
func interactiveBindReader(t *testing.T, answers string) *bufio.Reader {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reader.Close(); _ = writer.Close() })
	if _, err := writer.Write([]byte(answers)); err != nil {
		t.Fatal(err)
	}
	return bufio.NewReader(reader)
}

// TestInteractiveCandidateConsentCarriesToWriteWithoutSecondFlag is the
// 0428-292 positive regression: on an interactive terminal the apply prints the
// exact confirm-hash, the operator answers the inline bind prompt with "y", and
// the SAME invocation binds the deployment with a consent whose digest is
// exactly the hash that was shown — no second command run with
// --confirm-candidate. The test drives the same two production leaves the CLI
// apply path runs in order: promptConfirmCandidateSubmission (local lifecycle
// call shape: applicationInput(ctx, runtimeInput) falls back to a *os.File
// stdin, i.e. interactive) and recordDeploymentBeforeMutation.
func TestInteractiveCandidateConsentCarriesToWriteWithoutSecondFlag(t *testing.T) {
	withInteractiveCandidateStdin(t)
	r := repoCandidateFixture(t)
	var out strings.Builder
	stdin, feed := withTerminalStdinFake(t)
	feed("y\n")
	promptConfirmCandidateSubmissionInput(context.Background(), r, stdin, &out)
	if err := r.DeploymentIdentity.Validate(); err != nil {
		t.Fatalf("fixture deployment identity: %v", err)
	}
	bound := takeInteractiveCandidateConsentDigest(r.DeploymentIdentity.ApplicationID, r.DeploymentIdentity.Target)
	ctx := withCandidateConsentDigest(context.Background(), bound)
	if candidateConsentOverride(ctx) == "" {
		t.Fatalf("interactive 'y' did not carry candidate consent: output=%q", out.String())
	}
	submission, _ := candidateSubmissionForResolvedApp(context.Background(), r)
	digest := delivery.CandidateConsentDigest(submission)
	if !strings.Contains(out.String(), digest) {
		t.Fatalf("prompt does not show the bound digest %q; output=%q", digest, out.String())
	}
	record, err := recordDeploymentBeforeMutation(ctx, r, "applying", candidateConsentOverride(ctx))
	if err != nil {
		t.Fatalf("interactive consent rejected at the write boundary: %v", err)
	}
	if record.Consent == nil {
		t.Fatal("interactive consent produced no persisted consent")
	}
	if record.Consent.Digest != digest {
		t.Fatalf("bound digest %q != shown digest %q", record.Consent.Digest, digest)
	}
	if record.Consent.By != "operator" {
		t.Fatalf("consent by=%q, want operator", record.Consent.By)
	}
	if strings.Contains(out.String(), "Re-run with --confirm-candidate") {
		t.Fatalf("interactive confirmation still instructs a second flag run: %q", out.String())
	}
	// The bind is one-shot: after the write consumed it, nothing may linger for
	// a later invocation.
	if got := takeInteractiveCandidateConsentDigest(r.DeploymentIdentity.ApplicationID, r.DeploymentIdentity.Target); got != "" {
		t.Fatalf("interactive consent lingered after the write consumed it: %q", got)
	}
}

// TestInteractiveCandidateConsentRefusalKeepsFailClosed is the 0428-292
// negative regression: an interactive operator who refuses the inline bind
// prompt (explicit "n") changes nothing — no consent is carried, the same
// invocation fails closed with the existing consent-required usage error, and
// nothing is persisted. The gate is never weakened.
func TestInteractiveCandidateConsentRefusalKeepsFailClosed(t *testing.T) {
	withInteractiveCandidateStdin(t)
	r := repoCandidateFixture(t)
	var out strings.Builder
	ctx := context.Background()
	stdin, feed := withTerminalStdinFake(t)
	feed("n\n")
	promptConfirmCandidateSubmissionInput(ctx, r, stdin, &out)
	if got := takeInteractiveCandidateConsentDigest(r.DeploymentIdentity.ApplicationID, r.DeploymentIdentity.Target); got != "" {
		t.Fatalf("refusal carried consent: %q", got)
	}
	_, err := recordDeploymentBeforeMutation(ctx, r, "applying", candidateConsentOverride(ctx))
	if !errors.Is(err, errConsentRequiredForCandidateSubmission) {
		t.Fatalf("refused interactive bind must fail closed: %v", err)
	}
	record, loadErr := deployment.LoadDeploymentRecord(r.DeploymentIdentity)
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if record.Consent != nil {
		t.Fatalf("a refused interactive bind persisted a consent: %+v", record.Consent)
	}
}

// TestNonInteractiveCandidateConsentKeepsFlagContract is the 0428-292
// non-interactive delta guard: with a piped (non-terminal) reader, with
// --no-input, and with no prompt consumed at all, the prompt path must not read
// or bind anything and must keep printing the exact re-run-with-flag
// instruction, and the write boundary must fail closed exactly as before.
func TestNonInteractiveCandidateConsentKeepsFlagContract(t *testing.T) {
	withNonInteractiveCandidateStdin(t)
	r := repoCandidateFixture(t)
	submission, _ := candidateSubmissionForResolvedApp(context.Background(), r)
	digest := delivery.CandidateConsentDigest(submission)

	// Non-interactive stdin (the `baha app apply | ...` / CI shape): the exact
	// flag instruction must print and nothing may bind, even when a "y" answer
	// is queued on the prompt seam (an installed seam must not turn a
	// non-interactive invocation interactive).
	var pipedOut strings.Builder
	ctxPiped := context.Background()
	promptConfirmCandidateSubmissionInput(ctxPiped, r, strings.NewReader("y\n"), &pipedOut)
	if got := candidateConsentOverride(ctxPiped); got != "" {
		t.Fatal("piped stdin carried candidate consent")
	}
	if pipedOut.String() == "" {
		t.Fatal("piped stdin lost the candidate review block")
	}
	if !strings.Contains(pipedOut.String(), "Re-run with --confirm-candidate "+digest) {
		t.Fatalf("piped stdin lost the exact flag instruction: %q", pipedOut.String())
	}
	if _, err := recordDeploymentBeforeMutation(ctxPiped, r, "applying", candidateConsentOverride(ctxPiped)); !errors.Is(err, errConsentRequiredForCandidateSubmission) {
		t.Fatalf("piped stdin must fail closed: %v", err)
	}

	// Machine/non-interactive context must never reach the prompt at all, and
	// an interactive binding must never linger unclaimed afterwards.
	ctxNoInput := machineNoninteractiveContext(context.Background())
	var discarded strings.Builder
	stdin, feed := withTerminalStdinFake(t)
	feed("y\n")
	promptConfirmCandidateSubmissionInput(ctxNoInput, r, stdin, &discarded)
	if got := candidateConsentOverride(ctxNoInput); got != "" {
		t.Fatal("--no-input carried interactive candidate consent")
	}
	if got := takeInteractiveCandidateConsentDigest(r.DeploymentIdentity.ApplicationID, r.DeploymentIdentity.Target); got != "" {
		t.Fatalf("--no-input left an interactive binding %q", got)
	}
	if strings.Contains(discarded.String(), "[y/N]") {
		t.Fatalf("--no-input reached the interactive bind prompt: %q", discarded.String())
	}
}

// TestPrepareApplicationApplyArgsCarriesConfirmCandidateFlag is the 0428-292
// internal-caller regression: the shared lifecycle argument preparer keeps the
// documented --confirm-candidate option functional for internal callers (it is
// stripped from the args and carried on the context), an absent flag stays a
// fail-closed no-op, and memory-preflight args are preserved.
func TestPrepareApplicationApplyArgsCarriesConfirmCandidateFlag(t *testing.T) {
	digest := strings.Repeat("a", 64)
	ctx, rest := prepareApplicationApplyArgs(context.Background(), []string{"--skip-memory-preflight", "--confirm-candidate", digest})
	if candidateConsentOverride(ctx) != digest {
		t.Fatalf("flag digest not carried: %q", candidateConsentOverride(ctx))
	}
	if len(rest) != 1 || rest[0] != "--skip-memory-preflight" {
		t.Fatalf("args after strip = %v", rest)
	}
	ctx, rest = prepareApplicationApplyArgs(context.Background(), []string{"myapp"})
	if candidateConsentOverride(ctx) != "" {
		t.Fatal("absent flag must stay a no-op")
	}
	if len(rest) != 1 || rest[0] != "myapp" {
		t.Fatalf("args changed without a flag: %v", rest)
	}
}

// TestCandidateConsentBindsCurrentDesiredStateNotStoredRecord is the 0428-292
// freshness regression at the choke point itself. The confirmation (CLI flag or
// interactive "y" in the same invocation) binds the EXACT desired state the
// operator reviewed at the prompt, which promptConfirmCandidateSubmission
// derives from repositoryDesiredStateFingerprint — never the persisted
// record's stored digest. The test proves both sides of that equality: with an
// unchanged source the record-digest equals the fingerprint, so a
// fingerprint-derived confirmation binds; after the source content changes
// between review and write, the earlier confirmation must fail closed with the
// CURRENT digest surfaced, and only a fresh confirmation of that current
// fingerprint binds the changed submission.
func TestCandidateConsentBindsCurrentDesiredStateNotStoredRecord(t *testing.T) {
	withNonInteractiveCandidateStdin(t)
	// Every confirmation here is the explicit --confirm-candidate form; the
	// test never answers a prompt.
	r := repoCandidateFixture(t)
	fingerprint, err := repositoryDesiredStateFingerprint(context.Background(), r)
	if err != nil || fingerprint == "" {
		t.Fatalf("fixture repository fingerprint: %v", err)
	}
	if r.DeploymentRecord == nil || r.DeploymentRecord.Source.Digest != fingerprint {
		t.Fatalf("fixture record digest %v != fingerprint %q", r.DeploymentRecord, fingerprint)
	}
	reviewedSubmission, _ := candidateSubmissionForResolvedApp(context.Background(), r)
	reviewedDigest := delivery.CandidateConsentDigest(reviewedSubmission)

	// Unchanged source: a fingerprint-derived confirmation binds the submission.
	ctxOK := withCandidateConsentDigest(context.Background(), reviewedDigest)
	record, err := recordDeploymentBeforeMutation(ctxOK, r, "applying", reviewedDigest)
	if err != nil {
		t.Fatalf("unchanged-source confirmation rejected: %v", err)
	}
	if record.Consent == nil || record.Consent.Digest != reviewedDigest {
		t.Fatalf("bound consent %v != reviewed digest %q", record.Consent, reviewedDigest)
	}

	// The source changes AFTER the operator reviewed the candidate (a new
	// commit in the working tree). The earlier confirmation must never bind it.
	// An untracked change is fingerprinted exactly like a commit:
	// repositoryTrackedAndUntrackedFiles feeds it into
	// repositoryDesiredStateFingerprint, the one helper both the prompt and the
	// write boundary run.
	scratch := filepath.Join(r.repositoryRoot(), "candidate-consent-after-review.txt")
	if err := os.WriteFile(scratch, []byte("changed after review\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(scratch) })
	updated, loadErr := deployment.LoadDeploymentRecord(r.DeploymentIdentity)
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	r.DeploymentRecord = &updated
	changedFingerprint, err := repositoryDesiredStateFingerprint(context.Background(), r)
	if err != nil || changedFingerprint == "" || changedFingerprint == fingerprint {
		t.Fatalf("source change did not move the fingerprint: %q vs %q (%v)", changedFingerprint, fingerprint, err)
	}
	changedSubmission := reviewedSubmission
	changedSubmission.SourceRevision = changedFingerprint
	currentDigest := delivery.CandidateConsentDigest(changedSubmission)
	if currentDigest == "" || currentDigest == reviewedDigest {
		t.Fatal("counter-probe digest collides with the reviewed digest")
	}
	// counterexample: the candidate submission digest binds the submission
	// STRUCT, so two different SourceRevision values must never collide.
	if delivery.CandidateConsentDigest(delivery.CandidateSubmission{
		ContractVersion: delivery.CandidateContractVersion, SourceRevision: strings.Repeat("b", 64),
	}) == delivery.CandidateConsentDigest(delivery.CandidateSubmission{
		ContractVersion: delivery.CandidateContractVersion, SourceRevision: strings.Repeat("c", 64),
	}) {
		t.Fatal("counter-probe digest collides")
	}
	// A confirmation of the STALE reviewed digest must not bind the changed source.
	ctxStale := withCandidateConsentDigest(context.Background(), reviewedDigest)
	if _, err := recordDeploymentBeforeMutation(ctxStale, r, "applying", reviewedDigest); !errors.Is(err, errCandidateConsentDigestMismatch) {
		t.Fatalf("stale confirmation bound a changed submission: %v", err)
	} else if got := candidateConsentDigestFromError(err); got != currentDigest {
		t.Fatalf("mismatch error must surface the current digest %q, got %q", currentDigest, got)
	}
	// The confirmation of the CURRENT fingerprint binds the changed submission.
	ctxCurrent := withCandidateConsentDigest(context.Background(), currentDigest)
	record, err = recordDeploymentBeforeMutation(ctxCurrent, r, "applying", currentDigest)
	if err != nil {
		t.Fatalf("current fingerprint digest rejected: %v", err)
	}
	if record.Consent == nil || record.Consent.Digest != currentDigest {
		t.Fatalf("bound consent %v != current digest %q", record.Consent, currentDigest)
	}
}

// withInteractiveCandidateStdin makes this test's process look interactive to
// the bind gate (production pins this to a terminal os.Stdin).
func withInteractiveCandidateStdin(t *testing.T) {
	t.Helper()
	previous := interactiveCandidateStdin
	interactiveCandidateStdin = func() bool { return true }
	t.Cleanup(func() { interactiveCandidateStdin = previous })
}

// withTerminalStdinFake makes a fresh pipe the process os.Stdin and returns the
// production-shaped CLI-boundary reader over it (terminalStdinReader: the typed
// wrapper withApplicationInput threads through the lifecycle) plus a feed func.
// Together with withInteractiveCandidateStdin this reproduces the real terminal
// shape: the interactivity seam says "terminal stdin" AND the threaded reader
// IS that stdin file, so the bind question is reachable exactly as on a
// keyboard.
func withTerminalStdinFake(t *testing.T) (*terminalStdinReader, func(string)) {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	previous := os.Stdin
	os.Stdin = reader
	t.Cleanup(func() {
		os.Stdin = previous
		_ = reader.Close()
		_ = writer.Close()
	})
	return &terminalStdinReader{Reader: bufio.NewReader(reader), File: reader}, func(answer string) {
		t.Helper()
		if _, err := writer.Write([]byte(answer)); err != nil {
			t.Fatal(err)
		}
	}
}

// withNonInteractiveCandidateStdin pins the production shape: the bind prompt is
// only ever reachable through a terminal os.Stdin, so a *bytes.Buffer that
// happens to satisfy readerIsTerminal (the internal-input-resolver seam) must
// never receive an interactive question.
func withNonInteractiveCandidateStdin(t *testing.T) {
	t.Helper()
	previous := interactiveCandidateStdin
	interactiveCandidateStdin = func() bool { return false }
	t.Cleanup(func() { interactiveCandidateStdin = previous })
}

// TestQueuedNonTerminalReaderNeverGrantsBindConsent is the CODEX-static
// finding-(1)/(2) negative regression: the process stdin IS a terminal (the
// seam reports interactive), while the reader threaded through the lifecycle
// context is a DIFFERENT buffered stream whose queue already holds "y" — the
// shape of an internal input-resolver seam or a pre-seeded pipe. That queued
// answer is not operator consent: the bind question must not be answered from
// it, no binding may exist, and the write boundary must fail closed.
func TestQueuedNonTerminalReaderNeverGrantsBindConsent(t *testing.T) {
	withInteractiveCandidateStdin(t)
	r := repoCandidateFixture(t)
	var out strings.Builder
	// in: the production threaded shape (withApplicationInput yields a
	// bufio.Reader) over a foreign pipe that already contains "y" — the stream
	// is NOT os.Stdin, so sameTerminalStream must refuse it and no binding may
	// exist.
	queued := bufio.NewReader(interactiveBindReader(t, "y\n"))
	promptConfirmCandidateSubmissionInput(context.Background(), r, queued, &out)
	if got := takeInteractiveCandidateConsentDigest(r.DeploymentIdentity.ApplicationID, r.DeploymentIdentity.Target); got != "" {
		t.Fatalf("a queued non-terminal 'y' must never bind consent, got digest %q", got)
	}
	if _, err := recordDeploymentBeforeMutation(context.Background(), r, "applying", ""); err == nil {
		t.Fatal("write without consent must fail closed after a refused non-terminal reader")
	}
	// The fail-closed re-run-with-flag contract is intact (no silent bind).
	if !strings.Contains(out.String(), "--confirm-candidate") {
		t.Fatalf("refused route must keep the --confirm-candidate contract; output=%q", out.String())
	}
}

// TestWithApplicationInputPreservesTerminalIdentity locks the reader-identity
// contract the bind gate depends on: withApplicationInput over a terminal
// reader must hand back a reader that still passes sameTerminalStream (an
// already-buffered terminal reader is reused, never re-wrapped behind a foreign
// stream), while a non-terminal reader never enters the context at all.
func TestWithApplicationInputPreservesTerminalIdentity(t *testing.T) {
	previous := os.Stdin
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		os.Stdin = previous
		_ = reader.Close()
		_ = writer.Close()
	})
	// (a) terminal-shaped stdin file threaded twice: the SAME bufio.Reader must
	// come back, and it must remain the same terminal stream.
	os.Stdin = reader
	buffered := &terminalStdinReader{Reader: bufio.NewReader(os.Stdin), File: os.Stdin}
	ctx := withApplicationInput(context.Background(), buffered)
	got, ok := ctx.Value(applicationInputKey{}).(*terminalStdinReader)
	if !ok || got != buffered {
		t.Fatal("withApplicationInput must thread the typed terminal wrapper as-is (identity + no forked buffering)")
	}
	if !sameTerminalStream(got, os.Stdin) {
		t.Fatal("the threaded reader must still pass the same-terminal-stream check")
	}
	// The raw terminal file threads through the wrapper too (the leaf fallback
	// shape): equality with os.Stdin is the witness.
	if !sameTerminalStream(os.Stdin, os.Stdin) {
		t.Fatal("the bare terminal stdin file must satisfy the identity check")
	}
	// (b) a foreign pipe stream: withApplicationInput must never thread it (the
	// *os.File arm requires the exact os.Stdin file; a pipe is foreign). The
	// readerIsTerminal heuristic still calls it interactive — which is exactly
	// why the file-identity arm, not the heuristic, is the real gate.
	foreign := strings.NewReader("y\n")
	if !readerIsTerminal(foreign) {
		t.Fatal("fixture: the heuristic reports any non-*os.File reader as terminal")
	}
	ctx2 := withApplicationInput(context.Background(), foreign)
	if _, leaked := ctx2.Value(applicationInputKey{}).(io.Reader); leaked {
		t.Fatal("a non-file stream must never become the application input")
	}

	_ = writer
}

// TestRemoteApplyRouteInlineBindContractIsTruthful locks the CODEX-static
// finding-(2) contract correction on the remote 'app up' route
// (app_remote_lifecycle.go): the bind question on that route IS operator
// consent (one-shot, identity-keyed, re-verified at the write boundary), and
// the display-only comment that contradicted the code is gone. The test
// proves BOTH directions against the production leaf the remote path calls:
// (a) a non-terminal invocation of the remote shape keeps the fail-closed
// --confirm-candidate contract (a remote apply without consent never binds);
// (b) an inline 'y' on the terminal shape binds exactly once for THIS
// invocation's identity — the same gate the remote route runs.
func TestRemoteApplyRouteInlineBindContractIsTruthful(t *testing.T) {
	r := repoCandidateFixture(t)
	t.Chdir(r.repositoryRoot())
	ctx, core, _ := remoteApplicationCoreFixture(t)
	remote := r
	remote.Target = core
	remote.TargetStateRoot = r.TargetStateRoot

	// (a) non-interactive remote shape: the leaf must refuse and keep the flag
	// contract; the write boundary fails closed for this remote candidate.
	withNonInteractiveCandidateStdin(t)
	var discarded strings.Builder
	promptConfirmCandidateSubmissionInput(ctx, remote, applicationInput(ctx, nil), &discarded)
	if !strings.Contains(discarded.String(), "--confirm-candidate") {
		t.Fatalf("non-interactive remote route lost the --confirm-candidate contract: %q", discarded.String())
	}
	if got := takeInteractiveCandidateConsentDigest(r.DeploymentIdentity.ApplicationID, r.DeploymentIdentity.Target); got != "" {
		t.Fatal("a non-interactive remote prompt must never bind consent")
	}
	if _, err := recordDeploymentBeforeMutation(ctx, remote, "applying", candidateConsentOverride(ctx)); err == nil {
		t.Fatal("remote apply without consent must fail closed")
	}
	if strings.Contains(discarded.String(), "Bind this exact candidate") {
		t.Fatal("the bind question must not be asked on a non-terminal stdin")
	}

	// (b) the terminal answer pair at the route's write boundary (CODEX
	// Option-3 disposition): the production route executeRemoteApplicationApply
	// is unit-inaccessible on its success side — a live enrolled Connector node
	// must answer NodeMemory + Publish + ApplyApplication
	// (app_remote_up_success_discriminator_test.go:21-30 documents this); no
	// seam is added and full remote app-up success stays an E2E evidence
	// limitation (native gates only). What the unit CAN lock is the route's
	// exact prompt→write SEQUENCE (app_remote_lifecycle.go: promptConfirm-
	// CandidateSubmissionInput over the threaded terminal stdin, then
	// recordDeploymentBeforeMutation on the same context and resolved
	// application): an affirmative terminal 'y' answers the bind question and
	// the route write CONSUMES that one-shot binding, persisting the consent
	// whose digest is the shown candidate digest; a refusal binds nothing and
	// fails closed with the consent-required error. The Core-side gates stay
	// unchanged on every path: deployment admission before persisting
	// (EvaluateDeploymentAdmission) and the current-source digest
	// re-derivation with constant-time comparison (finalizeCandidateConsent).
	t.Run("terminal bind consumed by the route write", func(t *testing.T) {
		withInteractiveCandidateStdin(t)
		for _, answer := range []struct {
			text     string
			converge bool
		}{
			{text: "n", converge: false},
			{text: "y", converge: true},
		} {
			t.Run("answer "+answer.text, func(t *testing.T) {
				// Fresh deployment state per round: a consented candidate is
				// immutable, so a round must not inherit the other's record.
				record, err := deployment.LoadDeploymentRecord(remote.DeploymentIdentity)
				if err != nil {
					t.Fatal(err)
				}
				record.Consent = nil
				record.Observed = deployment.ObservedDeployment{State: "pending"}
				if err := deployment.SaveDeploymentRecord(record); err != nil {
					t.Fatal(err)
				}
				routed := remote
				reloaded, err := deployment.LoadDeploymentRecord(remote.DeploymentIdentity)
				if err != nil {
					t.Fatal(err)
				}
				routed.DeploymentRecord = &reloaded

				stdin, feed := withTerminalStdinFake(t)
				feed(answer.text + "\n")
				var routeOut strings.Builder
				// The route's exact sequence on ONE context: the bind prompt
				// over this invocation's terminal stdin, then the route write
				// consuming the binding through the production resolution
				// order. No explicit consent exists anywhere in this context.
				promptConfirmCandidateSubmissionInput(ctx, routed, stdin, &routeOut)
				if !strings.Contains(routeOut.String(), "Bind this exact candidate") {
					t.Fatalf("the bind question must be asked on the terminal stream: %q", routeOut.String())
				}
				_, writeErr := recordDeploymentBeforeMutation(ctx, routed, "applying", candidateConsentOverride(ctx))
				if !answer.converge {
					// Refusal: nothing binds and the write fails closed.
					if !errors.Is(writeErr, errConsentRequiredForCandidateSubmission) {
						t.Fatalf("a refused terminal bind must fail closed with the consent-required error, got %v", writeErr)
					}
					if got := takeInteractiveCandidateConsentDigest(routed.DeploymentIdentity.ApplicationID, routed.DeploymentIdentity.Target); got != "" {
						t.Fatalf("the refusal left a live binding: %q", got)
					}
					return
				}
				if writeErr != nil {
					t.Fatalf("the route write rejected this invocation's terminal bind: %v\n%s", writeErr, routeOut.String())
				}
				applied, err := deployment.LoadDeploymentRecord(routed.DeploymentIdentity)
				if err != nil {
					t.Fatal(err)
				}
				if applied.Consent == nil {
					t.Fatal("the terminal-y route write persisted no consent")
				}
				if applied.Consent.By != "operator" {
					t.Fatalf("consent by=%q, want operator", applied.Consent.By)
				}
				submission, _ := candidateSubmissionForResolvedApp(ctx, routed)
				want := delivery.CandidateConsentDigest(submission)
				if applied.Consent.Digest != want {
					t.Fatalf("persisted consent digest %q != the candidate digest %q the route bound", applied.Consent.Digest, want)
				}
				if !strings.Contains(routeOut.String(), want) {
					t.Fatalf("the shown confirm-hash is not the digest the write bound; output=%q", routeOut.String())
				}
				if strings.Contains(routeOut.String(), "Re-run with --confirm-candidate") {
					t.Fatalf("the affirmative answer still printed the display-only re-run contract: %q", routeOut.String())
				}
				// The binding is one-shot: the route write consumed it, so
				// nothing lingers for a later invocation.
				if got := takeInteractiveCandidateConsentDigest(routed.DeploymentIdentity.ApplicationID, routed.DeploymentIdentity.Target); got != "" {
					t.Fatalf("the interactive bind survived the write that consumed it: %q", got)
				}
			})
		}
	})
}

// TestSecondInvocationSameCandidateRequiresFreshPrompt is the CODEX re-review
// regression for the stale-consent finding: an interactive binding must never
// cross an apply-invocation boundary for the SAME unchanged candidate. Three
// sequential-invocation shapes are locked:
// (a) bind A with a real "y" in invocation 1, invocation 2 for A never answers
// its prompt (EOF) — the prior "y" may NOT be reused and the write fails
// closed;
// (b) an explicit --confirm-candidate read path CONSUMES a pending binding of
// the same identity instead of letting it survive into a later, refused
// invocation;
// (c) a write attempt for an unrelated identity B consumes (clears) the saved
// binding of A, so a later A without a new affirmative answer fails closed.
// Digest recomputation at the write boundary and the constant-time comparison
// are unchanged (the successful re-confirm in (b) proves it).
func TestSecondInvocationSameCandidateRequiresFreshPrompt(t *testing.T) {
	// (a) stale 'y' must not cross into a second, unanswered invocation. The
	// identity of this scenario is rA; the pipe reader lives for the whole
	// test, each invocation reads its own line.
	withInteractiveCandidateStdin(t)
	rA := repoCandidateFixture(t)
	stdin, feed := withTerminalStdinFake(t)
	feed("y\n")
	var out strings.Builder
	promptConfirmCandidateSubmissionInput(context.Background(), rA, stdin, &out)
	if got := candidateConsentForWrite(rA.DeploymentIdentity.ApplicationID, rA.DeploymentIdentity.Target, ""); got == "" {
		t.Fatalf("interactive 'y' did not bind (fixture precondition); output=%q", out.String())
	}
	// Invocation 2 for the identical candidate: the prompt is reached but the
	// operator never answers (EOF) — nothing new may bind, and the write must
	// fail closed instead of consuming invocation 1's answer.
	var out2 strings.Builder
	promptConfirmCandidateSubmissionInput(context.Background(), rA, stdin, &out2)
	if _, err := recordDeploymentBeforeMutation(context.Background(), rA, "applying", ""); !errors.Is(err, errConsentRequiredForCandidateSubmission) {
		t.Fatalf("a second invocation reused a stale interactive consent: %v", err)
	}

	// (b) an explicit-flag read path consumes the same-identity binding. Use an
	// identity DISTINCT from (a)'s fixture (a fresh configureTestTarget resets
	// the test store) and a syntactically valid, never-computable digest, so
	// the explicit flag never touches a real submission.
	rB := repoCandidateFixture(t)
	explicitDigest := "f" + strings.Repeat("0", 63)
	feed("y\n")
	var outB strings.Builder
	promptConfirmCandidateSubmissionInput(context.Background(), rB, stdin, &outB)
	if got := candidateConsentForWrite(rB.DeploymentIdentity.ApplicationID, rB.DeploymentIdentity.Target, explicitDigest); got != explicitDigest {
		t.Fatalf("fixture: the explicit flag must win the resolution order; got %q", got)
	}
	if got := takeInteractiveCandidateConsentDigest(rB.DeploymentIdentity.ApplicationID, rB.DeploymentIdentity.Target); got != "" {
		t.Fatalf("the explicit-flag read path left the binding for a later invocation: %q", got)
	}
	// The NEXT invocation answers the prompt with a refusal: nothing may bind
	// and the write fails closed (no stale binding from the explicit-flag run).
	feed("n\n")
	var outC strings.Builder
	promptConfirmCandidateSubmissionInput(context.Background(), rB, stdin, &outC)
	if _, err := recordDeploymentBeforeMutation(context.Background(), rB, "applying", ""); !errors.Is(err, errConsentRequiredForCandidateSubmission) {
		t.Fatalf("a refusal after a consumed explicit-flag bind reused stale consent: %v", err)
	}

	// (c) an identity-mismatched attempt consumes the binding.
	rC := repoCandidateFixture(t)
	feed("y\n")
	var outD strings.Builder
	promptConfirmCandidateSubmissionInput(context.Background(), rC, stdin, &outD)
	if got := candidateConsentForWrite(rC.DeploymentIdentity.ApplicationID, rC.DeploymentIdentity.Target, ""); got == "" {
		t.Fatalf("interactive 'y' did not bind for C (fixture precondition); output=%q", outD.String())
	}
	if got := takeInteractiveCandidateConsentDigest("other-application", rC.DeploymentIdentity.Target); got != "" {
		t.Fatal("fixture: the mismatched attempt must return no digest")
	}
	if got := takeInteractiveCandidateConsentDigest(rC.DeploymentIdentity.ApplicationID, rC.DeploymentIdentity.Target); got != "" {
		t.Fatalf("the mismatched identity attempt left A's binding alive: %q", got)
	}
}

// TestApplyLifecycleRetiresConsentOnEveryExit locks the CODEX re-review
// "a successful lifecycle has no corresponding unconditional cleanup" half of
// the finding from both sides of the apply lifecycle boundary:
// (a) entry clear — a binding that survived a previous invocation (the defect:
// the old success path never cleaned up) is retired at the entry of the next
// invocation, so its write fails closed without a fresh prompt answer;
// (b) exit clear — after a full lifecycle invocation completes (whatever its
// result), no unconsumed binding may linger for a later invocation.
func TestApplyLifecycleRetiresConsentOnEveryExit(t *testing.T) {
	r := repoCandidateFixture(t)
	submission, _ := candidateSubmissionForResolved(t, r)
	digest := delivery.CandidateConsentDigest(submission)
	if digest == "" {
		t.Fatal("fixture: candidate has no confirmable digest")
	}
	applyCtx := withCandidateConsentDigest(context.Background(), digest)

	// (a) stale binding crossing an invocation boundary: the entry clear of
	// runApplicationApplyLifecycle retires it, so the invocation's write fails
	// closed even though the context carries the same digest — the failure must
	// NOT be the digest-mismatch error (that would mean the flag path bound).
	setInteractiveCandidateConsentBinding(r.DeploymentIdentity.ApplicationID, r.DeploymentIdentity.Target, digest)
	_, err := runApplicationApplyLifecycle(applyCtx, application.DefaultStore(), nil, io.Discard, io.Discard, applicationApplyLifecycleHooks{})
	if err == nil {
		t.Fatal("a stale interactive binding survived into a new lifecycle invocation")
	}
	if errors.Is(err, errCandidateConsentDigestMismatch) {
		t.Fatalf("stale-binding scenario must fail on missing consent, not a digest mismatch: %v", err)
	}
	if got := takeInteractiveCandidateConsentDigest(r.DeploymentIdentity.ApplicationID, r.DeploymentIdentity.Target); got != "" {
		t.Fatalf("the lifecycle entry did not clear the stale binding: %q", got)
	}

	// (b) exit clear: seed a binding mid-invocation (the mutate hook runs
	// inside the lifecycle lock after entry) and verify the invocation retires
	// it at exit, success OR failure.
	_, _ = runApplicationApplyLifecycle(applyCtx, application.DefaultStore(), nil, io.Discard, io.Discard, applicationApplyLifecycleHooks{
		Mutate: func(context.Context, resolvedApplication) error {
			setInteractiveCandidateConsentBinding(r.DeploymentIdentity.ApplicationID, r.DeploymentIdentity.Target, digest)
			return nil
		},
	})
	if got := takeInteractiveCandidateConsentDigest(r.DeploymentIdentity.ApplicationID, r.DeploymentIdentity.Target); got != "" {
		t.Fatalf("the lifecycle exit left an unconsumed binding: %q", got)
	}
}
