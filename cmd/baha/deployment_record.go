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

func init() {
	// Route tenant-managed (Coreless) targets through a genuine authorized-owner
	// path instead of the blanket TenantID bypass the review rejected: a tenant
	// deployment is admitted only when the selected Target's Access binding
	// resolves to an enrolled baseharbor-node-connector node with a valid access
	// scope. Every other target keeps the single Core installation-owner
	// enforcement. The hook defaults to fail-closed, so no tenant target is
	// admitted without authorization.
	deployment.TenantOwnerResolver = func(targetName string) (bool, error) {
		resolved, isTenant, ok := tenantResolvedForTarget(targetName)
		if !ok {
			// Not a resolvable tenant target: leave single-installation-owner
			// enforcement (below) as the sole authority for Core-governed targets.
			return true, nil
		}
		if !isTenant {
			return true, nil
		}
		scope, err := connectorScopeForTarget(resolved)
		if err != nil {
			return false, nil
		}
		return scope.Validate() == nil, nil
	}
}

func recordPendingDeployment(ctx context.Context, resolved resolvedApplication) (deployment.DeploymentRecord, error) {
	return recordDeploymentBeforeMutation(ctx, resolved, "configured")
}

func recordDeploymentBeforeMutation(ctx context.Context, resolved resolvedApplication, state string) (deployment.DeploymentRecord, error) {
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
// from a tenant that lacks a valid enrolled binding.
func tenantResolvedForTarget(targetName string) (deployment.ResolvedTarget, bool, bool) {
	cfg, err := deployment.LoadConfig()
	if err != nil {
		return deployment.ResolvedTarget{}, false, false
	}
	resolved, err := cfg.ResolveTarget(targetName, "")
	if err != nil {
		return deployment.ResolvedTarget{}, false, false
	}
	if resolved.TenantID == "" {
		return resolved, false, true
	}
	scope, err := connectorScopeForTarget(resolved)
	if err != nil {
		return deployment.ResolvedTarget{}, false, true
	}
	if scope.Validate() != nil {
		return deployment.ResolvedTarget{}, false, true
	}
	return resolved, true, true
}

// installationRootFor resolves the Core installation runtime root
