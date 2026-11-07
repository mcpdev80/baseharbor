package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
	"github.com/mcpdev80/baseharbor/internal/delivery"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/targetaccess"
)

func init() {
	// Route tenant-managed (Coreless) targets through a genuine authorized-owner
	// path instead of the blanket TenantID bypass the review rejected: a tenant
	// deployment is admitted only when the selected Target's Access binding
	// resolves to an enrolled baseharbor-node-connector node with a valid access
	// scope. Every other target keeps the single Core installation-owner
	// enforcement. The hook defaults to fail-closed, so no tenant target is
	// admitted without authorization.
	deployment.TenantOwnerResolver = func(targetName string) (bool, error) {
		_, isTenant, ok, scopeErr := tenantResolvedForTarget(targetName)
		if scopeErr != nil {
			// A genuine tenant whose enrolled access scope is corrupt or
			// unreadable must surface a REAL error here, never be translated
			// into a silent fall-through: returning (false, nil) would let a
			// pre-existing core-owner admit the deployment despite the invalid
			// tenant scope. Propagate (false, err) so VerifyDeploymentOwner
			// reports the failure explicitly.
			return false, scopeErr
		}
		if !ok {
			// Not a resolvable tenant target: do NOT grant. Return (false, nil)
			// so VerifyDeploymentOwner falls through to the authoritative
			// single-installation-owner check for Core-governed targets
			// instead of admitting this target without an owner.
			return false, nil
		}
		if !isTenant {
			// Resolvable but not a genuine tenant: same fail-through. Grant
			// nothing so the Core owner check stays the sole authority.
			return false, nil
		}
		// Genuine tenant with a valid enrolled access scope: the scope is the
		// authorized-owner proof. Admit through the grant-only seam.
		return true, nil
	}
	// TEST-SCOPE ONLY owner loan (card t-20261106-r27-remote-up-podman-owner-missing,
	// lane w286): an isolated test Target without its own installation.json may
	// resolve against the boot Target's genuine Core installation, but only
	// through the marker- and provenance-guarded verifier below. Tenant and
	// connector Targets never reach a grant through this hook: the tenant hook
	// above runs first and any non-granting result that is not a plain
	// os.ErrNotExist load failure never consults the loan (see
	// internal/deployment.VerifyDeploymentOwner).
	deployment.TargetLoanVerifier = deploymentTargetLoanVerifier
}

func recordPendingDeployment(ctx context.Context, resolved resolvedApplication) (deployment.DeploymentRecord, error) {
	return recordDeploymentBeforeMutation(ctx, resolved, "configured", "")
}

// installationLoanLedger grants the single one-time installation loan per
// isolated test Target within one baha process. The loan never transfers or
// duplicates installation ownership: it lets VerifyDeploymentOwner resolve a
// marker-proven isolated TEST Target against the installation loaded from the
// boot Target's runtime root, so exactly one installation.json keeps durable
// ownership (ADR 0021 §1, §2) while each parallel test cell owns its own
// resource namespace. The ledger is held under the same .installation.lock
// primitive the production owner check uses.
type installationLoanLedger struct {
	mu       sync.Mutex
	granted  map[string]coreinstallation.State
	bootRoot string
}

var installationLoans = &installationLoanLedger{granted: map[string]coreinstallation.State{}}

func (l *installationLoanLedger) lookup(targetName string) (coreinstallation.State, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	state, ok := l.granted[targetName]
	return state, ok
}

func (l *installationLoanLedger) grant(targetName string, state coreinstallation.State) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.granted[targetName] = state
}

func (l *installationLoanLedger) currentBoot() (string, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.bootRoot == "" {
		return "", false
	}
	return l.bootRoot, true
}

func (l *installationLoanLedger) setBoot(root string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.bootRoot = root
}

// reset clears the ledger; the regression fixtures call it to assert a
// deterministic cold-start grant within one process.
func (l *installationLoanLedger) reset() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.granted = map[string]coreinstallation.State{}
	l.bootRoot = ""
}

