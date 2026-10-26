package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/machine"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

type selectedCoreDataDeletionKey struct{}

func withSelectedCoreDataDeletion(ctx context.Context, selected bool) context.Context {
	return context.WithValue(ctx, selectedCoreDataDeletionKey{}, selected)
}

type selectedCoreDestroyPlan struct {
	Impact     runtimeDestroyImpact
	Owner      controlPlaneStopPlan
	Containers []bhruntime.ProjectResource
	Volumes    []bhruntime.ProjectResource
	DeleteData bool
}

type selectedCoreDestroyProvider interface {
	bhruntime.RuntimeServiceStopProvider
	bhruntime.RuntimeServiceRetirementProvider
	DestroyOwnedProjectResources(context.Context, string, []bhruntime.ProjectResource) error
}

// Select only the canonical Core module. Another provider or app sharing its
// native project is neither the Core owner nor implicit permission to delete it.
func buildSelectedCoreDestroyPlan(ctx context.Context) (selectedCoreDestroyPlan, error) {
	p := selectedCoreDestroyPlan{}
	p.DeleteData, _ = ctx.Value(selectedCoreDataDeletionKey{}).(bool)
	core, err := buildControlPlaneStopPlanForOperation(ctx, "control-plane.destroy")
	if err != nil {
		return p, err
	}
	p.Owner = core
	impact := runtimeDestroyImpact{Core: []runtimeDestroyCoreImpact{}, Resources: []ownedDestroyResource{}, Preserved: []fullDestroyResult{}, LocalState: []application.ResourceLocation{}}
	functions := append([]application.FunctionConsequence{}, core.Functions...)
	for i := range functions {
		functions[i].Outcome = "unknown"
		functions[i].Behavior = "The selected Core SQL/OpenBao processes are removed even when this consumer still depends on them. Required SQL access, secret retrieval, leases, PKI issuance/renewal and dependent authentication or management may fail. Workloads, bindings and security policy remain; no replacement, unauthenticated fallback, or renewal continuity is claimed."
	}
	impact.Core = append(impact.Core, runtimeDestroyCoreImpact{Target: core.Target, Project: core.Project, Functions: functions, Materials: core.Materials, RuntimeObserved: core.RuntimeObserved, Processes: core.ObservedProcesses})
	for _, resource := range core.Resources {
		selected := resource.Kind == "container" || (resource.Kind == "volume" && p.DeleteData)
		if resource.Kind == "container" {
			p.Containers = append(p.Containers, resource)
		}
		if resource.Kind == "volume" {
			p.Volumes = append(p.Volumes, resource)
		}
		if selected {
			impact.Resources = append(impact.Resources, ownedDestroyResource{Target: core.Target, Project: core.Project, Kind: resource.Kind, Name: resource.Name})
		} else {
			impact.Preserved = append(impact.Preserved, fullDestroyResult{Status: "PRESERVED", Target: core.Target, Resource: resource.Kind + " " + resource.Name, Detail: "Not selected for deletion; existing module/recovery ownership remains. Data retention does not prove consumer continuity."})
		}
	}
	impact.Preserved = append(impact.Preserved, fullDestroyResult{Status: "PRESERVED", Target: core.Target, Resource: "Core configuration and consumer registrations", Detail: "Protected Core files, provider bindings, application registrations, TLS/OIDC/mTLS policy and host trust remain for explicit resume or retry. Retained keys/recovery cannot recreate deleted volume contents."})
	after := "Remove only canonical Core containers and their native startup units. Keep all volumes, networks, protected files, consumer registrations, other providers and application workloads. Explicit resume uses the existing owner and credentials; actual data access remains to be verified."
	if p.DeleteData {
		after = "Remove only canonical Core containers/startup units and explicitly selected owned Core volumes. Volume contents are permanently lost; retained credentials/keys cannot restore them. Keep networks, protected files, consumer registrations, other providers, application workloads and security policy. In-use volume removal may fail; retain owners for retry."
	}
	impact.Effects = []application.OperationalConsequence{{Aspect: "selected-core-destroy", Before: "The displayed canonical Core resources and actual registered consumers.", After: after, Evidence: "canonical Core owner plus existing selected-stop/retirement and owned-resource removal authorities"}}
	impact.Scope, err = application.ComposeProviderImpactScope(application.ProviderScopeOwner{Owner: "control-plane.destroy", Reference: core.inventory.Files.Compose, Subject: core.Project, Destructive: p.DeleteData, Revision: runtimeMutationRevision(struct {
		Impact     runtimeDestroyImpact
		Owners     map[string]string
		Native     string
		DeleteData bool
	}{impact, core.protected, core.nativeRevision, p.DeleteData})}, nil, false)
	p.Impact = impact
	return p, err
}

