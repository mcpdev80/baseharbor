package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
)

type restorePromptOutputKey struct{}
type verifiedRestoreScopeKey struct{}

type verifiedRestoreScope struct {
	Identity deployment.DeploymentIdentity
	Intent   string
	PlanID   string
}

func prepareAuthorizedApplicationRestore(ctx context.Context, resolved resolvedApplication, data applicationRestoreData, out io.Writer) (context.Context, func(), error) {
	unlock, err := lockApplicationTargetLifecycle(resolved)
	if err != nil {
		return ctx, nil, err
	}
	planned, err := buildApplicationRestorePlan(ctx, resolved, data)
	if err != nil {
		unlock()
		return ctx, nil, err
	}
	if err := authorizeApplicationRestore(ctx, resolved, data, out); err != nil {
		unlock()
		return ctx, nil, err
	}
	if err := validateApplicationProviderExecution(resolved, planned.Providers.Impact); err != nil {
		unlock()
		return ctx, nil, err
	}
	ctx = context.WithValue(ctx, verifiedRestoreScopeKey{}, verifiedRestoreScope{Identity: resolved.DeploymentIdentity, Intent: resolved.Manifest.YAML(), PlanID: planned.Scope.PlanID})
	return ctx, unlock, nil
}

func requireVerifiedRestoreScope(ctx context.Context, resolved resolvedApplication) error {
	proof, ok := ctx.Value(verifiedRestoreScopeKey{}).(verifiedRestoreScope)
	if !ok || proof.Identity != resolved.DeploymentIdentity || proof.Intent != resolved.Manifest.YAML() {
		return machine.NewError(machine.ErrorApprovalRequired, "Restore reset requires the verified current archive/owner consequence authority.", "Use the normal restore lifecycle with two current acknowledgements and separate owned-data consent.", false)
	}
	return nil
}

// Restore overwrites selected durable state. The validated archive binds the
// same existing consequence scope, rather than granting implicit reset consent.
func buildApplicationRestorePlan(ctx context.Context, resolved resolvedApplication, data applicationRestoreData) (applicationDestructionPlan, error) {
	p := applicationDestructionPlan{}
	if err := authorizeApplicationOperation(ctx, "restore", resolved); err != nil {
		return p, err
	}
	if len(data.archiveRevision) != 64 || data.manifest.YAML() != resolved.Manifest.YAML() {
		return p, classifyProviderChangeConsent(&application.ProviderChangeError{Code: "invalid-impact-plan"})
	}
	if err := revalidateRestoreProviderIntent(ctx, resolved); err != nil {
		return p, err
	}
	var err error
	_, p.Providers, err = buildApplicationRuntimeRetirementPlan(ctx, resolved)
	if err != nil {
		return p, err
	}
	root, err := deployment.DeploymentRoot(resolved.DeploymentIdentity)
	if err != nil {
		return p, err
	}
	ownerPath := filepath.Join(root, "deployment.json")
	owner, err := runtimeMutationOwnerBytes(ownerPath, true)
	if err != nil {
		return p, err
	}
	defer zeroBytes(owner)
	if len(owner) != 0 {
		record, err := deployment.LoadDeploymentRecord(resolved.DeploymentIdentity)
		if err != nil || record.Identity != resolved.DeploymentIdentity {
			return p, runtimeMutationOwnerError()
		}
	}
	if _, err := os.Lstat(root); err == nil {
		p.Materials, _, err = observeOwnedProviderFiles(resolved.Manifest.ApplicationID, resolved.Target.Name, "deployment/"+resolved.DeploymentIdentity.DeploymentID, root)
		if err != nil {
			return p, err
		}
	} else if !os.IsNotExist(err) {
		return p, err
	}
	configPath, err := deployment.ConfigPath()
	if err != nil {
		return p, err
	}
	config, err := runtimeMutationOwnerBytes(configPath, false)
	if err != nil {
		return p, err
	}
	defer zeroBytes(config)
	materialRevision, err := applicationImpactFileRevision(application.ProviderImpactObservation{Providers: map[string]application.ProviderImpactFact{
		resolved.DeploymentIdentity.DeploymentID: {Materials: p.Materials},
	}})
	if err != nil {
		return p, err
	}
	sourceRevision, err := repositoryDesiredStateFingerprint(ctx, resolved)
	if err != nil {
		return p, err
	}
	placements, err := desiredBackendPlacements(resolved.Manifest)
	if err != nil {
		return p, err
	}
	sourceRevision = runtimeMutationRevision(struct {
		Source     string
		Placements any
	}{sourceRevision, placements})
	p.Effects = []application.OperationalConsequence{{Aspect: "application-recovery", Before: "Selected application runtime, data, credentials and retained recovery owners at the displayed locations.", After: "Stop the selected workload, reset its owned backend and secret scope, and overwrite the selected durable state from the validated recovery archive. Newer selected data can be permanently lost. Recreate ephemeral identities and verify actual restored services before claiming success. Unselected, shared and foreign state remains outside archive restoration; no universal credential, lease or login continuity is claimed.", Evidence: "validated encrypted archive and current protected owners; full consumer consequences and interrupted recovery remain unverified"}}
	externalEffects, externalRevision, err := restoreExternalRecoveryScope(resolved)
	if err != nil {
		return p, err
	}
	p.Effects = append(p.Effects, externalEffects...)
	sourceRevision = runtimeMutationRevision(struct{ Source, External string }{sourceRevision, externalRevision})
	leaves := []application.ProviderImpactPlan{}
	if p.Providers.Impact != nil {
		leaves = append(leaves, *p.Providers.Impact)
	}
	subject, reference := resolved.DeploymentIdentity.DeploymentID, ownerPath
	if len(owner) == 0 {
		subject = "application/" + resolved.Manifest.ApplicationID + "/" + resolved.Manifest.Environment
		reference = resolved.ManifestPath
	}
	p.Scope, err = application.ComposeProviderImpactScope(application.ProviderScopeOwner{Owner: "application-restore", Subject: subject, Reference: reference, Destructive: true, Revision: runtimeMutationRevision(struct {
		Archive, Owner, Config, Materials, Source string
		Effects                                   []application.OperationalConsequence
	}{data.archiveRevision, runtimeMutationRevision(owner), runtimeMutationRevision(config), materialRevision, sourceRevision, p.Effects})}, leaves, false)
	return p, err
}