// testLoanAuthorityForTarget implements the TEST-SCOPE ONLY loan branch of the
// one-installation-owner check. It returns a non-nil failure whenever ANY
// production-shaped condition holds; only a marker-proven, freshly isolated
// test Target whose boot Core is READY may receive a loan authority. It never
// creates, rewrites or copies an installation.json: the returned authority is
// the very State the boot Target's runtime root loads, so the boot
// installation stays the single durable owner of everything this process
// writes.
func testLoanAuthorityForTarget(targetName string) (coreinstallation.State, error) {
	if tenantScopedTarget(targetName) {
		return coreinstallation.State{}, errors.New("test owner loan refused: tenant targets keep the enrolled-scope authority")
	}
	if !bahaTestProcessEnvironment() {
		return coreinstallation.State{}, errors.New("test owner loan refused: no baha test-process provenance")
	}
	// Startup-bound management authorities (the remote/MCP execution shape)
	// never lend their installation across Targets: the w273 remote
	// expectation stays fail-closed. The guard reads the current command
	// context installed at the write seam below.
	if authority, bound := bahaLoanContext().Value(coreAuthorityContextKey{}).(deployment.ResolvedTarget); bound && authority.AccessProvider != "" && authority.AccessProvider != string(targetaccess.ProviderLocal) {
		return coreinstallation.State{}, errors.New("test owner loan refused: a startup-bound management installation may not lend its authority across Targets")
	}
	targetRoot, err := deployment.TargetStateRoot(targetName)
	if err != nil {
		return coreinstallation.State{}, err
	}
	if !bahaTestIsolationMarkerPresent(targetRoot) {
		return coreinstallation.State{}, errors.New("test owner loan refused: Target carries no baseharbor.test-isolation/v1 marker")
	}
	cfg, err := deployment.LoadConfig()
	if err != nil {
		return coreinstallation.State{}, err
	}
	target, err := cfg.ResolveTarget(targetName, "")
	if err != nil {
		return coreinstallation.State{}, err
	}
	if target.AccessProvider != "" && target.AccessProvider != string(targetaccess.ProviderLocal) {
		return coreinstallation.State{}, errors.New("test owner loan refused: connector/remote access Targets keep the management-installation authority")
	}
	if state, ok := installationLoans.lookup(targetName); ok {
		if err := coreinstallation.AssertStateIsGenuineInstallation(state); err != nil {
			return coreinstallation.State{}, err
		}
		// A cached grant is only valid while the boot installation itself
		// still loads as genuine and READY from the boot root.
		bootRoot, err := bahaTestBootRuntimeRoot(cfg, targetName)
		if err != nil {
			return coreinstallation.State{}, err
		}
		fresh, err := coreinstallation.Load(bootRoot)
		if err != nil {
			return coreinstallation.State{}, err
		}
		if !fresh.Ready || fresh.ID != state.ID {
			return coreinstallation.State{}, errors.New("test owner loan refused: the boot installation changed after the grant")
		}
		return state, nil
	}
	bootRoot, err := bahaTestBootRuntimeRoot(cfg, targetName)
	if err != nil {
		return coreinstallation.State{}, err
	}
	state, err := coreinstallation.Load(bootRoot)
	if err != nil {
		return coreinstallation.State{}, err
	}
	if !state.Ready {
		return coreinstallation.State{}, errors.New("test owner loan refused: the boot installation is not ready")
	}
	if state.Spec.Target == targetName {
		return coreinstallation.State{}, errors.New("test owner loan refused: the Target is already the installation owner")
	}
	// Acquire the lock primitive over the marker-proven test root before
	// recording the loan. A protected, non-symlinked state root is a hard
	// requirement of the production owner rule; the loan never widens it.
	unlock, err := coreinstallation.AcquireLifecycleLock(targetRoot)
	if err != nil {
		return coreinstallation.State{}, err
	}
	defer unlock()
	if boot, had := installationLoans.currentBoot(); had && boot != bootRoot {
		// A single process may only ever lend from ONE boot installation;
		// a second distinct root means the selection changed mid-process and
		// the ledger resets to fail closed on the next verification.
		installationLoans.reset()
		return coreinstallation.State{}, errors.New("test owner loan refused: the boot installation changed within one process")
	}
	installationLoans.grant(targetName, state)
	installationLoans.setBoot(bootRoot)
	return state, nil
}

