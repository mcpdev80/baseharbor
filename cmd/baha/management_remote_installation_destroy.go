package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/targetaccess"
)

// destroyConnectedRemoteInstallation runs the remote Core's authoritative
// installation destroy first, then runs the native local path with remote
// targets excluded. This preserves local runtime cleanup while ensuring the
// workstation never attempts to inspect a remote Docker/Podman socket.
func destroyConnectedRemoteInstallation(ctx context.Context, confirmed bool, out, errOut io.Writer) (bool, error) {
	targets, discovery := discoverFullDestroyTargets(ctx)
	if countFullDestroyBlockers(discovery) > 0 {
		return true, errors.New("cannot safely discover connected Targets; no resources were removed")
	}
	var remote []deployment.ResolvedTarget
	for _, target := range targets {
		if target.AccessProvider == string(targetaccess.ProviderCoreRemote) {
			remote = append(remote, target)
		}
	}
	if len(remote) == 0 {
		return false, nil
	}
	if len(remote) > 1 {
		return true, errors.New("full destroy found multiple remote Core installations; no resources were removed because each needs a separately reviewed plan")
	}
	target := remote[0]
	if !confirmed {
		if noInput(ctx) || !readerIsTerminal(applicationInput(ctx, nil)) {
			return true, usageError("remote installation destroy requires --yes in non-interactive mode", "Run baha destroy --yes; the current remote provider plan and data-deletion consent will still be required.")
		}
		confirmed = true
	}
	fmt.Fprintf(out, "WARNING: permanently remove the BaseHarbor installation on remote Target %s and clean this workstation. Application source repositories and external infrastructure are preserved.\n", target.Name)
	localCtx, proceed, err := preflightLocalInstallationDestroy(ctx, confirmed, out)
	if err != nil {
		return true, err
	}
	if !proceed {
		return true, nil
	}
	if err := remoteInstallationDestroy(ctx, target, out); err != nil {
		return true, err
	}
	if err := cleanupWorkstationConveniences(ctx, target.Name, out); err != nil {
		return true, fmt.Errorf("remote Core was destroyed, but workstation host/CA cleanup is incomplete: %w", err)
	}
	if err := destroyInstallation(localCtx, confirmed, out, errOut); err != nil {
		return true, fmt.Errorf("remote Core was destroyed; local installation cleanup still needs attention: %w", err)
	}
	return true, nil
}

func preflightLocalInstallationDestroy(ctx context.Context, confirmed bool, out io.Writer) (context.Context, bool, error) {
	localCtx := context.WithValue(ctx, fullDestroyRemotePhaseKey{}, true)
	localCtx = withFullDestroyExcludeRemoteTargets(localCtx)
	localCtx = withTargetOverride(localCtx, "local")
	inventory, err := prepareFullDestroyInventory(localCtx)
	if err != nil {
		return localCtx, false, err
	}
	impact, err := buildRuntimeDestroyImpact(localCtx, inventory.plans, inventory.preserved, true)
	if err != nil {
		return localCtx, false, err
	}
	ack := &application.ProviderChangeConsent{}
	localCtx = context.WithValue(localCtx, runtimeDestroyAcknowledgementKey{}, ack)
	proceed, err := authorizeRuntimeDestroyImpact(localCtx, impact, confirmed, out)
	if err != nil || !proceed {
		return localCtx, proceed, err
	}
	return withProviderChangeConsent(localCtx, ack), true, nil
}

