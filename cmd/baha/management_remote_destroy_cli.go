package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
)

func remoteApplicationDestroyCLI(ctx context.Context, target deployment.ResolvedTarget, args []string, out, errOut io.Writer) error {
	preserveData := isApplicationStopMode(ctx)
	operation := "destroy"
	commandName := "app destroy"
	if preserveData {
		operation = "app.stop"
		commandName = "app stop"
	}
	filtered, format, err := parseReadOutputArgs(args, commandName)
	if err != nil {
		return err
	}
	if format == outputJSON {
		ctx = machineNoninteractiveContext(ctx)
	}
	if preserveData {
		approved := false
		for _, arg := range filtered {
			approved = approved || arg == "--yes" || arg == "-y"
		}
		if !approved {
			return usageError("remote application stop requires explicit --yes approval", "Run baha app stop NAME --yes; the current provider plan will still require its own acknowledgement.")
		}
	}
	for _, arg := range filtered {
		if arg == "--yes" || arg == "-y" {
			ctx = withAssumeYes(ctx, true)
		}
	}
	reader := bufio.NewReader(applicationInput(ctx, runtimeInput))
	seen := map[string]bool{}
	requestArgs := filtered
	if preserveData {
		requestArgs = make([]string, 0, len(filtered))
		for _, arg := range filtered {
			if arg != "--yes" && arg != "-y" {
				requestArgs = append(requestArgs, arg)
			}
		}
	}
	for attempt := 0; attempt < 3; attempt++ {
		var result bytes.Buffer
		requestErr := remoteApplicationMutation(ctx, target, operation, requestArgs, &result)
		if requestErr == nil {
			refreshWorkstationHosts(ctx, target, errOut)
			if format == outputJSON {
				_, err = io.Copy(out, &result)
				return err
			}
			return renderRemoteDestroyCompletion(out, result.Bytes(), preserveData)
		}
		next, retry, approvalErr := acknowledgeRemoteDestroyChallenge(ctx, target, requestErr, reader, out, seen, !preserveData)
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

func acknowledgeRemoteDestroyChallenge(ctx context.Context, target deployment.ResolvedTarget, cause error, reader *bufio.Reader, out io.Writer, seen map[string]bool, deleteData bool) (context.Context, bool, error) {
	var challenge *machine.Error
	if !errors.As(cause, &challenge) || challenge.Code != machine.ErrorApprovalRequired ||
		(challenge.CauseCode != "impact-confirmation-required" && challenge.CauseCode != "data-deletion-consent-required") {
		return ctx, false, nil
	}
	plan, err := decodeRemoteApplicationRemovalPlan(challenge.Details)
	if err != nil || plan.planID == "" || plan.confirmations < 1 || plan.confirmations > 2 || seen[plan.planID] {
		return ctx, false, errors.New("Core returned an invalid or repeated provider removal plan; inspect it before retrying")
	}
	if plan.destructive != deleteData {
		return ctx, false, errors.New("Core returned a plan whose data disposition does not match the requested stop/destroy operation")
	}
	core, err := dialVerifiedRemoteCore(ctx, target.Name, target.RuntimeProvider, target.TenantID)
	if err != nil {
		return ctx, false, err
	}
	for _, leaf := range plan.targets {
		if leaf.Target != core.serverTargetName() {
			return ctx, false, errors.New("provider removal plan belongs to another Core Target")
		}
	}
	if noInput(ctx) {
		return ctx, false, errors.New("Core requires explicit acknowledgement of the current application removal plan; run interactively or provide a plan-bound consent file")
	}
	if err := renderRemoteApplicationRemovalPlan(ctx, out, target.Name, plan); err != nil {
		return ctx, false, err
	}
	if !readerIsTerminal(reader) {
		consent := application.ProviderChangeConsent{PlanID: plan.planID}
		if plan.scope != nil {
			return ctx, false, applicationDestructionPlanError(applicationDestructionPlan{Scope: *plan.scope}, application.ValidateProviderImpactScopeConsent(*plan.scope, consent))
		}
		if plan.impact != nil {
			return ctx, false, providerImpactConsentError(*plan.impact, application.ValidateProviderChangeConsent(*plan.impact, consent))
		}
		return ctx, false, errors.New("Core requires interactive acknowledgement of the current application removal plan")
	}
	question := "Remove this application and apply the listed provider changes?"
	if !deleteData {
		question = "Stop the application runtime and apply the listed provider changes while retaining its data?"
	}
	accepted, err := promptYesNo(reader, out, question, false)
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
	consent := application.ProviderChangeConsent{PlanID: plan.planID, First: true, Second: plan.confirmations == 2}
	if deleteData {
		approved, err := promptYesNo(reader, out, "Permanently delete this application's owned data?", false)
		if err != nil {
			return ctx, false, err
		}
		consent.DeleteData = approved
		if !approved {
			fmt.Fprintln(out, "Removal cancelled; no resources were changed.")
			return ctx, false, cli.Presented(errors.New("application data deletion was not authorized"))
		}
	}
	if plan.scope != nil {
		if err := application.ValidateProviderImpactScopeConsent(*plan.scope, consent); err != nil {
			return ctx, false, err
		}
	} else if plan.impact != nil {
		if err := application.ValidateProviderChangeConsent(*plan.impact, consent); err != nil {
			return ctx, false, providerImpactConsentError(*plan.impact, err)
		}
	}
	seen[plan.planID] = true
	return withProviderChangeConsent(ctx, &consent), true, nil
}

type remoteApplicationRemovalPlan struct {
	impact        *application.ProviderImpactPlan
	scope         *application.ProviderImpactScope
	targets       []application.ProviderImpactPlan
	effects       []application.OperationalConsequence
	planID        string
	confirmations int
	destructive   bool
	complete      bool
}

func decodeRemoteApplicationRemovalPlan(details any) (remoteApplicationRemovalPlan, error) {
	data, err := json.Marshal(details)
	if err != nil {
		return remoteApplicationRemovalPlan{}, err
	}
	var destruction applicationDestructionPlan
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&destruction) == nil && destruction.Scope.PlanID != "" {
		if destruction.Scope.Version != application.ProviderImpactScopeVersion || destruction.Scope.ConfirmationsRequired < 1 || destruction.Scope.ConfirmationsRequired > 2 {
			return remoteApplicationRemovalPlan{}, errors.New("invalid application removal scope")
		}
		return remoteApplicationRemovalPlan{
			impact: destruction.Providers.Impact, scope: &destruction.Scope, targets: destruction.Scope.Targets,
			effects: destruction.Effects, planID: destruction.Scope.PlanID,
			confirmations: destruction.Scope.ConfirmationsRequired, destructive: destruction.Scope.Destructive,
			complete: destruction.Scope.Complete,
		}, nil
	}
	var impact application.ProviderImpactPlan
	decoder = json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&impact) != nil || impact.Version != application.ProviderImpactVersion || impact.PlanID == "" || impact.Target == "" || len(impact.Impacts) == 0 || impact.ConfirmationsRequired < 1 || impact.ConfirmationsRequired > 2 {
		return remoteApplicationRemovalPlan{}, errors.New("invalid provider impact plan")
	}
	return remoteApplicationRemovalPlan{
		impact: &impact, targets: []application.ProviderImpactPlan{impact}, planID: impact.PlanID,
		confirmations: impact.ConfirmationsRequired, destructive: impact.Destructive, complete: impact.Complete,
	}, nil
}