// deploymentTargetLoanVerifier is the consumer binding of the TEST-SCOPE ONLY
// seam in internal/deployment.VerifyDeploymentOwner (card
// t-20261106-r27-remote-up-podman-owner-missing, lane w286). The seam calls
// this only after coreinstallation.Load of the Target's own runtime root
// returned os.ErrNotExist, so a genuine foreign installation.json at the
// Target still fails closed with ErrDeploymentOwnerForeign and never reaches
// a loan. Any refusal surfaces as ErrDeploymentOwnerMissing at the seam.
func deploymentTargetLoanVerifier(targetName string) (*coreinstallation.State, error) {
	if targetName == "" {
		return nil, errors.New("test owner loan refused: empty Target name")
	}
	state, err := testLoanAuthorityForTarget(targetName)
	if err != nil {
		return nil, err
	}
	return &state, nil
}

// bahaLoanContext is the context view of the loan guard; see
// setLoanExecutionContext.
func bahaLoanContext() context.Context {
	return loanContextForTests
}

// loanContextForTests holds the current command context for the context-free
// loan guard. Production commands install it via setLoanExecutionContext at
// the deployment write seams; the initial value is context.Background, so the
// guard always observes a usable context.
var loanContextForTests context.Context = context.Background()

// setLoanExecutionContext records the CURRENT command context for the
// context-free loan guard: every CLI command runs its write seams on one
// goroutine, and the guard needs the remote/MCP startup-bound authority to
// refuse across Targets. The value is only consulted by the TEST-SCOPE loan
// path, so a stale entry can never widen a production check.
func setLoanExecutionContext(ctx context.Context) {
	if ctx != nil {
		loanContextForTests = ctx
	}
}

func tenantScopedTarget(targetName string) bool {
	cfg, err := deployment.LoadConfig()
	if err != nil {
		return false
	}
	// Same context-free fallback selection as the boot resolution: the loan
	// seam must never consult BASEHARBOR_TARGET, and the tenant guard covers
	// the whole configuration shape — the activation value alone can make the
	// CLI resolve a different definition than the name-addressed one.
	target, err := cfg.ResolveTarget(targetName, bahaTestBootTargetName(cfg))
	return err == nil && strings.TrimSpace(target.TenantID) != ""
}

// The marker constants live with the test writer in
// deployment_owner_loan_regression_test.go so the trigger never appears in a
// production binary; the reader guard agrees with it byte-for-byte:
// "baseharbor.test-isolation.json" / "baseharbor.test-isolation/v1".
func bahaTestIsolationMarkerPresent(targetRoot string) bool {
	path := filepath.Join(targetRoot, "baseharbor.test-isolation.json")
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return false
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var marker struct {
		Version string `json:"version"`
	}
	if json.Unmarshal(raw, &marker) != nil || marker.Version != "baseharbor.test-isolation/v1" {
		return false
	}
	return true
}

// bahaTestBootRuntimeRoot derives the boot installation's runtime root from
// the SAME Target selection requireApplicationCore resolves (configuration
// DefaultTarget or the "local" fallback) and verifies it is a genuine Core
// installation before returning it.
// bahaTestBootRuntimeRoot resolves the boot installation's runtime root from
// the boot Target selection below and verifies it holds a genuine Core
// installation before returning it.
func bahaTestBootRuntimeRoot(cfg deployment.Config, targetName string) (string, error) {
	bootName := bahaTestBootTargetName(cfg)
	if bootName == targetName {
		return "", errors.New("test owner loan refused: boot Target equals the requested Target")
	}
	root, err := deployment.TargetStateRoot(bootName)
	if err != nil {
		return "", err
	}
	runtimeRoot := filepath.Join(root, "runtime")
	if _, err := coreinstallation.Load(runtimeRoot); err != nil {
		return "", err
	}
	return runtimeRoot, nil
}

// bahaTestBootTargetName names the boot installation the loan may come from.
// It mirrors the CLI's own fallback selection WITHOUT the BASEHARBOR_TARGET
// branch (the loan seam is context-free and never receives the command's
// per-call environment): the activated/persisted selection first — exactly as
// effectiveTarget prefers it — then the configuration default, then a lone
// configured Target, then the "local" fallback. Under runtime isolation
// BASEHARBOR_TARGET points at the marker-proven test Target itself; a loan can
// never come from the Target it is granted to — the equal-name guard in
// testLoanAuthorityForTarget fails closed in that case — while the boot
// resolution must still find the Core the trust plane configured with
// --default (an activated selection, when present, is the operator's own boot
// choice and therefore authoritative).
func bahaTestBootTargetName(cfg deployment.Config) string {
	if persisted, err := readPersistedTarget(); err == nil && persisted != "" {
		if _, ok := cfg.Targets[persisted]; ok || persisted == "local" {
			return persisted
		}
	}
	if name := strings.TrimSpace(cfg.DefaultTarget); name != "" {
		return name
	}
	if len(cfg.Targets) == 1 {
		for name := range cfg.Targets {
			return name
		}
	}
	return "local"
}

