package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/delivery"
	"github.com/mcpdev80/baseharbor/internal/deployment"
)

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

func recordDeploymentBeforeMutation(ctx context.Context, resolved resolvedApplication, state string, rawCandidateConsent string) (deployment.DeploymentRecord, error) {
	if !resolved.SourceAvailable {
		return deployment.DeploymentRecord{}, fmt.Errorf("cannot record pending deployment without available source")
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
			return deployment.DeploymentRecord{}, errConsentRequiredForCandidateSubmission
		}
		if err := submission.Validate(consent); err != nil {
			return deployment.DeploymentRecord{}, err
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
	return deployment.SaveDeploymentRecord(record)
}
