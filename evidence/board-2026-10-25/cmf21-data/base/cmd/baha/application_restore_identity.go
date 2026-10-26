package main

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
)

// A validated archive is an identity claim for recovery, not permission to
// allocate a different identity or ignore surviving owners. Keep this recovery
// adapter separate from ordinary lifecycle initialization.
func revalidateRestoreProviderIntent(ctx context.Context, resolved resolvedApplication) error {
	target, err := effectiveTarget(withTargetOverride(ctx, resolved.Target.Name))
	if err != nil {
		return err
	}
	stale := func() error {
		return classifyProviderChangeConsent(&application.ProviderChangeError{Code: "stale-impact-consent"})
	}
	if target != resolved.Target {
		return stale()
	}
	record, found, err := deployment.FindDeployment(target.Name, resolved.Manifest.ApplicationID, resolved.Manifest.Environment)
	if err != nil {
		return err
	}
	if found && record.Identity != resolved.DeploymentIdentity {
		return stale()
	}
	if !resolved.FromRepository || !resolved.SourceAvailable {
		return nil
	}
	selection, err := application.ResolveRepositoryEnvironment(resolved.RepositoryRoot, resolved.Manifest.Environment)
	if err != nil {
		return err
	}
	selection, err = resolveRestoreRepositorySource(ctx, target, selection, resolved.Manifest)
	if err != nil {
		return err
	}
	current := resolved
	current.Manifest = selection.Manifest
	selection.Manifest, err = resolveAppliedBackendSelection(current)
	if err != nil {
		return err
	}
	if selection.ManifestPath != resolved.ManifestPath || !reflect.DeepEqual(selection.Manifest, resolved.Manifest) {
		return stale()
	}
	return nil
}

// Called under the Target lifecycle lock after archive/owner consent and
// non-mutating preflight. Reuse the canonical Store and deployment owner so an
// interrupted recovery retains its generated deployment identity for retry.
func materializeRestoreIdentity(ctx context.Context, resolved resolvedApplication, data applicationRestoreData) error {
	if err := requireVerifiedRestoreScope(ctx, resolved); err != nil {
		return err
	}
	if err := revalidateRestoreProviderIntent(ctx, resolved); err != nil {
		return err
	}
	current, err := buildApplicationRestorePlan(ctx, resolved, data)
	if err != nil {
		return err
	}
	proof := ctx.Value(verifiedRestoreScopeKey{}).(verifiedRestoreScope)
	if current.Scope.PlanID != proof.PlanID {
		return applicationDestructionPlanError(current, &application.ProviderChangeError{Code: "stale-impact-consent"})
	}
	owner := application.Store{Root: filepath.Join(resolved.RepositoryRoot, ".baseharbor", "apps")}
	claims, err := repositoryIdentityClaims(resolved.Target, resolved.RepositoryRoot, owner, resolved.Manifest)
	if err != nil {
		return err
	}
	claims.ApplicationIDs = append(claims.ApplicationIDs, resolved.Manifest.ApplicationID)
	restored, err := owner.InitializeIdentity(resolved.Manifest, claims)
	if err != nil {
		return err
	}
	if restored.ApplicationID != resolved.Manifest.ApplicationID {
		return fmt.Errorf("restored canonical identity differs from validated archive")
	}
	_, found, err := deployment.FindDeployment(resolved.Target.Name, resolved.Manifest.ApplicationID, resolved.Manifest.Environment)
	if err != nil || found {
		return err
	}
	_, err = recordPendingDeployment(ctx, resolved)
	return err
}