// bahaTestProcessEnvironment is the second, narrow provenance gate: the loan
// activates only inside a `go test`-executed baha. The first test below
// documents both gates so they stay reviewable at one place.
func bahaTestProcessEnvironment() bool {
	return bahaTestProcessEnv != nil && bahaTestProcessEnv()
}

// bahaTestProcessEnv is overridden only by the test harness; the production
// value is nil so this function cannot be activated from the environment.
var bahaTestProcessEnv func() bool

// errConsentRequiredForCandidateSubmission is returned when a repository-owned
// apply has no explicit operator/automation consent. A candidate submission may
// only be persisted when an explicit consent pins the exact validated
// submission; a missing consent fails closed before any deployment state write.
var errConsentRequiredForCandidateSubmission = errors.New(
	"candidate submission requires an explicit operator/automation consent (--confirm-candidate); refusing to record deployment state",
)

// errCandidateConsentDigestMismatch is returned when the operator confirm
// digest does not match the canonical content-hash of the exact submission the
// candidate leaf validated. Binding a wrong digest would let a stale, partial,
// or forged confirm authorize a different submission, so the apply path fails
// closed before writing any deployment state.
var errCandidateConsentDigestMismatch = errors.New(
	"candidate confirm digest does not match the validated submission identity; refusing to record deployment state",
)

// errCandidateAlreadyImmutable is returned when a repository-owned apply with a
// valid consent tries to record a candidate that is already persisted with its
// own consent for the same deployment identity. A candidate is immutable once
// bound: a second identical submission (same app/target/source identity and the
// same consented digest) is refused with this explicit cause rather than
// silently overwriting the already-bound consent. A genuinely new candidate
// (a different fingerprint/digest) is not caught by this guard and still applies.
var errCandidateAlreadyImmutable = errors.New(
	"candidate is already persisted with consent and is immutable; a second identical submission is refused, apply a changed source to record a new candidate",
)

