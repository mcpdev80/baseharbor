package main

import (
	"context"
	"encoding/json"
	"io"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/machine"
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

// appMCPEditInput is the valid MCP propose/apply boundary. The propose phase
// requires only the Target-bound edit intent; the apply phase additionally
// demands the exact typed proposal and provider-consequence consent, which the
// handler enforces after the schema has accepted an optional proposal/consent.
type appMCPEditInput struct {
	Target      string                             `json:"target" jsonschema:"exact resolved deployment target for this edit"`
	Name        string                             `json:"name,omitempty" jsonschema:"optional stored application name; omit inside an application repository"`
	Environment string                             `json:"environment,omitempty" jsonschema:"optional deployment environment selected from repository intent"`
	Phase       string                             `json:"phase" jsonschema:"required exactly \"propose\" or \"apply\""`
	Desired     appMachineDesiredRequest           `json:"desired" jsonschema:"Target-bound edit intent; required for both phases"`
	Proposal    *appMachineDesiredResult           `json:"proposal,omitempty" jsonschema:"typed proposal from the matching propose phase; required for apply"`
	Consent     *application.ProviderChangeConsent `json:"consent,omitempty" jsonschema:"fresh provider-consequence consent from the matching propose phase; required for apply"`
}

func registerMCPAppEditTool(server *mcp.Server, store application.Store) {
	registerMCPAppEditToolWithAuthorities(server, store, defaultAppEditAuthorities)
}

// The registered transport consumes the existing Edit authorities. Production
// execution stays on the existing lifecycle; contract tests supply bounded
// authority fixtures through this same registration path.
func registerMCPAppEditToolWithAuthorities(server *mcp.Server, store application.Store, authoritiesFor func(application.Store, []string) appEditAuthorities) {
	mcp.AddTool(server, machineMCPTool("app.edit", "Propose or apply reviewed application needs with exact fresh provider-impact consent.", false),
		func(ctx context.Context, _ *mcp.CallToolRequest, input appMCPEditInput) (*mcp.CallToolResult, any, error) {
			ctx = withTargetOverride(ctx, input.Target)
			args := machineApplicationArgs(input.Name, input.Environment)
			resolved, err := resolveApplication(ctx, store, args, "edit")
			if err != nil {
				return machineMCPFailure(err)
			}
			if input.Target == "" || input.Target != resolved.Target.Name || input.Desired.Target != input.Target {
				return machineMCPFailure(machine.NewError(machine.ErrorConflict,
					"app edit Target does not match the resolved deployment Target.",
					"Resubmit with the exact resolved deployment Target for this application.", false))
			}
			if err := authorizeResolvedMCPOperation(ctx, "app.edit", resolved, ""); err != nil {
				return machineMCPFailure(err)
			}
			authorities := authoritiesFor(store, args)
			switch strings.ToLower(strings.TrimSpace(input.Phase)) {
			case "propose":
				if input.Proposal != nil || input.Consent != nil {
					return machineMCPFailure(machine.NewError(machine.ErrorValidationFailed,
						"app edit propose must not carry the typed proposal or provider-consequence consent.",
						"Resubmit propose with only the Target-bound desired intent.", false))
				}
				proposal, err := appMachinePropose(ctx, input.Desired, authorities.Read, authorities.Choices, authorities.Plan)
				if err != nil {
					return machineMCPFailure(err)
				}
				return nil, proposal, nil
			case "apply":
				if input.Proposal == nil || input.Consent == nil {
					return machineMCPFailure(machine.NewError(machine.ErrorValidationFailed,
						"app edit apply requires the exact typed proposal and provider-consequence consent.",
						"Resubmit apply with the matching proposal and the fresh consent from the propose phase.", false))
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
				execute := authorities.Execute
				if execute == nil {
					execute = appEditRuntimeExecuteAuthority(store, args, resolved, source, authorities, io.Discard, io.Discard, nil, nil)
				}
				result, err := appMachineChangeDesired(ctx, appMachineDesiredChangeRequest{
					Desired: input.Desired, Proposal: *input.Proposal, Consent: *input.Consent,
				}, authorities.Read, authorities.Choices, authorities.Plan,
					authorities.Authorize, func(ctx context.Context, plan application.ProviderImpactPlan, consent application.ProviderChangeConsent) ([]application.ProviderExecutionOutcome, error) {
						return execute(ctx, desired, plan, consent)
					})
				if err != nil {
					if result.Version == application.ProviderImpactVersion && result.PlanID != "" {
						failure := *machine.Classify(classifyMachineCLIError(err))
						failure.Details = result
						return machineMCPFailure(&failure)
					}
					return machineMCPFailure(err)
				}
				return nil, result, nil
			default:
				return machineMCPFailure(machine.NewError(machine.ErrorValidationFailed,
					"app edit phase must be exactly \"propose\" or \"apply\".",
					"Resubmit with one of the two supported phases.", false))
			}
		})
}
