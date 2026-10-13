package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
)

type applicationDestructionPlan struct {
	Scope     application.ProviderImpactScope      `json:"scope"`
	Providers application.ProviderPlan             `json:"providers"`
	Materials []application.ProviderMaterial       `json:"materials"`
	Effects   []application.OperationalConsequence `json:"effects"`
}

// Full reset and remote teardown use the same consequence authority as local
// retirement. Unknown remote material stays unknown; consent grants no ownership.
func buildApplicationDestructionPlan(ctx context.Context, resolved resolvedApplication, reset bool) (applicationDestructionPlan, error) {
	p := applicationDestructionPlan{}
	if err := authorizeApplicationOperation(ctx, "destroy", resolved); err != nil {
		return p, err
	}
	if err := revalidateApplicationProviderIntent(ctx, resolved); err != nil {
		return p, err
	}
	record, err := deployment.LoadDeploymentRecord(resolved.DeploymentIdentity)
	if err != nil || record.Identity != resolved.DeploymentIdentity {
		return p, runtimeMutationOwnerError()
	}
	_, p.Providers, err = buildApplicationRuntimeRetirementPlan(ctx, resolved)
	if err != nil {
		return p, err
	}
	root, err := deployment.DeploymentRoot(resolved.DeploymentIdentity)
	if err != nil {
		return p, err
	}
	p.Materials, _, err = observeOwnedProviderFiles(resolved.Manifest.ApplicationID, resolved.Target.Name, "deployment/"+record.Identity.DeploymentID, root)
	if err != nil {
		return p, err
	}
	config, err := deployment.ConfigPath()
	if err != nil {
		return p, err
	}
	configuration, err := runtimeMutationOwnerBytes(config, false)
	if err != nil {
		return p, err
	}
	defer zeroBytes(configuration)
	after := "Remove selected owned remote runtime containers; retain persistent data, protected publication, credentials and the deployment owner for retry/reactivation. Node data/access and recovery usability remain unverified."
	if reset {
		after = "Permanently delete the selected application-owned runtime data and protected deployment state where the existing executor supports it. Authored source, repository Compose data, foreign resources and other consumers remain outside this reset. No data handoff or continued application access is claimed; partial cleanup retains the existing owner for retry."
	}
	p.Effects = []application.OperationalConsequence{{Aspect: "application-destruction", Before: "Selected deployment, actual provider bindings and protected recovery references.", After: after, Evidence: "canonical deployment owner and existing consumer union; complete material/access consequences remain unverified"}}
	leaves := []application.ProviderImpactPlan{}
	if p.Providers.Impact != nil {
		leaves = append(leaves, *p.Providers.Impact)
	}
	p.Scope, err = application.ComposeProviderImpactScope(application.ProviderScopeOwner{Owner: "application-deployment", Subject: record.Identity.DeploymentID, Reference: filepath.Join(root, "deployment.json"), Destructive: reset, Revision: runtimeMutationRevision(struct {
		Record         deployment.DeploymentRecord
		ConfigRevision string
		Plan           applicationDestructionPlan
	}{record, runtimeMutationRevision(configuration), p})}, leaves, false)
	return p, err
}

func authorizeApplicationDestruction(ctx context.Context, resolved resolvedApplication, reset bool, out io.Writer) (applicationDestructionPlan, error) {
	p, err := buildApplicationDestructionPlan(ctx, resolved, reset)
	if err != nil {
		return p, err
	}
	if out == nil {
		out = io.Discard
	}
	fmt.Fprintln(out, "Achtung: review selected application destruction, affected consumers and material locations; ordinary --yes grants neither acknowledgement nor data deletion.")
	if err := writeJSON(out, p); err != nil {
		return p, err
	}
	consent, err := invocationProviderChangeConsent(ctx)
	if err != nil {
		return p, err
	}
	in := applicationInput(ctx, nil)
	if consent == nil && !noInput(ctx) && in != nil && readerIsTerminal(in) {
		ack, err := readProviderImpactAcknowledgements(bufio.NewReader(in), out, application.ProviderImpactPlan{PlanID: p.Scope.PlanID, ConfirmationsRequired: p.Scope.ConfirmationsRequired, Destructive: p.Scope.Destructive})
		if err != nil {
			return p, err
		}
		consent = &ack
	}
	ack := application.ProviderChangeConsent{PlanID: p.Scope.PlanID}
	if consent != nil {
		ack = *consent
	}
	if err := application.ValidateProviderImpactScopeConsent(p.Scope, ack); err != nil {
		return p, applicationDestructionPlanError(p, err)
	}
	current, err := buildApplicationDestructionPlan(ctx, resolved, reset)
	if err != nil {
		return p, err
	}
	if current.Scope.PlanID != p.Scope.PlanID {
		return current, applicationDestructionPlanError(current, &application.ProviderChangeError{Code: "stale-impact-consent"})
	}
	return p, nil
}

func applicationDestructionPlanError(p applicationDestructionPlan, err error) error {
	classified := classifyProviderChangeConsent(err)
	var failure *machine.Error
	if errors.As(classified, &failure) {
		failure.Details = p
	}
	return classified
}