func authorizeApplicationRestore(ctx context.Context, resolved resolvedApplication, data applicationRestoreData, out io.Writer) error {
	if promptOut, ok := ctx.Value(restorePromptOutputKey{}).(io.Writer); ok {
		out = promptOut
	}
	p, err := buildApplicationRestorePlan(ctx, resolved, data)
	if err != nil {
		return err
	}
	if out == nil {
		out = io.Discard
	}
	fmt.Fprintln(out, "Achtung: restore can overwrite newer owned data. Confirm this exact archive/owner plan twice and separately authorize data deletion; ordinary approval supplies none of these decisions.")
	if err := writeJSON(out, p); err != nil {
		return err
	}
	consent, err := invocationProviderChangeConsent(ctx)
	if err != nil {
		return err
	}
	in := applicationInput(ctx, guidedBackupInput)
	if consent == nil && !noInput(ctx) && in != nil && readerIsTerminal(in) {
		ack, err := readProviderImpactAcknowledgements(bufio.NewReader(in), out, application.ProviderImpactPlan{PlanID: p.Scope.PlanID, ConfirmationsRequired: p.Scope.ConfirmationsRequired, Destructive: true})
		if err != nil {
			return err
		}
		consent = &ack
	}
	ack := application.ProviderChangeConsent{PlanID: p.Scope.PlanID}
	if consent != nil {
		ack = *consent
	}
	if err := application.ValidateProviderImpactScopeConsent(p.Scope, ack); err != nil {
		return applicationDestructionPlanError(p, err)
	}
	current, err := buildApplicationRestorePlan(ctx, resolved, data)
	if err != nil {
		return err
	}
	if current.Scope.PlanID != p.Scope.PlanID {
		return applicationDestructionPlanError(current, &application.ProviderChangeError{Code: "stale-impact-consent"})
	}
	return nil
}
