package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
)

// The decrypted, validated archive supplies the original identity when its
// canonical owner is lost. Reconcile every surviving source/deployment claim
// first; matching portable YAML alone cannot adopt another application's owner.
// This remains read-only and never manufactures a replacement identity.
func resolveRestoreRepositorySource(ctx context.Context, target deployment.ResolvedTarget, selection application.RepositoryEnvironmentSelection, archived application.Manifest) (application.RepositoryEnvironmentSelection, error) {
	owner := application.Store{Root: filepath.Join(selection.RepositoryRoot, ".baseharbor", "apps")}
	claims, err := repositoryIdentityClaims(target, selection.RepositoryRoot, owner, selection.Manifest)
	if err != nil {
		return selection, err
	}
	claims.ApplicationIDs = append(claims.ApplicationIDs, archived.ApplicationID)
	selection.Manifest, err = application.ResolveIdentity(selection.Manifest, claims)
	if err != nil {
		return selection, err
	}
	return resolveRepositoryLifecycleIntent(ctx, target, selection, "restore")
}

func resolveRestoreTarget(ctx context.Context, _ application.Store, backupManifest application.Manifest) (resolvedApplication, error) {
	target, err := effectiveTarget(ctx)
	if err != nil {
		return resolvedApplication{}, err
	}
	targetRoot, err := deployment.TargetStateRoot(target.Name)
	if err != nil {
		return resolvedApplication{}, err
	}
	existing, deploymentFound, err := deployment.FindDeployment(target.Name, backupManifest.ApplicationID, backupManifest.Environment)
	if err != nil {
		return resolvedApplication{}, err
	}
	var id deployment.DeploymentIdentity
	if deploymentFound {
		id = existing.Identity
		id.Application = backupManifest.Name
	} else {
		id, err = deployment.NewDeploymentIdentity(target.Name, backupManifest.ApplicationID, backupManifest.Name, backupManifest.Environment)
		if err != nil {
			return resolvedApplication{}, err
		}
	}
	deploymentRoot, err := deployment.DeploymentRoot(id)
	if err != nil {
		return resolvedApplication{}, err
	}
	resolved := resolvedApplication{
		Target:              target,
		DeploymentIdentity:  id,
		Manifest:            backupManifest,
		TargetStateRoot:     targetRoot,
		DeploymentStateRoot: deploymentRoot,
		Store:               application.Store{Root: filepath.Join(deploymentRoot, "state"), Namespace: target.Name},
	}

	cwd, err := os.Getwd()
	if err != nil {
		return resolved, err
	}
	found, err := application.HasRepositoryApplication(cwd)
	if err != nil {
		return resolved, err
	}
	if !found {
		var record deployment.DeploymentRecord
		var recordErr error
		if deploymentFound {
			record = existing
			record.Identity = id
		} else {
			recordErr = os.ErrNotExist
		}
		if recordErr == nil && deployment.SourceAvailable(record.Source) {
			selection, sourceErr := application.ResolveRepositoryEnvironment(record.Source.Repository, backupManifest.Environment)
			if sourceErr != nil {
				return resolved, fmt.Errorf("restore preflight registered source: %w", sourceErr)
			}
			selection, sourceErr = resolveRestoreRepositorySource(ctx, target, selection, backupManifest)
			if sourceErr != nil {
				return resolved, sourceErr
			}
			if selection.Manifest.YAML() != backupManifest.YAML() {
				return resolved, errors.New("registered repository environment manifest does not match backup desired state")
			}
			resolved.DeploymentRecord = &record
			resolved.ManifestPath = selection.ManifestPath
			resolved.RepositoryRoot = selection.RepositoryRoot
			resolved.SourceAvailable = true
			resolved.FromRepository = true
			resolved.Manifest, err = resolveAppliedBackendSelection(resolved)
			return resolved, err
		}
		if recordErr != nil && !errors.Is(recordErr, os.ErrNotExist) {
			return resolved, fmt.Errorf("restore preflight deployment record: %w", recordErr)
		}
		return resolved, errors.New("restore preflight source repository is unavailable; run restore from the matching repository or restore the registered repository path before mutation")
	}
	selection, err := application.ResolveRepositoryEnvironment(cwd, backupManifest.Environment)
	if err != nil {
		return resolved, err
	}
	selection, err = resolveRestoreRepositorySource(ctx, target, selection, backupManifest)
	if err != nil {
		return resolved, err
	}
	if selection.Manifest.YAML() != backupManifest.YAML() {
		return resolved, errors.New("selected repository environment manifest does not match backup desired state")
	}
	resolved.ManifestPath = selection.ManifestPath
	resolved.RepositoryRoot = selection.RepositoryRoot
	resolved.SourceAvailable = true
	resolved.FromRepository = true
	resolved.Manifest, err = resolveAppliedBackendSelection(resolved)
	return resolved, err
}
