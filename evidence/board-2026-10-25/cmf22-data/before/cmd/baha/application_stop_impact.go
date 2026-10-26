package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
)

// Stopping preserves desired intent and bindings. It is not last-consumer
// withdrawal: shared providers remain requested for a later application up.
type applicationStopPlan struct {
	Scope           application.ProviderImpactScope   `json:"scope"`
	Functions       []application.FunctionConsequence `json:"functions"`
	Materials       []application.MaterialConsequence `json:"materials"`
	protected       map[string]string
	appliedRevision string
}

func buildApplicationStopPlan(ctx context.Context, resolved resolvedApplication) (applicationStopPlan, error) {
	p := applicationStopPlan{}
	target, err := effectiveTarget(withTargetOverride(ctx, resolved.Target.Name))
	if err != nil {
		return p, err
	}
	if target != resolved.Target {
		return p, classifyProviderChangeConsent(&application.ProviderChangeError{Code: "stale-impact-consent"})
	}
	prior, err := retainedApplicationBackendIntent(resolved)
	if err != nil {
		return p, err
	}
	applied := resolved
	applied.Manifest = prior
	snapshot, preferences, err := applicationLifecycleSnapshot(applied, observeApplicationProviderFacts(ctx, applied))
	if err != nil {
		return p, err
	}
	union, err := resolveApplicationProviderPlanFromSnapshot(applied, snapshot, preferences)
	if err != nil {
		return p, err
	}
	requests := []application.ProviderChangeRequest{}
	for _, instance := range snapshot.Registry.Instances {
		if instance.Ownership == capability.OwnershipBaseHarbor && instance.Scope == capability.ScopeApplication && instance.OwnerApplicationID == prior.ApplicationID && instance.OwnerEnvironment == prior.Environment {
			switch instance.Provider.Kind {
			case capability.ProviderPostgreSQL, capability.ProviderValkey, capability.ProviderMongoDB, capability.ProviderRabbitMQ, capability.ProviderPrometheus, capability.ProviderLoki, capability.ProviderTempo, capability.ProviderCaddy, capability.ProviderKeycloak:
			default:
				continue // This command does not operate unselected standalone providers.
			}
			requests = append(requests, application.ProviderChangeRequest{InstanceID: instance.ID, Operation: "stop", Selection: "explicit", Consumers: []string{}, Materials: []application.MaterialDisposition{}})
		}
	}
	impact, err := resolveApplicationProviderImpact(ctx, applied, snapshot, union, requests)
	if err != nil {
		return p, err
	}
	root, err := deployment.DeploymentRoot(resolved.DeploymentIdentity)
	if err != nil {
		return p, err
	}
	owners := map[string]string{}
	record, err := deployment.LoadDeploymentRecord(resolved.DeploymentIdentity)
	if err != nil || record.Identity != resolved.DeploymentIdentity {
		return p, runtimeMutationOwnerError()
	}
	p.appliedRevision = runtimeMutationRevision(record.Applied)
	configPath, err := deployment.ConfigPath()
	if err != nil {
		return p, err
	}
	paths := []string{configPath, filepath.Join(resolved.TargetStateRoot, "provider-registry.json"), filepath.Join(root, "deployment.json")}
	records, err := deployment.ListDeployments(resolved.Target.Name)
	if err != nil {
		return p, err
	}
	for _, record := range records {
		other, err := deployment.DeploymentRoot(record.Identity)
		if err != nil {
			return p, err
		}
		if other != root {
			paths = append(paths, filepath.Join(other, "deployment.json"))
		}
	}
	for _, path := range paths {
		data, err := runtimeMutationOwnerBytes(path, path != configPath && path != filepath.Join(root, "deployment.json"))
		if err != nil {
			return p, err
		}
		owners[path] = runtimeMutationRevision(data)
		if data != nil {
			p.Materials = append(p.Materials, runtimeRetainedOwnerMaterial(resolved.Target.Name, filepath.Base(path), path))
		}
	}
	p.protected = owners
	p.Functions = []application.FunctionConsequence{{Consumer: "application/" + prior.ApplicationID + "/" + prior.Environment, Function: "workload-availability", Outcome: "unavailable", Required: true, Via: []string{resolved.DeploymentIdentity.DeploymentID}, Behavior: "Application workload, its broker and the explicitly listed application-scoped provider realizations stop. Desired intent and bindings remain for explicit up. Shared and unselected standalone providers are retained. In-flight work, sessions, reconnection and transient-state effects are unverified; retained data does not prove continued application access.", Evidence: "existing applied deployment intent and resolved consumer union"}}
	p.Scope, err = application.ComposeProviderImpactScope(application.ProviderScopeOwner{Owner: "application-deployment", Reference: filepath.Join(root, "deployment.json"), Subject: resolved.DeploymentIdentity.DeploymentID, Revision: runtimeMutationRevision(struct {
		Owners map[string]string
		Plan   applicationStopPlan
	}{owners, p})}, []application.ProviderImpactPlan{impact}, false)
	return p, err
}