func recordDeploymentBeforeMutation(ctx context.Context, resolved resolvedApplication, state string, rawCandidateConsent string) (deployment.DeploymentRecord, error) {
	if !resolved.SourceAvailable {
		return deployment.DeploymentRecord{}, fmt.Errorf("cannot record pending deployment without available source")
	}
	// One-installation-owner binding: authoritative deployment state may only be
	// written when the Target's Core installation owns its state directory. The
	// production write path asserts ownership; a missing or foreign installation
	// fails closed. The record identity is never rewritten to carry an
	// installation reference. A marker-proven isolated TEST Target with no own
	// installation may instead resolve against the boot Target's genuine
	// installation through the guarded loan seam inside
	// deployment.VerifyDeploymentOwner (see deploymentTargetLoanVerifier). The
	// current command context (including a startup-bound management authority
	// in the remote/MCP shape) is handed to the context-free guard here so the
	// w273 fail-closed expectation is enforced where the verification runs.
	setLoanExecutionContext(ctx)
	if resolved.TargetStateRoot != "" {
		if err := deployment.VerifyDeploymentOwner(resolved.DeploymentIdentity.Target, installationRootFor(resolved.TargetStateRoot), resolved.DeploymentIdentity.DeploymentID); err != nil {
			return deployment.DeploymentRecord{}, err
		}
	}
	remoteProject, err := retainedRemoteProject(resolved)
	if err != nil {
		return deployment.DeploymentRecord{}, err
	}
	intent, err := json.Marshal(resolved.Manifest)
	if err != nil {
		return deployment.DeploymentRecord{}, fmt.Errorf("encode normalized pending intent: %w", err)
	}
	revision, err := repositoryDesiredStateFingerprint(ctx, resolved)
	if err != nil {
		return deployment.DeploymentRecord{}, fmt.Errorf("fingerprint pending source: %w", err)
	}
	record := deployment.DeploymentRecord{
		Version:  deployment.DeploymentRecordVersion,
		Identity: resolved.DeploymentIdentity,
		Source: deployment.DeploymentSource{
			Kind:       "repository",
			Repository: resolved.repositoryRoot(),
			Manifest:   resolved.ManifestPath,
			Digest:     revision,
		},
		Applied: deployment.AppliedDeployment{
			RemoteProject:   remoteProject,
			Intent:          intent,
			RuntimeProvider: resolved.Target.RuntimeProvider,
			GeneratedState: map[string]string{
				"deployment": resolved.DeploymentStateRoot,
				"state":      filepath.Join(resolved.DeploymentStateRoot, "state"),
			},
			LastAppliedRef: revision,
		},
		References: deployment.ReferenceDimensions{
			// Intended (desired) reference resolved from the submitted source.
			deployment.ReferenceIntended: &deployment.ReferenceDimension{
				Release: revision,
			},
			// Applied (last-applied) reference mirrors the submit-time digest so a
			// fresh apply re-derives it; the dimension is written in isolation.
			deployment.ReferenceApplied: &deployment.ReferenceDimension{
				Release: revision,
			},
		},
		Observed: deployment.ObservedDeployment{
			State: state,
			Ready: false,
		},
	}
	if err := retainDeploymentGeneratedState(&record, false); err != nil {
		return deployment.DeploymentRecord{}, err
	}
	// Preserve the previous authoritative realization until convergence and
	// verification finish. An interrupted apply must still know what runtime
	// was previously requested, even after the repository intent changed.
	if state == "applying" {
		current, loadErr := deployment.LoadDeploymentRecord(resolved.DeploymentIdentity)
		if loadErr != nil && !errors.Is(loadErr, os.ErrNotExist) {
			return deployment.DeploymentRecord{}, loadErr
		}
		if loadErr == nil {
			previous := current.Applied.Intent
			if retained := current.Applied.GeneratedState["previous_intent"]; retained != "" {
				previous = json.RawMessage(retained)
			}
			var prior application.Manifest
			if len(previous) == 0 || json.Unmarshal(previous, &prior) != nil || prior.ApplicationID != resolved.Manifest.ApplicationID || prior.Environment != resolved.Manifest.Environment {
				return deployment.DeploymentRecord{}, invalidProviderConsumerInventory(fmt.Errorf("previous deployment intent does not match its owner"))
			}
			record.Applied.GeneratedState["previous_intent"] = string(previous)
		}
	}
	if err := recordBackendPlacementOwner(&record, resolved, state == "applying"); err != nil {
		return deployment.DeploymentRecord{}, err
	}
	// Fail closed at the real product submission point: for a repository-owned
	// application the deployment source IS the exact candidate identity bound to
	// this application and target before any state change. This choke point runs
	// only on the applying mutation (state == "applying"); the "configured"
	// pending state used by init/restore planning is not a candidate submission.
	// Validate it against the fail-closed candidate contract (exactly one
	// immutable identity: a 40/64-hex commit object ID or an opaque algo:hex
	// digest; a mutable branch/tag or an absent identity is rejected) before any
	// deployment state is written. Managed-app creates use a local source path
	// and are not candidate submissions, so they are not validated here.
	//
	// Fail-closed means "refuse without trace": the candidate identity is bound
	// to the deployment source the instant a partial applying record is written,
	// so a wrong or absent confirm digest must abort BEFORE that record is
	// persisted. A refused apply must leave the prior record (or none) untouched,
	// so a later app destroy can still resolve Core runtime files and tear
	// down cleanly, and a follow-up apply with valid consent reaches READY.
	if record.Source.Kind == "repository" && state == "applying" {
		submission := delivery.CandidateSubmission{
			ContractVersion:  delivery.CandidateContractVersion,
			ApplicationID:    record.Identity.ApplicationID,
			TargetName:       record.Identity.Target,
			SourceRepository: record.Source.Repository,
			SourceRevision:   record.Source.Digest,
			ImageDigest:      "",
		}
		// Finalize the operator's confirm digest into an ECHT consent: pin the
		// canonical content-hash of the validated submission so the persisted
		// consent binds the exact submission that is about to be written.
		consent, consErr := finalizeCandidateConsent(rawCandidateConsent, "operator", submission)
		if consErr != nil {
			return deployment.DeploymentRecord{}, consErr
		}
		if consent == nil {
			var buf [4096]byte
			n := runtime.Stack(buf[:], false)
			fmt.Fprint(os.Stderr, string(buf[:n]))
			return deployment.DeploymentRecord{}, errConsentRequiredForCandidateSubmission
		}
		if err := submission.Validate(consent); err != nil {
			return deployment.DeploymentRecord{}, err
		}
		// Immutability gate (r27 candidate-ingress, round 1): a candidate is
		// immutable once it is persisted with its own consent. A second identical
		// submission — the same deployment identity carrying the same consented
		// digest — is refused before any state is written so the already-bound
		// consent can never be silently overwritten. A genuinely new candidate
		// (a different fingerprint/digest) is not caught here and still applies.
		// The prior record is read through the owner-validated loader, so a missing
		// record (first apply) or a non-consented prior record is not an immutability
		// conflict; only a persisted consent with a matching digest is.
		//
		// Discriminator (OPTION 2): the gate fires only against a prior record that
		// has reached a COMPLETED/verified realization (Observed.State == "ready").
		// A pending "applying" record is an interrupted first attempt, not a
		// finished candidate, so a same-apply retry (same digest) must still
		// succeed — it is a re-attempt of an interrupted apply, not a silent
		// re-record. Only a second identical submission over an already-ready
		// deployment is refused. The freshly computed confirm digest pins this
		// submission; only an identical re-record over a finished candidate fails
		// closed.
		if prior, priorErr := deployment.LoadDeploymentRecord(resolved.DeploymentIdentity); priorErr == nil {
			if prior.Consent != nil && prior.Consent.Digest == consent.Digest && prior.Observed.State == "ready" {
				return deployment.DeploymentRecord{}, errCandidateAlreadyImmutable
			}
		}
		record.Consent = consent
	}
	if err := deployment.SaveDeploymentRecord(record); err != nil {
		return deployment.DeploymentRecord{}, err
	}
	return record, nil
}

