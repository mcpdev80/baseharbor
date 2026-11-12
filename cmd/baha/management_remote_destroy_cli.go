package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
)

func remoteApplicationDestroyCLI(ctx context.Context, target deployment.ResolvedTarget, args []string, out, errOut io.Writer) error {
	filtered, format, err := parseReadOutputArgs(args, "app destroy")
	if err != nil {
		return err
	}
	if format == outputJSON {
		ctx = machineNoninteractiveContext(ctx)
	}
	for _, arg := range filtered {
		if arg == "--yes" || arg == "-y" {
			ctx = withAssumeYes(ctx, true)
		}
	}
	reader := bufio.NewReader(applicationInput(ctx, runtimeInput))
	seen := map[string]bool{}
	for attempt := 0; attempt < 3; attempt++ {
		var result bytes.Buffer
		requestErr := remoteApplicationMutation(ctx, target, "destroy", filtered, &result)
		if requestErr == nil {
			refreshWorkstationHosts(ctx, target, errOut)
			if format == outputJSON {
				_, err = io.Copy(out, &result)
				return err
			}
			return renderRemoteDestroyCompletion(out, result.Bytes())
		}
		next, retry, approvalErr := acknowledgeRemoteDestroyChallenge(ctx, target, requestErr, reader, out, seen)
		if approvalErr != nil {
			return approvalErr
		}
		if !retry {
			return requestErr
		}
		ctx = next
	}
	return errors.New("Core requires further authorization; inspect the current provider plan before retrying")
}

func acknowledgeRemoteDestroyChallenge(ctx context.Context, target deployment.ResolvedTarget, cause error, reader *bufio.Reader, out io.Writer, seen map[string]bool) (context.Context, bool, error) {
	var challenge *machine.Error
	if !errors.As(cause, &challenge) || challenge.Code != machine.ErrorApprovalRequired ||
		(challenge.CauseCode != "impact-confirmation-required" && challenge.CauseCode != "data-deletion-consent-required") {
		return ctx, false, nil
	}
	data, err := json.Marshal(challenge.Details)
	if err != nil {
		return ctx, false, err
	}
	var plan application.ProviderImpactPlan
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&plan) != nil || plan.Version != application.ProviderImpactVersion || plan.PlanID == "" || plan.Target == "" || len(plan.Impacts) == 0 || plan.ConfirmationsRequired < 1 || plan.ConfirmationsRequired > 2 || seen[plan.PlanID] {
		return ctx, false, errors.New("Core returned an invalid or repeated provider removal plan; inspect it before retrying")
	}
	core, err := dialVerifiedRemoteCore(ctx, target.Name, target.RuntimeProvider, target.TenantID)
	if err != nil {
		return ctx, false, err
	}
	if plan.Target != core.serverTargetName() {
		return ctx, false, errors.New("provider removal plan belongs to another Core Target")
	}
	plan.Target = target.Name
	if noInput(ctx) {
		return ctx, false, providerImpactConsentError(plan, application.ValidateProviderChangeConsent(plan, application.ProviderChangeConsent{PlanID: plan.PlanID}))
	}
	if err := printProviderImpactForCLI(ctx, out, plan); err != nil {
		return ctx, false, err
	}
	if !readerIsTerminal(reader) {
		return ctx, false, providerImpactConsentError(plan, application.ValidateProviderChangeConsent(plan, application.ProviderChangeConsent{PlanID: plan.PlanID}))
	}
	accepted, err := promptYesNo(reader, out, "Remove this application and apply the listed provider changes?", false)
	if err != nil {
		if errors.Is(err, io.EOF) {
			fmt.Fprintln(out, "Removal cancelled; nothing was changed.")
			return ctx, false, cli.Presented(err)
		}
		return ctx, false, err
	}
	if !accepted {
		fmt.Fprintln(out, "Removal cancelled; nothing was changed.")
		return ctx, false, cli.Presented(errors.New("application removal cancelled"))
	}
	consent := application.ProviderChangeConsent{PlanID: plan.PlanID, First: true, Second: plan.ConfirmationsRequired == 2}
	if plan.Destructive {
		deleteData, err := promptYesNo(reader, out, "Permanently delete the listed application data?", false)
		if err != nil {
			return ctx, false, err
		}
		consent.DeleteData = deleteData
		if !deleteData {
			fmt.Fprintln(out, "Removal cancelled; no resources were changed.")
			return ctx, false, cli.Presented(errors.New("application data deletion was not authorized"))
		}
	}
	if err := application.ValidateProviderChangeConsent(plan, consent); err != nil {
		return ctx, false, providerImpactConsentError(plan, err)
	}
	seen[plan.PlanID] = true
	return withProviderChangeConsent(ctx, &consent), true, nil
}

func renderRemoteDestroyCompletion(out io.Writer, raw []byte) error {
	var receipt struct {
		Application string `json:"application"`
		Environment string `json:"environment"`
		Target      string `json:"target"`
		Verified    bool   `json:"verified"`
		State       string `json:"state"`
		DataDeleted *bool  `json:"data_deleted"`
	}
	if json.Unmarshal(raw, &receipt) != nil || !receipt.Verified || receipt.Application == "" || receipt.Target == "" {
		return errors.New("remote Core destroy receipt is invalid")
	}
	fmt.Fprintf(out, "Application %s on %s (%s) removed.\n", receipt.Application, receipt.Target, receipt.Environment)
	if receipt.DataDeleted != nil && *receipt.DataDeleted {
		fmt.Fprintln(out, "Application data was permanently deleted as requested.")
	} else {
		fmt.Fprintln(out, "Persistent data and recovery material were retained.")
	}
	fmt.Fprintln(out, "Next: baha app status")
	return nil
}
