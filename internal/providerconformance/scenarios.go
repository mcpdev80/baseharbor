package providerconformance

import (
	"context"
	"fmt"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/reconciliation"
)

// FaultFixture is implemented by disposable fixtures, not production drivers.
// It injects a failure at the selected phase and restores normal behavior with
// FailureNone. State fingerprints exclude injected controls and counters.
type FaultFixture interface{ SetFailure(FailureMode) }
type OwnershipFixture interface {
	SetReconciliationOwnership(reconciliation.Ownership)
}

// RunScenarios extends the original lifecycle harness with the same failure,
// recovery and ownership checks used by the in-tree provider proof. It never
// invokes a shell or assumes a particular provider deployment technology.
func RunScenarios(ctx context.Context, target Target) Report {
	report := Run(ctx, target)
	add := func(name string, err error) {
		c := Check{Name: name, Status: Pass}
		if err != nil {
			c.Status = Fail
			c.Message = err.Error()
			report.Status = Fail
		}
		report.Checks = append(report.Checks, c)
	}
	state, stateOK := target.Request.Driver.(StateDigester)
	drift, driftOK := target.Request.Driver.(Drifter)
	destroy, destroyOK := target.Request.Driver.(Destroyer)
	faults, faultsOK := target.Request.Driver.(FaultFixture)
	ownership, ownershipOK := target.Request.Driver.(OwnershipFixture)
	_, reconcileOK := target.Request.Driver.(capability.ReconciliationDriver)
	if !stateOK || !driftOK || !destroyOK || !faultsOK || !ownershipOK || !reconcileOK {
		add("scenario-fixture-hooks", fmt.Errorf("state, drift, destroy, fault, ownership and typed reconciliation hooks are required"))
		return report
	}
	if report.Status == Fail {
		return report
	}
	add("scenario-fixture-hooks", nil)
	stable := state.ConformanceStateDigest()
	drift.ConformanceDrift()
	if state.ConformanceStateDigest() == stable {
		add("drift-injection", fmt.Errorf("fixture did not introduce observable drift"))
		return report
	}
	add("drift-injection", nil)
	result, err := converge(ctx, target)
	if err == nil && result.Status != capability.StatusReady {
		err = fmt.Errorf("repair did not become ready")
	}
	if err == nil && state.ConformanceStateDigest() != stable {
		err = fmt.Errorf("repair changed stable identity or state")
	}
	add("drift-repair-stable-identity", err)
	if err != nil {
		return report
	}

	// Preflight outage must refuse execution without any resource mutation.
	faults.SetFailure(FailureUnavailable)
	before := state.ConformanceStateDigest()
	execution, _, err := capability.Prepare(ctx, target.Application, []capability.Request{target.Request})
	if err == nil || execution != nil || state.ConformanceStateDigest() != before {
		add("outage-fails-without-mutation", fmt.Errorf("outage was accepted or changed state"))
	} else {
		add("outage-fails-without-mutation", nil)
	}
	faults.SetFailure(FailureNone)
	if _, err := converge(ctx, target); err != nil {
		add("outage-recovery", err)
	} else {
		add("outage-recovery", nil)
	}

	resource := capability.Resource{Application: target.Application, Kind: target.Request.Requirement.Kind, Name: target.Request.Requirement.Name, Provider: target.Request.Driver.Descriptor().Kind}
	// Force a fresh owned resource for partial-bind/provision failure proofs;
	// a stable typed NOOP must never call the provider mutation phase.
	for _, mode := range []FailureMode{FailureProvision, FailureMalformedBinding, FailureVerify} {
		if err := destroy.ConformanceDestroy(ctx, resource); err != nil {
			add("fixture-reset", err)
			return report
		}
		faults.SetFailure(mode)
		result, err := converge(ctx, target)
		if err == nil || result.Status == capability.StatusReady {
			add(string(mode)+"-fails-closed", fmt.Errorf("injected failure reported success"))
		} else {
			add(string(mode)+"-fails-closed", nil)
		}
		faults.SetFailure(FailureNone)
		result, err = converge(ctx, target)
		if err == nil && result.Status != capability.StatusReady {
			err = fmt.Errorf("retry did not become ready")
		}
		add(string(mode)+"-retry", err)
		if err != nil {
			return report
		}
		stable = state.ConformanceStateDigest()
		_, err = converge(ctx, target)
		if err == nil && state.ConformanceStateDigest() != stable {
			err = fmt.Errorf("retry changed converged identity")
		}
		add(string(mode)+"-retry-idempotent", err)
	}

	drift.ConformanceDrift()
	ownership.SetReconciliationOwnership(reconciliation.OwnershipForeign)
	before = state.ConformanceStateDigest()
	execution, result, err = capability.Prepare(ctx, target.Application, []capability.Request{target.Request})
	if err == nil || execution != nil || len(result.Reconciliation) != 1 || result.Reconciliation[0].Result.State != reconciliation.StateForeignOwnership || state.ConformanceStateDigest() != before {
		add("foreign-ownership-blocks-before-mutation", fmt.Errorf("foreign-owned resource was accepted or mutated"))
	} else {
		add("foreign-ownership-blocks-before-mutation", nil)
	}
	ownership.SetReconciliationOwnership(reconciliation.OwnershipBaseHarbor)
	if _, err := converge(ctx, target); err != nil {
		add("ownership-fixture-recovery", err)
		return report
	}

	before = state.ConformanceStateDigest()
	sibling := resource
	sibling.Application += "-sibling"
	if err := destroy.ConformanceDestroy(ctx, sibling); err == nil || state.ConformanceStateDigest() != before {
		add("destroy-refuses-sibling", fmt.Errorf("foreign destroy was accepted or mutated state"))
	} else {
		add("destroy-refuses-sibling", nil)
	}
	err = destroy.ConformanceDestroy(ctx, resource)
	if err == nil && state.ConformanceStateDigest() == before {
		err = fmt.Errorf("owned destroy left resource state unchanged")
	}
	add("destroy-owned-resource", err)
	if err != nil {
		return report
	}
	before = state.ConformanceStateDigest()
	err = destroy.ConformanceDestroy(ctx, resource)
	if err == nil && state.ConformanceStateDigest() != before {
		err = fmt.Errorf("repeated destroy changed state")
	}
	add("destroy-idempotent", err)
	return report
}

func converge(ctx context.Context, target Target) (capability.Result, error) {
	execution, result, err := capability.Prepare(ctx, target.Application, []capability.Request{target.Request})
	if err != nil {
		return result, err
	}
	result, err = execution.ProvisionAndBind(ctx)
	if err != nil {
		return result, err
	}
	return execution.Verify(ctx)
}