func recordAppliedDeployment(ctx context.Context, resolved resolvedApplication, files application.RuntimeFiles) error {
	return recordAppliedDeploymentObservation(ctx, resolved, files, "ready", true)
}

func recordAppliedDeploymentObservation(ctx context.Context, resolved resolvedApplication, files application.RuntimeFiles, state string, ready bool) error {
	if !resolved.SourceAvailable {
		return fmt.Errorf("cannot record applied deployment without available source")
	}
	remoteProject, err := retainedRemoteProject(resolved)
	if err != nil {
		return err
	}
	intent, err := json.Marshal(resolved.Manifest)
	if err != nil {
		return fmt.Errorf("encode normalized applied intent: %w", err)
	}
	revision, err := repositoryDesiredStateFingerprint(ctx, resolved)
	if err != nil {
		return fmt.Errorf("fingerprint applied source: %w", err)
	}
	record := deployment.DeploymentRecord{
		Version:  deployment.DeploymentRecordVersion,
		Identity: resolved.DeploymentIdentity,
		Source: deployment.DeploymentSource{
			Kind:       "repository",
			Repository: resolved.repositoryRoot(),
			Manifest:   resolved.ManifestPath,
			Digest:     revision,
		},
		Applied: deployment.AppliedDeployment{
			RemoteProject:   remoteProject,
			Intent:          intent,
			RuntimeProvider: resolved.Target.RuntimeProvider,
			GeneratedState: map[string]string{
				"deployment": resolved.DeploymentStateRoot,
				"runtime":    files.Dir,
				"state":      filepath.Join(resolved.DeploymentStateRoot, "state"),
			},
			LastAppliedRef: revision,
		},
		Observed: deployment.ObservedDeployment{
			State: state,
			Ready: ready,
		},
	}
	// Preserve the other reference dimensions: advancing the applied dimension
	// must not drop the intended (desired) or prior observed dimension. Load the
	// existing record, update only the applied dimension, and save the merged
	// map so the three dimensions stay independent through the real lifecycle.
	// On the first apply there is no prior record yet: the missing-file error is
	// expected, so start from an empty map instead of aborting the write.
	existing, loadErr := deployment.LoadDeploymentRecord(resolved.DeploymentIdentity)
	if loadErr != nil && !errors.Is(loadErr, os.ErrNotExist) {
		return loadErr
	}
	if existing.References == nil {
		existing.References = deployment.ReferenceDimensions{}
	}
	existing.References[deployment.ReferenceApplied] = &deployment.ReferenceDimension{
		Kind:    deployment.ReferenceApplied,
		Release: revision,
		At:      time.Now().UTC(),
	}
	record.References = existing.References
	if err := retainDeploymentGeneratedState(&record, ready); err != nil {
		return err
	}
	if err := recordBackendPlacementOwner(&record, resolved, false); err != nil {
		return err
	}
	if ready {
		record.Observed.VerifiedAt = time.Now().UTC()
	}
	return deployment.SaveDeploymentRecord(record)
}

