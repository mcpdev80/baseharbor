package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"

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
	operation := "destroy"
	if isApplicationStopMode(ctx) {
		operation = "app.stop"
	} else if proof, ok := ctx.Value(installationApplicationScopeKey{}).(installationApplicationScope); ok && proof.Children[resolved.DeploymentIdentity.DeploymentID] != "" {
		if err := application.ValidateProviderImpactScopeConsent(proof.Scope, proof.Consent); err != nil {
			return applicationDestructionPlan{}, classifyProviderChangeConsent(err)
		}
		operation = "installation.destroy"
	}
	return buildApplicationDestructionPlanForOperation(ctx, resolved, reset, operation)
}

func buildApplicationDestructionPlanForOperation(ctx context.Context, resolved resolvedApplication, reset bool, operation string) (applicationDestructionPlan, error) {
	p := applicationDestructionPlan{}
	if err := authorizeApplicationOperation(ctx, operation, resolved); err != nil {
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
		after = "Permanently delete exclusively app-owned runtime data and protected deployment state. Authored source, shared infrastructure, foreign resources and data with ambiguous ownership remain. No data handoff or continued application access is claimed; partial cleanup retains the existing owner for retry."
	}
	aspect := "application-destruction"
	if isApplicationStopMode(ctx) {
		aspect = "application-runtime-stop"
	}
	p.Effects = []application.OperationalConsequence{{Aspect: aspect, Before: "Selected deployment, actual provider bindings and protected recovery references.", After: after, Evidence: "canonical deployment owner and existing consumer union; complete material/access consequences remain unverified"}}
	leaves := []application.ProviderImpactPlan{}
	if p.Providers.Impact != nil {
		leaves = append(leaves, *p.Providers.Impact)
	}
	materialRevision, err := applicationImpactFileRevision(application.ProviderImpactObservation{Providers: map[string]application.ProviderImpactFact{
		record.Identity.DeploymentID: {Materials: p.Materials},
	}})
	if err != nil {
		return p, err
	}
	// Footprint quantities are runtime samples, not changed authority. The
	// shared impact plan ID already binds semantic provider/consumer effects.
	// Protected material content is bound separately without exposing digests.
	impactPlanID := ""
	if p.Providers.Impact != nil {
		impactPlanID = p.Providers.Impact.PlanID
	}
	p.Scope, err = application.ComposeProviderImpactScope(application.ProviderScopeOwner{Owner: "application-deployment", Subject: record.Identity.DeploymentID, Reference: filepath.Join(root, "deployment.json"), Destructive: reset, Revision: runtimeMutationRevision(struct {
		Record           deployment.DeploymentRecord
		ConfigRevision   string
		MaterialRevision string
		ImpactPlanID     string
		Effects          []application.OperationalConsequence
		Reset            bool
	}{record, runtimeMutationRevision(configuration), materialRevision, impactPlanID, p.Effects, reset})}, leaves, false)
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
	if err := printApplicationDestructionSummary(out, resolved, p, reset); err != nil {
		return p, err
	}
	consent, err := invocationProviderChangeConsent(ctx)
	if err != nil {
		return p, err
	}
	if delegated, present, err := delegatedInstallationApplicationConsent(ctx, p, reset); present {
		if err != nil {
			return p, applicationDestructionAuthorizationError(p, err)
		}
		consent = delegated
	}
	in := applicationInput(ctx, nil)
	if consent == nil && !noInput(ctx) && in != nil && readerIsTerminal(in) {
		ack, err := readProviderApplyAcknowledgement(bufio.NewReader(in), out, application.ProviderImpactPlan{PlanID: p.Scope.PlanID, ConfirmationsRequired: p.Scope.ConfirmationsRequired, Destructive: p.Scope.Destructive})
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
		return p, applicationDestructionAuthorizationError(p, err)
	}
	current, err := buildApplicationDestructionPlan(ctx, resolved, reset)
	if err != nil {
		return p, err
	}
	if current.Scope.PlanID != p.Scope.PlanID {
		return current, applicationDestructionAuthorizationError(current, &application.ProviderChangeError{Code: "stale-impact-consent"})
	}
	return p, nil
}

func printApplicationDestructionSummary(out io.Writer, resolved resolvedApplication, plan applicationDestructionPlan, reset bool) error {
	operation := "Stop"
	if reset {
		operation = "Destroy"
	}
	fmt.Fprintf(out, "\n%s application %s on Target %s\n", operation, resolved.Manifest.Name, resolved.Target.Name)
	if plan.Providers.Impact != nil {
		groups := map[string]map[string]bool{}
		for _, impact := range plan.Providers.Impact.Impacts {
			action := providerActionLabel(impact.Operation)
			if groups[action] == nil {
				groups[action] = map[string]bool{}
			}
			if len(impact.Functions) == 0 {
				groups[action][impact.InstanceID] = true
				continue
			}
			for _, function := range impact.Functions {
				groups[action][providerFunctionLabel(function.Function)] = true
			}
		}
		actions := make([]string, 0, len(groups))
		for action := range groups {
			actions = append(actions, action)
		}
		sort.Strings(actions)
		for _, action := range actions {
			services := make([]string, 0, len(groups[action]))
			for service := range groups[action] {
				services = append(services, service)
			}
			sort.Strings(services)
			fmt.Fprintf(out, "  %s providers: %s\n", action, strings.Join(services, ", "))
		}
	}
	if reset {
		fmt.Fprintln(out, "  App-owned data: permanently deleted when ownership is verified")
		fmt.Fprintln(out, "  Shared, foreign or unknown data: kept")
	} else {
		fmt.Fprintln(out, "  App data: preserved for reactivation")
	}
	if !plan.Scope.Complete {
		fmt.Fprintln(out, "  Inventory: incomplete; unverified resources are left untouched")
	}
	return nil
}

func applicationDestructionAuthorizationError(plan applicationDestructionPlan, err error) error {
	classified := applicationDestructionPlanError(plan, err)
	var failure *machine.Error
	if errors.As(classified, &failure) && failure.CauseCode == "impact-confirmation-required" {
		failure.Next = "No changes were made. Run baha app destroy NAME --yes in an interactive terminal, review the short plan and confirm once."
	}
	return classified
}

func applicationDestructionPlanError(p applicationDestructionPlan, err error) error {
	classified := classifyProviderChangeConsent(err)
	var failure *machine.Error
	if errors.As(classified, &failure) {
		failure.Details = p
	}
	return classified
}
