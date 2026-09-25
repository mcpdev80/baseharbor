package main

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
)

func recordAppliedDeployment(ctx context.Context, resolved resolvedApplication, files application.RuntimeFiles) error {
	if !resolved.SourceAvailable {
		return fmt.Errorf("cannot record applied deployment without available source")
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
			State:      "ready",
			Ready:      true,
			VerifiedAt: time.Now().UTC(),
		},
	}
	return deployment.SaveDeploymentRecord(record)
}

func recordObservedDeployment(resolved resolvedApplication, state string, ready bool) error {
	if resolved.DeploymentRecord == nil {
		return nil
	}
	record := *resolved.DeploymentRecord
	record.Observed = deployment.ObservedDeployment{
		State:      state,
		Ready:      ready,
		VerifiedAt: time.Now().UTC(),
	}
	return deployment.SaveDeploymentRecord(record)
}
