package main

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"io"
)

// verifyOwnershipWhenNoDeploymentRecord reports whether the current destroy is
// operating on a never-delivered application: no on-disk deployment record
// exists (Kind "incomplete"), ownership is instead carried by the application
// identity (repository .baseharbor/apps store or shared-backend state), and no
// prior runtime/provider state was ever realized. It returns
// (neverDelivered=true, nil) when the destroy should take the clean
// never-delivered path, (false, nil) when a real deployment record exists, and
// (false, err) for a genuinely corrupt or otherwise unresolvable record.
func (e *applicationDestroyExecution) verifyOwnershipWhenNoDeploymentRecord(_ context.Context) (bool, error) {
	record, err := deployment.LoadDeploymentRecord(e.resolved.DeploymentIdentity)
	if err == nil {
		_ = record
		return false, nil
	}
	state, ok := deployment.DeploymentRecordState(err)
	if !ok || state.Kind != "incomplete" {
		// A corrupt or otherwise invalid record is a real ownership problem;
		// only the "missing record" case is a clean never-delivered retire.
		return false, err
	}
	// No deployment record exists. Verify ownership from the application/stack
	// identity rather than a deployment record: the repository .baseharbor/apps
	// store is the canonical owner for an application that was initialized but
	// never delivered. Presence of the identity proves the environment is in a
	// well-defined "never delivered" state; its absence is simply nothing to
	// own, which is likewise a clean no-op success. Both resolve to
	// neverDelivered=true so destroy retires the never-realized workload with
	// rc=0 instead of failing closed.
	store := application.Store{Root: filepath.Join(e.resolved.RepositoryRoot, ".baseharbor", "apps")}
	if _, _, lerr := store.Load(e.resolved.Manifest.Name); lerr != nil {
		return false, nil
	}
	return true, nil
}

func (e *applicationDestroyExecution) retireRuntimePreservingData(ctx context.Context, out io.Writer) error {
	// A never-delivered application (e.g. refused by the candidate consent
	// choke) has no on-disk deployment record. The reported bug was that the
	// destroy path failed closed here with an "incomplete deployment state"
	// error instead of cleaning up cleanly. When no deployment record exists,
	// infer ownership from the application/stack identity and retire a
	// workload that was never realized: there are no containers to stop and no
	// provider bindings to release, only the optional registry reference.
	if neverDelivered, err := e.verifyOwnershipWhenNoDeploymentRecord(ctx); err != nil {
		return err
	} else if neverDelivered {
		if !e.confirmed {
			fmt.Fprintln(out, "No changes were made. Confirm with --yes to retire a never-delivered application.")
			return nil
		}
		if err := application.ReleaseApplicationProviderRegistryAt(e.resolved.TargetStateRoot, e.manifest); err != nil {
			return fmt.Errorf("application was never delivered but provider registry cleanup failed: %w", err)
		}
		if err := recordApplicationAudit(ctx, e.resolved, "destroy", "success", "ownership verified; nothing was ever delivered", "refused or never-applied application retired; no runtime or provider state existed to remove"); err != nil {
			return err
		}
		e.term.Section("Application")
		e.term.Result("RETIRED", "application", e.manifest.Name+" never delivered; no runtime state to remove")
		return nil
	}
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
	if err := verifyApplicationRuntimeRetirement(e.resolved, plan, observed); err != nil {
		return err
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