func renderRemoteApplicationRemovalPlan(ctx context.Context, out io.Writer, target string, plan remoteApplicationRemovalPlan) error {
	if plan.impact != nil {
		impact := *plan.impact
		impact.Target = target
		impact.Destructive = plan.destructive
		impact.Complete = plan.complete
		if err := printProviderImpactForCLI(ctx, out, impact); err != nil {
			return err
		}
	} else {
		fmt.Fprintf(out, "\nApplication on %s\n  Stop released application runtimes\n", target)
	}
	for _, effect := range plan.effects {
		if strings.TrimSpace(effect.After) != "" {
			fmt.Fprintf(out, "  %s\n", effect.After)
		}
	}
	if plan.destructive {
		fmt.Fprintln(out, "  App-owned data and registration will be permanently deleted; shared or ambiguously owned data is retained.")
	} else {
		fmt.Fprintln(out, "  Persistent data, credentials and the recovery owner will be retained.")
	}
	if !plan.complete {
		fmt.Fprintln(out, "  Some existing resources could not be fully checked.")
	}
	return nil
}

func renderRemoteDestroyCompletion(out io.Writer, raw []byte, preserveData bool) error {
	var receipt struct {
		Application   string `json:"application"`
		Environment   string `json:"environment"`
		Target        string `json:"target"`
		Verified      bool   `json:"verified"`
		State         string `json:"state"`
		DataDeleted   *bool  `json:"data_deleted"`
		DataPreserved bool   `json:"data_preserved"`
	}
	if json.Unmarshal(raw, &receipt) != nil || !receipt.Verified || receipt.Application == "" || receipt.Target == "" {
		return errors.New("remote Core destroy receipt is invalid")
	}
	if preserveData {
		if !receipt.DataPreserved && (receipt.DataDeleted == nil || *receipt.DataDeleted) {
			return errors.New("remote Core stop receipt does not prove data retention")
		}
		fmt.Fprintf(out, "Application %s on %s (%s) stopped; persistent data and recovery owner retained.\n", receipt.Application, receipt.Target, receipt.Environment)
		fmt.Fprintln(out, "Next: baha app apply")
	} else {
		if receipt.DataDeleted == nil || !*receipt.DataDeleted {
			return errors.New("remote Core destroy receipt does not prove data deletion")
		}
		fmt.Fprintf(out, "Application %s on %s (%s) removed.\n", receipt.Application, receipt.Target, receipt.Environment)
		fmt.Fprintln(out, "Application-owned data was permanently deleted.")
		fmt.Fprintln(out, "Next: baha app init")
	}
	return nil
}
