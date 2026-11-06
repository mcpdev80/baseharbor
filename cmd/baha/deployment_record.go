package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/delivery"
	"github.com/mcpdev80/baseharbor/internal/deployment"
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
}

func recordPendingDeployment(ctx context.Context, resolved resolvedApplication) (deployment.DeploymentRecord, error) {
	return recordDeploymentBeforeMutation(ctx, resolved, "configured", "")
}

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
	// installation reference.
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