func remoteInstallationDestroy(ctx context.Context, target deployment.ResolvedTarget, out io.Writer) error {
	core, err := dialVerifiedRemoteCore(ctx, target.Name, target.RuntimeProvider, target.TenantID)
	if err != nil {
		return fmt.Errorf("verify remote Core for Target %s: %w", target.Name, err)
	}
	input := applicationInput(ctx, nil)
	interactive := !noInput(ctx) && readerIsTerminal(input)
	preview, err := sendRemoteInstallationDestroy(ctx, core, target, false, nil)
	if err != nil {
		return err
	}
	if preview.State != machine.ExecutionSucceeded {
		if preview.Error != nil {
			return preview.Error
		}
		return errors.New("remote Core returned an invalid installation destroy preview")
	}
	var previewReport destroyReport
	if json.Unmarshal(preview.Result, &previewReport) != nil || !previewReport.Preview || previewReport.Destroyed || !previewReport.All || previewReport.Impact == nil {
		return errors.New("remote Core did not return a complete, non-mutating installation destroy preview")
	}
	plan := *previewReport.Impact
	if report, ok := ctx.Value(destroyReportContextKey{}).(*destroyReport); ok {
		if report.Impact == nil {
			report.Impact = &plan
		} else {
			report.AdditionalPlans = append(report.AdditionalPlans, plan)
		}
	}
	if out != io.Discard {
		pretty, _ := json.MarshalIndent(plan, "", "  ")
		fmt.Fprintln(out, "\nRemote installation impact plan:")
		fmt.Fprintln(out, string(pretty))
	}
	acknowledged, err := invocationProviderChangeConsent(ctx)
	if err != nil {
		return err
	}
	if acknowledged == nil && !interactive {
		failure := machine.NewError(machine.ErrorApprovalRequired, "Remote installation destroy needs current-plan acknowledgements.", "Review the returned provider_change_plan and repeat the operation interactively; plan details do not grant approval.", false)
		failure.Details = plan
		return failure
	}
	if acknowledged == nil {
		value, err := readProviderImpactAcknowledgements(bufio.NewReader(input), out, application.ProviderImpactPlan{
			PlanID: plan.Scope.PlanID, ConfirmationsRequired: plan.Scope.ConfirmationsRequired, Destructive: plan.Scope.Destructive,
		})
		if err != nil {
			return err
		}
		acknowledged = &value
	} else if err := application.ValidateProviderImpactScopeConsent(plan.Scope, *acknowledged); err != nil {
		return runtimeDestroyImpactError(plan, err)
	}
	if !acknowledged.First || plan.Scope.ConfirmationsRequired > 1 && !acknowledged.Second || plan.Scope.Destructive && !acknowledged.DeleteData {
		return errors.New("installation destroy was not fully acknowledged; no resources were removed")
	}
	execution, err := sendRemoteInstallationDestroy(ctx, core, target, true, acknowledged)
	if err != nil {
		return err
	}
	if execution.State != machine.ExecutionSucceeded {
		if execution.Error != nil {
			return execution.Error
		}
		return errors.New("remote Core returned an invalid installation destroy receipt")
	}
	var report destroyReport
	if json.Unmarshal(execution.Result, &report) != nil || !report.Destroyed || report.Preview || !report.All {
		return errors.New("remote Core did not return a verified completed installation destroy report")
	}
	if combined, ok := ctx.Value(destroyReportContextKey{}).(*destroyReport); ok {
		combined.Resources = append(combined.Resources, report.Resources...)
		combined.Preserved = append(combined.Preserved, report.Preserved...)
		combined.Results = append(combined.Results, report.Results...)
	}
	if out != io.Discard {
		renderFullDestroyReport(out, report.Results)
		fmt.Fprintf(out, "Remote Core installation on %s was permanently removed.\n", target.Name)
	}
	return nil
}

func sendRemoteInstallationDestroy(ctx context.Context, core *verifiedRemoteCore, target deployment.ResolvedTarget, approval bool, consent *application.ProviderChangeConsent) (machine.Execution, error) {
	payload, err := json.Marshal(struct {
		Approval              bool                               `json:"approval"`
		ProviderChangeConsent *application.ProviderChangeConsent `json:"provider_change_consent,omitempty"`
	}{approval, consent})
	if err != nil {
		return machine.Execution{}, err
	}
	var requestID [16]byte
	if _, err := rand.Read(requestID[:]); err != nil {
		return machine.Execution{}, fmt.Errorf("create remote destroy request identity: %w", err)
	}
	execution, err := core.requestMachineExecutionTerminal(ctx, "installation.destroy", machine.OperationContext{
		Target: target.Name, Environment: "dev",
	}, payload, hex.EncodeToString(requestID[:]))
	if err != nil {
		return machine.Execution{}, fmt.Errorf("remote installation destroy result is unknown; inspect the Core before retrying: %w", err)
	}
	return execution, nil
}