func recordObservedDeployment(resolved resolvedApplication, state string, ready bool) error {
	if resolved.DeploymentRecord == nil {
		return nil
	}
	record, err := deployment.LoadDeploymentRecord(resolved.DeploymentIdentity)
	if err != nil {
		return err
	}
	record.Observed = deployment.ObservedDeployment{State: state, Ready: ready}
	if ready {
		record.Observed.VerifiedAt = time.Now().UTC()
	}
	// Observe in isolation: update only the observed dimension and preserve the
	// intended and applied dimensions. Rebuild the whole map here (as the buggy
	// original did) would drop the applied dimension on convergence, after which
	// DeploymentRecord.Stale() sees no applied reference and passes silently.
	// Load the existing map, overwrite only the observed cell (falling back to
	// the applied reference when the runtime reports none), and save the merged
	// map so all three dimensions remain independent.
	existing, loadErr := deployment.LoadDeploymentRecord(resolved.DeploymentIdentity)
	if loadErr != nil && !errors.Is(loadErr, os.ErrNotExist) {
		return loadErr
	}
	if existing.References == nil {
		existing.References = deployment.ReferenceDimensions{}
	}
	observedRef := ""
	if applied, ok := existing.References[deployment.ReferenceApplied]; ok && applied != nil {
		observedRef = applied.Release
	}
	existing.References[deployment.ReferenceObserved] = &deployment.ReferenceDimension{
		Kind:    deployment.ReferenceObserved,
		Release: observedRef,
		At:      time.Now().UTC(),
	}
	record.References = existing.References
	return deployment.SaveDeploymentRecord(record)
}

// installationRootFor resolves the Core installation runtime root
// (targets/<target>/runtime) that VerifyDeploymentOwner reads, from a resolved
// resolved path and reuses deployment.TargetStateRoot so the shared
// "baseharbor/" data prefix is applied consistently. It returns "" when the
// Target name cannot be recovered or validated, so a caller without a
// resolvable Target is not gated.
func installationRootFor(targetStateRoot string) string {
	name := filepath.Base(targetStateRoot)
	root, err := deployment.TargetStateRoot(name)
	if err != nil {
		return ""
	}
	return filepath.Join(root, "runtime")
}

// tenantResolvedForTarget resolves a tenant-managed Target's Access binding and
// reports whether it is an enrolled tenant (with a valid access scope). The
// resolved binding is the genuine authorized-owner path for a Coreless target:
// its scope proves the node that governs the deployment, replacing the blanket
// TenantID bypass that the state-owner review rejected. The middle return
// distinguishes "not a tenant at all" (enforcement stays with the Core owner)
// from a tenant that lacks a valid enrolled binding. The final return carries
// the resolved access-scope error so the caller can propagate it as a real
// hook error instead of translating it into a silent non-tenant fall-through.
func tenantResolvedForTarget(targetName string) (deployment.ResolvedTarget, bool, bool, error) {
	cfg, err := deployment.LoadConfig()
	if err != nil {
		return deployment.ResolvedTarget{}, false, false, nil
	}
	resolved, err := cfg.ResolveTarget(targetName, "")
	if err != nil {
		return deployment.ResolvedTarget{}, false, false, nil
	}
	if resolved.TenantID == "" {
		return resolved, false, true, nil
	}
	scope, scopeErr := connectorScopeForTarget(resolved)
	if scopeErr != nil {
		// Do NOT collapse this into (isTenant=false, ok=true): a tenant whose
		// enrolled scope is corrupt or unreadable must be reported as an error,
		// never admitted through the Core-owner path.
		return deployment.ResolvedTarget{}, false, true, scopeErr
	}
	if scope.Validate() != nil {
		// Tenant without a valid scope: no error, just no authorization —
		// fail closed through the Core check.
		return deployment.ResolvedTarget{}, false, true, nil
	}
	return resolved, true, true, nil
}

// installationRootFor resolves the Core installation runtime root
