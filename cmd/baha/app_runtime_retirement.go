package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"io"
)

func (e *applicationDestroyExecution) retireRuntimePreservingData(ctx context.Context, out io.Writer) error {
	if err := e.runPreflight(ctx); err != nil {
		return err
	}
	if err := revalidateApplicationProviderIntent(ctx, e.resolved); err != nil {
		return err
	}
	withdrawn, plan, err := buildApplicationRuntimeRetirementPlan(ctx, e.resolved)
	if err != nil {
		return err
	}
	if err := json.NewEncoder(out).Encode(plan); err != nil {
		return err
	}
	fmt.Fprintln(out, "Runtime retirement removes selected owned workload/provider containers and application routes; persistent data, logical scopes, credentials, recovery files and canonical deployment owner are retained.")
	if !e.confirmed {
		fmt.Fprintln(out, "No changes were made. Review the retirement plan and required acknowledgements before --yes.")
		return nil
	}
	consent, err := invocationProviderChangeConsent(ctx)
	if err != nil {
		return err
	}
	if len(plan.Impact.Impacts) > 0 {
		if err := acknowledgeProviderImpact(ctx, applicationInput(ctx, nil), out, *plan.Impact, consent); err != nil {
			return err
		}
	}
	if err := revalidateApplicationProviderIntent(ctx, e.resolved); err != nil {
		return err
	}
	_, current, err := buildApplicationRuntimeRetirementPlan(ctx, e.resolved)
	if err != nil {
		return err
	}
	if current.Impact == nil || current.Impact.PlanID != plan.Impact.PlanID {
		return providerImpactConsentError(*plan.Impact, &application.ProviderChangeError{Code: "stale-impact-consent"})
	}
	// Unknown future consequences do not prohibit consent. Physical ownership
	// and complete consumer inventory remain necessary to execute exact retirement.
	if _, err := prepareApplicationProviderRetirement(ctx, withdrawn); err != nil {
		return err
	}
	retained, err := e.observeRetirementRuntimeMaterials()
	if err != nil {
		return err
	}
	pending, err := recordPendingApplicationRuntimeRetirement(e.resolved, *plan.Impact)
	if err != nil {
		return err
	}
	withdrawn.DeploymentRecord = &pending
	if err := e.cleanupDevelopmentCanonicalRoutes(ctx); err != nil {
		return err
	}
	if _, err := stopRepositoryWorkload(ctx, e.compose, e.resolved, e.files); err != nil {
		return err
	}
	if _, err := destroyRepositoryWorkloadRuntime(ctx, e.compose, e.resolved, e.files); err != nil {
		return err
	}
	if err := retireReleasedApplicationProviders(ctx, withdrawn, e.compose, nil, plan, out); err != nil {
		return err
	}
	if err := application.ReleaseApplicationProviderReferencesAt(e.resolved.TargetStateRoot, e.manifest); err != nil {
		return err
	}
	observed := observeApplicationProviderFacts(ctx, withdrawn)
	for _, change := range plan.Changes {
		if change.Action != "stop" && change.Action != "remove-runtime" {
			continue
		}
		fact, found := observed[change.InstanceID]
		if !found || fact.State != application.ProviderStoppedOwned || fact.Containers.Value == nil || *fact.Containers.Value != 0 {
			return &machine.Error{Code: machine.ErrorOwnershipAmbiguous, CauseCode: "runtime_retirement_unverified", Resource: change.InstanceID, Message: "Selected runtime retirement could not be verified.", Next: "Inspect the retained deployment owner and retry; data and previous intent remain available."}
		}
	}
	if err := recordVerifiedApplicationRuntimeRetirement(e.resolved, *plan.Impact, nil, retained); err != nil {
		return err
	}
	for _, material := range retained {
		fmt.Fprintf(out, "Retained %s: %s; runtime removal does not authorize data deletion or prove recovery usability.\n", material.Location.Reference, material.Evidence)
	}
	fmt.Fprintln(out, "Owned runtime retired; data and canonical recovery owner retained. Apply developer-authored intent to reactivate. Unverified function/recovery receipts remain partial.")
	return recordApplicationAudit(ctx, e.resolved, "destroy", "success", "runtime ownership verified; data retained", "owned runtime retired; consequence knowledge remains partial")
}
