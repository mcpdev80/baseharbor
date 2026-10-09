package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
	"github.com/mcpdev80/baseharbor/internal/coreupdate"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/health"
	"github.com/mcpdev80/baseharbor/internal/identityprovider"
	platformopenbao "github.com/mcpdev80/baseharbor/internal/openbao"
)

// verifyUnchangedCoreForBinaryUpdate is intentionally narrower than a provider
// upgrade. A Core whose exact immutable provider identities already match the
// selected release can accept a binary-only update without restarts or migration.
// A Core with deployments or any data-bearing delta must use the future native
// provider lifecycle; treating those as binary-only is unsafe.
func verifyUnchangedCoreForBinaryUpdate(ctx context.Context, selectedRelease string) error {
	return verifyUnchangedCore(ctx, selectedRelease, false)
}

// Provider reconciliation has already admitted and verified owned application
// scopes. Its final inventory check must retain those deployments rather than
// applying the deliberately narrower binary-only admission rule.
func verifyReconciledCore(ctx context.Context, selectedRelease string) error {
	return verifyUnchangedCore(ctx, selectedRelease, true)
}

func verifyUnchangedCore(ctx context.Context, selectedRelease string, reconciled bool) error {
	target, err := effectiveTarget(ctx)
	if err != nil {
		return err
	}
	root, err := targetRuntimeStateRoot(target)
	if err != nil {
		return err
	}
	state, err := coreinstallation.Load(root)
	if err != nil {
		return fmt.Errorf("load owned Core identity: %w", err)
	}
	if !state.Ready || state.Spec.Target != target.Name || state.Spec.Runtime != target.RuntimeProvider {
		return errors.New("Core is not ready or selection no longer matches owning installation")
	}
	registrations, err := deployment.ListDeployments(target.Name)
	if err != nil {
		return fmt.Errorf("inspect application registry: %w", err)
	}
	if len(registrations) > 0 && !reconciled {
		return errors.New("Core has registered deployments; provider-native application/isolated inventory reconciliation is required")
	}
	rt, err := detectRuntimeForTarget(ctx, target)
	if err != nil {
		return err
	}
	plan, err := inspectCoreRuntimePlan(ctx, selectedRelease, state, rt)
	if err != nil {
		return err
	}
	if !reconciled && len(plan.Deltas) != 4 {
		return fmt.Errorf("binary-only Core update requires exactly four verified shared/backing providers, found %d", len(plan.Deltas))
	}
	for _, delta := range plan.Deltas {
		if delta.Classification != coreupdate.NoChange {
			return fmt.Errorf("Core %s/%s requires %s; binary-only update refused", delta.Installed.Scope, delta.Installed.Instance, delta.Classification)
		}
	}
	files, err := existingTargetRuntimeFiles(ctx)
	if err != nil {
		return fmt.Errorf("Core runtime files unavailable: %w", err)
	}
	if _, ready := health.Format(health.RuntimeChecksForFiles(files)); !ready {
		return errors.New("Core SQL/Secrets health is degraded")
	}
	if err := platformopenbao.CheckManager(ctx, rt, files); err != nil {
		return fmt.Errorf("Core secret manager cannot be verified: %w", err)
	}
	dataDir, err := targetDataRoot(target)
	if err != nil {
		return err
	}
	if err := identityprovider.VerifyCoreIdentity(ctx, dataDir, target.Name, state.ID, state.IdentityIssuer); err != nil {
		return fmt.Errorf("Core identity semantics failed: %w", err)
	}
	if reconciled {
		identity, err := identityprovider.ExistingCoreRuntimeFiles(dataDir, target.Name)
		if err != nil {
			return err
		}
		ops := &coreNativeRuntimeOps{runtime: rt, core: files, identity: identity, dataDir: dataDir, target: target.Name, installation: state.ID, issuer: state.IdentityIssuer}
		if err := ops.verifyOwnedApplicationSecretScopes(ctx); err != nil {
			return err
		}
		if err := ops.verifyOpenBaoBackingSQL(ctx); err != nil {
			return err
		}
		if err := ops.verifyKeycloakBackingSQL(ctx); err != nil {
			return err
		}
		return identityprovider.VerifyCoreOperatorTokenFlow(ctx, dataDir, target.Name, state.ID, state.IdentityIssuer)
	}
	return nil
}
