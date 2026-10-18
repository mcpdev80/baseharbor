package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func appEditRegisteredCommand(store application.Store) *cli.Command {
	return &cli.Command{
		Name: "edit", Summary: "Edit current application needs with preview and impact consent",
		Usage: "baha app edit [NAME] [-e ENV|--environment ENV]",
		Long:  "Cancellation, No and EOF remain write-free; only the existing confirmed Edit handler applies a fresh approved plan.",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			if noInput(ctx) || !readerIsTerminal(appNewInput) {
				return usageError("app edit requires a terminal", "Use an interactive session; --yes cannot replace consequence consent.")
			}
			_, err := runAppEdit(ctx, store, args, appNewInput, out, errOut)
			return err
		},
	}
}

func appFootprintRegisteredCommand(store application.Store) *cli.Command {
	return &cli.Command{
		Name: "footprint", Summary: "Read the current application footprint",
		Usage: "baha app footprint show [NAME] [-o json|--output json] [--details]",
		Children: []*cli.Command{{
			Name: "show", Summary: "Display the authoritative Target footprint without mutation",
			Usage: "baha app footprint show [NAME] [-o json|--output json] [--details]",
			Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
				details := false
				rest := make([]string, 0, len(args))
				for _, a := range args {
					if a == "--details" {
						details = true
					} else {
						rest = append(rest, a)
					}
				}
				selection, format, err := parseReadOutputArgs(rest, "app footprint show")
				if err != nil {
					return err
				}
				resolved, err := resolveApplication(ctx, store, selection, "footprint show")
				if err != nil {
					return err
				}
				return appFootprintShow(ctx, out, func(ctx context.Context) (GuidedReadContext, error) {
					return readGuidedApplicationContext(ctx, resolved)
				}, appFootprintShowOptions{JSON: format == outputJSON, Details: details, Target: resolved.Target.Name})
			},
		}},
	}
}

func registerMCPFootprintShowTool(server *mcp.Server, store application.Store) {
	mcp.AddTool(server, machineMCPTool("app.footprint.show", "Read the canonical Target-bound application footprint.", false),
		func(ctx context.Context, _ *mcp.CallToolRequest, input machineApplicationInput) (*mcp.CallToolResult, any, error) {
			ctx = withTargetOverride(ctx, input.Target)
			resolved, err := resolveApplication(ctx, store, machineApplicationArgs(input.Name, input.Environment), "footprint show")
			if err != nil {
				return machineMCPFailure(err)
			}
			if err := authorizeResolvedMCPOperation(ctx, "app.footprint.show", resolved, ""); err != nil {
				return machineMCPFailure(err)
			}
			view, err := readGuidedApplicationContext(ctx, resolved)
			if err != nil {
				return machineMCPFailure(err)
			}
			raw, err := appMachineFootprintJSON(view, resolved.Target.Name)
			if err != nil {
				return machineMCPFailure(err)
			}
			var footprint application.Footprint
			if err := json.Unmarshal(raw, &footprint); err != nil {
				return machineMCPFailure(err)
			}
			return nil, footprint, nil
		})
}

// MCP change is deliberately two phase; callers must resubmit the exact typed
// proposal and provider-consequence consent, which are freshly revalidated.
type appMCPEditInput struct {
	Target      string
	Name        string
	Environment string
	Phase       string
	Desired     appMachineDesiredRequest
	Proposal    *appMachineDesiredResult
	Consent     *application.ProviderChangeConsent
}

func registerMCPAppEditTool(server *mcp.Server, store application.Store) {
	mcp.AddTool(server, machineMCPTool("app.edit", "Propose or apply reviewed application needs with exact fresh provider-impact consent.", false),
		func(ctx context.Context, _ *mcp.CallToolRequest, input appMCPEditInput) (*mcp.CallToolResult, any, error) {
			ctx = withTargetOverride(ctx, input.Target)
			args := machineApplicationArgs(input.Name, input.Environment)
			resolved, err := resolveApplication(ctx, store, args, "edit")
			if err != nil {
				return machineMCPFailure(err)
			}
			if input.Target == "" || input.Target != resolved.Target.Name || input.Desired.Target != input.Target {
				return machineMCPFailure(errors.New("app edit: wrong Target"))
			}
			if err := authorizeResolvedMCPOperation(ctx, "app.edit", resolved, ""); err != nil {
				return machineMCPFailure(err)
			}
			authorities := defaultAppEditAuthorities(store, args)
			switch strings.ToLower(strings.TrimSpace(input.Phase)) {
			case "propose":
				if input.Proposal != nil || input.Consent != nil {
					return machineMCPFailure(errors.New("app edit: unexpected consent or proposal"))
				}
				proposal, err := appMachinePropose(ctx, input.Desired, authorities.Read, authorities.Choices, authorities.Plan)
				if err != nil {
					return machineMCPFailure(err)
				}
				return nil, proposal, nil
			case "apply":
				if input.Proposal == nil || input.Consent == nil {
					return machineMCPFailure(errors.New("app edit: exact proposal and consent required"))
				}
				source, err := appEditSourceBytes(resolved)
				if err != nil {
					return machineMCPFailure(err)
				}
				loaded, err := appMachineHydrate(ctx, input.Target, source, appEditLoadReader(authorities.Read))
				if err != nil {
					return machineMCPFailure(err)
				}
				preferences := make([]appProviderPreference, 0, len(input.Proposal.Preferences))
				for _, p := range input.Proposal.Preferences {
					preferences = append(preferences, appProviderPreference{Target: input.Target, ProviderPreference: p})
				}
				desired, err := appEditDesiredFromDecision(loaded, appWizardDecision{
					Target: input.Target, Requirements: append([]capability.Requirement(nil), input.Desired.Requirements...),
					Preferences: preferences,
				})
				if err != nil {
					return machineMCPFailure(err)
				}
				execute := appEditRuntimeExecuteAuthority(store, args, resolved, source, authorities, io.Discard, io.Discard, nil, nil)
				result, err := appMachineChangeDesired(ctx, appMachineDesiredChangeRequest{
					Desired: input.Desired, Proposal: *input.Proposal, Consent: *input.Consent,
				}, authorities.Read, authorities.Choices, authorities.Plan,
					authorities.Authorize, func(ctx context.Context, plan application.ProviderImpactPlan, consent application.ProviderChangeConsent) ([]application.ProviderExecutionOutcome, error) {
						return execute(ctx, desired, plan, consent)
					})
				if err != nil {
					return machineMCPFailure(err)
				}
				return nil, result, nil
			default:
				return machineMCPFailure(errors.New("app edit: phase must be propose or apply"))
			}
		})
}