func destroySelectedControlPlane(parent context.Context, confirmed bool, out io.Writer) error {
	if err := authorizeCurrentMCPContext(parent, "control-plane.destroy", "", "", ""); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(parent, 5*time.Minute)
	defer cancel()
	target, err := effectiveTarget(ctx)
	if err != nil {
		return err
	}
	root, err := targetDataRoot(target)
	if err != nil {
		return err
	}
	if _, err := os.Lstat(root); err != nil {
		return err
	}
	unlock, err := lockApplicationTargetLifecycle(resolvedApplication{Target: target, TargetStateRoot: root})
	if err != nil {
		return err
	}
	defer unlock()
	p, err := buildSelectedCoreDestroyPlan(ctx)
	if err != nil {
		return err
	}
	if report, ok := ctx.Value(destroyReportContextKey{}).(*destroyReport); ok {
		report.Resources, report.Preserved = p.Impact.Resources, p.Impact.Preserved
	}
	proceed, err := authorizeRuntimeDestroyImpact(ctx, p.Impact, confirmed, out)
	if err != nil || !proceed {
		return err
	}
	provider, err := detectRuntimeForTarget(ctx, target)
	if err != nil {
		return err
	}
	native, ok := provider.(selectedCoreDestroyProvider)
	if !ok {
		return machine.NewError(machine.ErrorUnsupported, "The selected runtime does not implement owned Core container retirement.", "Use a supported runtime adapter; Core state is preserved.", false)
	}
	current, err := buildSelectedCoreDestroyPlan(ctx)
	if err != nil {
		return err
	}
	if current.Owner.targetBinding != target || current.Impact.Scope.PlanID != p.Impact.Scope.PlanID {
		return runtimeDestroyImpactError(current.Impact, &application.ProviderChangeError{Code: "stale-impact-consent"})
	}
	results, err := executeSelectedCoreDestroy(ctx, p, native)
	recordDestroyResults(ctx, results, err == nil)
	owner := application.ProviderScopeOwnerOutcome{Owner: p.Impact.Scope.Owner, Disposition: "unknown", Evidence: "Protected owners require read-back; consumer functions remain unverified."}
	if err == nil {
		owner.Disposition, owner.Verified, owner.Evidence = "retained", true, "Read-back confirms original Core files and consumer registrations are unchanged. Native retirement/data disposition is verified separately; consumer functions remain partially verified."
	}
	receipt, receiptErr := application.RecordProviderImpactScopeResult(p.Impact.Scope, owner, nil)
	if report, ok := ctx.Value(destroyReportContextKey{}).(*destroyReport); ok {
		report.ProviderChangeResult = &receipt
	}
	err = errors.Join(err, receiptErr)
	renderFullDestroyReport(out, results)
	if err != nil {
		return &machine.Error{Code: machine.ErrorConflict, CauseCode: "control_plane_destroy_partial", Message: "Selected Core cleanup is incomplete. Protected owners and consumer registrations remain for a fresh plan and retry.", Details: results, Cause: err}
	}
	fmt.Fprintln(out, "Selected Core runtime removed. Consumer consequences remain partially verified; use the retained owners for explicit resume or retry.")
	return nil
}

func executeSelectedCoreDestroy(ctx context.Context, p selectedCoreDestroyPlan, provider selectedCoreDestroyProvider) ([]fullDestroyResult, error) {
	results := append([]fullDestroyResult{}, p.Impact.Preserved...)
	before, err := provider.InspectProjectResources(ctx, p.Owner.Project, p.Owner.Resources)
	if err != nil {
		return results, err
	}
	baseline := map[bhruntime.ProjectResource]bool{}
	for _, resource := range before {
		baseline[resource] = true
	}
	environment, err := bhruntime.RuntimeEnvironment(p.Owner.inventory.Files)
	if err != nil {
		return results, runtimeMutationOwnerError()
	}
	stopped, err := bhruntime.StopOwnedProjectServices(ctx, provider, p.Owner.Project, filepath.Dir(p.Owner.inventory.Files.Compose), environment, p.Owner.Resources, p.Owner.Services, p.Owner.inventory.Files.Compose)
	if err != nil || !stopped.Verified {
		return results, errors.Join(err, errors.New("Core processes were not verified stopped; no volume deletion attempted"))
	}
	retired, err := bhruntime.RetireOwnedServices(ctx, provider, p.Owner.Project, p.Owner.Resources, p.Containers)
	for _, resource := range retired.Removed {
		results = append(results, fullDestroyResult{Status: "REMOVED", Target: p.Owner.Target, Resource: resource.Kind + " " + resource.Name})
	}
	if err != nil {
		return results, err
	}
	if p.DeleteData {
		if err := provider.DestroyOwnedProjectResources(ctx, p.Owner.Project, p.Volumes); err != nil {
			return results, err
		}
	}
	actual, err := provider.InspectProjectResources(ctx, p.Owner.Project, p.Owner.Resources)
	if err != nil {
		return results, err
	}
	present := map[bhruntime.ProjectResource]bool{}
	for _, resource := range actual {
		present[resource] = true
	}
	for _, resource := range p.Owner.Resources {
		removed := resource.Kind == "container" || (resource.Kind == "volume" && p.DeleteData)
		if (removed && present[resource]) || (!removed && baseline[resource] && !present[resource]) {
			return results, errors.New("selected Core resource disposition could not be verified; preserve owner state")
		}
		if removed && resource.Kind == "volume" {
			status, detail := "NOT FOUND", "Owned volume was already absent; no new deletion or data recovery is claimed."
			if baseline[resource] {
				status, detail = "REMOVED", "Owned volume contents permanently removed; retained credentials do not recover data."
			}
			results = append(results, fullDestroyResult{Status: status, Target: p.Owner.Target, Resource: "volume " + resource.Name, Detail: detail})
		}
	}
	for path, revision := range p.Owner.protected {
		data, readErr := runtimeMutationOwnerBytes(path, true)
		unchanged := readErr == nil && runtimeMutationRevision(data) == revision
		zeroBytes(data)
		if !unchanged {
			return results, runtimeMutationOwnerError()
		}
	}
	return results, nil
}
