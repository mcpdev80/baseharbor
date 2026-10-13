package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
)

// Destruction uses the existing impact-scope authority. This projection is
// deliberately incomplete until actual provider/consumer transitions are wired;
// it cannot report delivery, authentication or renewal continuity.
type runtimeDestroyImpact struct {
	Scope      application.ProviderImpactScope      `json:"scope"`
	All        bool                                 `json:"all"`
	Resources  []ownedDestroyResource               `json:"resources"`
	Preserved  []fullDestroyResult                  `json:"preserved"`
	Core       []runtimeDestroyCoreImpact           `json:"core"`
	Effects    []application.OperationalConsequence `json:"effects"`
	LocalState []application.ResourceLocation       `json:"local_state_selected_for_deletion"`
}

type runtimeDestroyCoreImpact struct {
	Target          string                            `json:"target"`
	Project         string                            `json:"project"`
	Functions       []application.FunctionConsequence `json:"functions"`
	Materials       []application.MaterialConsequence `json:"materials"`
	RuntimeObserved bool                              `json:"runtime_observed"`
	Processes       []coreStopProcessObservation      `json:"observed_processes"`
}

func buildRuntimeDestroyImpact(ctx context.Context, plans []targetDestroyInventory, preserved []fullDestroyResult, all bool) (runtimeDestroyImpact, error) {
	p := runtimeDestroyImpact{All: all, Core: []runtimeDestroyCoreImpact{}, Preserved: preserved}
	_, report := withDestroyReport(ctx, all)
	recordDestroyPlan(context.WithValue(ctx, destroyReportContextKey{}, report), plans, preserved)
	p.Resources = report.Resources
	p.Preserved = report.Preserved
	config, err := deployment.ConfigPath()
	if err != nil {
		return p, err
	}
	data, err := runtimeMutationOwnerBytes(config, true)
	if err != nil {
		return p, err
	}
	owners := map[string]string{config: runtimeMutationRevision(data)}
	zeroBytes(data)
	if all {
		root, err := deployment.DataRoot()
		if err != nil {
			return p, err
		}
		p.LocalState = []application.ResourceLocation{{Owner: "installation", Kind: "path", Reference: root}, {Owner: "installation", Kind: "path", Reference: filepath.Dir(config)}}
	} else {
		for _, plan := range plans {
			root, err := targetRuntimeStateRoot(plan.Target)
			if err != nil {
				return p, err
			}
			p.LocalState = append(p.LocalState, application.ResourceLocation{Owner: "target/" + plan.Target.Name, Target: plan.Target.Name, Kind: "path", Reference: root})
		}
	}
	operation := "control-plane.destroy"
	if all {
		operation = "installation.destroy"
	}
	for _, plan := range plans {
		if err := authorizeMCPOperation(ctx, operation, plan.Target.Name, "dev", "", ""); err != nil {
			return p, err
		}
		records, err := deployment.ListDeployments(plan.Target.Name)
		if err != nil {
			return p, invalidProviderConsumerInventory(err)
		}
		for _, record := range records {
			if err := authorizeMCPOperation(ctx, operation, plan.Target.Name, record.Identity.Environment, record.Identity.ApplicationID, ""); err != nil {
				return p, err
			}
			owners["deployment/"+record.Identity.DeploymentID] = runtimeMutationRevision(record)
		}
		root, err := targetRuntimeStateRoot(plan.Target)
		if err != nil {
			return p, err
		}
		if err := observeDestroyConsumerAuthority(ctx, operation, plan.Target, owners); err != nil {
			return p, err
		}
		if _, err := os.Lstat(filepath.Join(root, "compose.yaml")); err == nil {
			core, err := buildControlPlaneStopPlanForOperation(withTargetOverride(ctx, plan.Target.Name), operation)
			if err != nil {
				return p, err
			}
			// The stop projection's files are selected for deletion here. Never
			// reuse its retained-material or temporary-outage statements.
			for i := range core.Materials {
				core.Materials[i].After = application.MaterialDisposition{MaterialID: core.Materials[i].Before.ID, Disposition: "unknown"}
				core.Materials[i].Meaning = "Destruction selects owned runtime state for permanent deletion. External recovery locations shown separately are preserved. Exact deletion and all consumer consequences require post-execution verification."
			}
			core.Effects = nil
			for i := range core.Functions {
				core.Functions[i].Behavior = "Permanent destruction may remove required SQL, secret, certificate, identity and management access. No replacement, authentication bypass or renewal continuity is claimed; cached token/certificate validity and downstream effects remain unknown."
			}
			p.Core = append(p.Core, runtimeDestroyCoreImpact{Target: core.Target, Project: core.Project, Functions: core.Functions, Materials: core.Materials, RuntimeObserved: core.RuntimeObserved, Processes: core.ObservedProcesses})
			owners["core-observation/"+plan.Target.Name] = core.Scope.Owner.Revision
			for path, revision := range core.protected {
				owners[path] = revision
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return p, runtimeMutationOwnerError()
		}
	}
	p.Effects = []application.OperationalConsequence{{Aspect: "permanent-destruction", Before: "Existing owned resources, registered consumers and retained protected state.", After: "Permanently delete the displayed owned resources and selected local runtime state; --all also selects application deployments and installation configuration. Source repositories and displayed external recovery material are preserved. No working secret delivery, login, leases, certificates, renewal or consumer reconfiguration is guaranteed.", Evidence: "existing native ownership inventory; complete consumer transitions remain unverified"}}
	p.Scope, err = application.ComposeProviderImpactScope(application.ProviderScopeOwner{Owner: operation, Reference: config, Subject: operation, Destructive: true, Revision: runtimeMutationRevision(struct {
		Plan    runtimeDestroyImpact
		Owners  map[string]string
		Targets []deployment.ResolvedTarget
	}{p, owners, destroyImpactTargets(plans)})}, nil, false)
	return p, err
}

func destroyImpactTargets(plans []targetDestroyInventory) []deployment.ResolvedTarget {
	values := make([]deployment.ResolvedTarget, len(plans))
	for i := range plans {
		values[i] = plans[i].Target
	}
	return values
}

func authorizeRuntimeDestroyImpact(ctx context.Context, p runtimeDestroyImpact, requested bool, out io.Writer) (bool, error) {
	if report, ok := ctx.Value(destroyReportContextKey{}).(*destroyReport); ok {
		report.Impact = &p
	}
	if p.Scope.Destructive {
		fmt.Fprintln(out, "Achtung: permanent destruction requires two current-plan confirmations and separate data-deletion consent.")
	} else {
		fmt.Fprintln(out, "Achtung: selected runtime removal requires two current-plan confirmations; persistent data and protected owners remain.")
	}
	if err := writeJSON(out, p); err != nil {
		return false, err
	}
	consent, err := invocationProviderChangeConsent(ctx)
	if err != nil {
		return false, err
	}
	in := applicationInput(ctx, nil)
	if consent == nil && !noInput(ctx) && in != nil && readerIsTerminal(in) {
		value, err := readProviderImpactAcknowledgements(bufio.NewReader(in), out, application.ProviderImpactPlan{PlanID: p.Scope.PlanID, ConfirmationsRequired: p.Scope.ConfirmationsRequired, Destructive: p.Scope.Destructive})
		if err != nil {
			return false, err
		}
		consent = &value
	}
	if consent == nil && !requested {
		fmt.Fprintln(out, "No changes were made. Supply the two current-plan acknowledgements and separate data-deletion consent to execute.")
		return false, nil
	}
	ack := application.ProviderChangeConsent{PlanID: p.Scope.PlanID}
	if consent != nil {
		ack = *consent
	}
	if err := application.ValidateProviderImpactScopeConsent(p.Scope, ack); err != nil {
		return false, runtimeDestroyImpactError(p, err)
	}
	return true, nil
}

func runtimeDestroyImpactError(p runtimeDestroyImpact, err error) error {
	err = classifyProviderChangeConsent(err)
	var failure *machine.Error
	if errors.As(err, &failure) {
		failure.Details = p
	}
	return err
}