func stopApplicationRuntime(ctx context.Context, resolved resolvedApplication, out, errOut io.Writer) error {
	_, err := stopApplicationRuntimeWithResult(ctx, resolved, out, errOut)
	return err
}

func stopApplicationRuntimeWithResult(ctx context.Context, resolved resolvedApplication, out, errOut io.Writer) (applicationStopResult, error) {
	result := applicationStopResult{Application: resolved.Manifest.Name, Environment: resolved.Manifest.Environment, State: "unknown"}
	if err := authorizeApplicationOperation(ctx, "app.stop", resolved); err != nil {
		return result, err
	}
	unlock, err := lockApplicationTargetLifecycle(resolved)
	if err != nil {
		return result, err
	}
	defer unlock()
	plan, err := authorizeApplicationStop(ctx, resolved, out)
	if err != nil {
		return result, err
	}
	// A pending source edit does not change the runtime being stopped.
	prior, err := retainedApplicationBackendIntent(resolved)
	if err != nil {
		return result, err
	}
	resolved.Manifest = prior
	operationErr := executeApplicationStop(ctx, resolved, out, errOut)
	owner := application.ProviderScopeOwnerOutcome{Owner: plan.Scope.Owner, Disposition: "unknown", Evidence: "Post-operation recovery ownership is unverified."}
	verified := true
	for path, revision := range plan.protected {
		if path == plan.Scope.Owner.Reference {
			continue // The existing observed lifecycle state is intentionally updated.
		}
		data, err := runtimeMutationOwnerBytes(path, true)
		verified = verified && err == nil && runtimeMutationRevision(data) == revision
	}
	_, readErr := runtimeMutationOwnerBytes(plan.Scope.Owner.Reference, false)
	record, recordErr := deployment.LoadDeploymentRecord(resolved.DeploymentIdentity)
	verified = verified && readErr == nil && recordErr == nil && record.Identity == resolved.DeploymentIdentity && runtimeMutationRevision(record.Applied) == plan.appliedRevision
	if verified {
		owner.Disposition, owner.Verified = "retained", true
		owner.Evidence = "Existing recovery owners, provider bindings and applied intent are preserved; only observed lifecycle state may change."
	}
	receipt, receiptErr := application.RecordProviderImpactScopeResult(plan.Scope, owner, nil)
	result.ProviderChangeResult = &receipt
	if operationErr == nil && verified && record.Observed.State == "stopped" && !record.Observed.Ready {
		result.State, result.DataPreserved = "stopped", true
	}
	if operationErr != nil || !verified || receiptErr != nil || result.State != "stopped" {
		return result, &machine.Error{Code: machine.ErrorConflict, CauseCode: "application_stop_partial", Message: "Application stop is incomplete; inspect actual runtime and retained recovery ownership before retrying.", Details: result, Cause: errors.Join(operationErr, receiptErr, recordErr, readErr)}
	}
	return result, nil
}

func authorizeApplicationStop(ctx context.Context, resolved resolvedApplication, out io.Writer) (applicationStopPlan, error) {
	plan, err := buildApplicationStopPlan(ctx, resolved)
	if err != nil {
		return plan, err
	}
	consent, err := invocationProviderChangeConsent(ctx)
	if err != nil {
		return plan, err
	}
	if out == nil {
		out = io.Discard
	}
	fmt.Fprintln(out, "Achtung: review the current application stop, affected consumers and retained recovery locations.")
	if err := writeJSON(out, plan); err != nil {
		return plan, err
	}
	in := applicationInput(ctx, nil)
	if consent == nil && !noInput(ctx) && in != nil && readerIsTerminal(in) {
		presentation := application.ProviderImpactPlan{PlanID: plan.Scope.PlanID, ConfirmationsRequired: plan.Scope.ConfirmationsRequired}
		value, err := readProviderImpactAcknowledgements(bufio.NewReader(in), out, presentation)
		if err != nil {
			return plan, err
		}
		consent = &value
	}
	ack := application.ProviderChangeConsent{PlanID: plan.Scope.PlanID}
	if consent != nil {
		ack = *consent
	}
	if err := application.ValidateProviderImpactScopeConsent(plan.Scope, ack); err != nil {
		return plan, applicationStopPlanError(plan, err)
	}
	current, err := buildApplicationStopPlan(ctx, resolved)
	if err != nil {
		return plan, err
	}
	if current.Scope.PlanID != plan.Scope.PlanID {
		return current, applicationStopPlanError(current, &application.ProviderChangeError{Code: "stale-impact-consent"})
	}
	return plan, nil
}

func applicationStopPlanError(plan applicationStopPlan, err error) error {
	classified := classifyProviderChangeConsent(err)
	var failure *machine.Error
	if errors.As(classified, &failure) {
		failure.Details = plan
	}
	return classified
}
