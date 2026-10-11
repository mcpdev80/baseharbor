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
	"github.com/mcpdev80/baseharbor/internal/deployment"
)

func recordPendingDeployment(ctx context.Context, resolved resolvedApplication) (deployment.DeploymentRecord, error) {
	return recordDeploymentBeforeMutation(ctx, resolved, "configured")
}

func recordDeploymentBeforeMutation(ctx context.Context, resolved resolvedApplication, state string) (deployment.DeploymentRecord, error) {
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
