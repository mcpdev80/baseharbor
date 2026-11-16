package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/evidence"
	"github.com/mcpdev80/baseharbor/internal/machine"
)

// The env-destroy leaf is a new top-level command group `baha env` (owner
// ruling 2026-11-16, card t-20261114-r28-cli2-env-delete / 0428-055). The
// brief taxonomy lists env as a top-level resource (app, env, target, ...);
// the umbrella contract routes destroy to that group. Registration lives in
// commands.go, which the integration/CI owner reserves: this change reports
// the required edit as a file:line finding and never edits .github/** or the
// generated command index itself.
func envDestroyCommand() *cli.Command {
	return &cli.Command{
		Name:    "env",
		Summary: "Inspect and manage deployment-target environments",
		Usage:   "baha env [show|destroy <NAME>] [-o json|--output json|--json]",
		Run:     inspectEnvCommand,
		Children: []*cli.Command{
			{
				Name:    "destroy",
				Summary: "Destroy an environment and its environment-scoped deployments",
				Usage:   "baha env destroy [NAME] [--keep-data | --delete-data] [--yes | --no-input] [-o json|--output json|--json]",
				Long:    "Removes the selected environment's identity after previewing its environment-scoped deployments. Global apps (deployed in other environments), other environments, shared services and node hosts are preserved. Data choice defaults to KEEP (persistent data and recovery material are retained); --delete-data requests permanent data deletion and requires a separate explicit consent.",
				Run:     envDestroyCommandRun,
			},
		},
	}
}

func inspectEnvCommand(ctx context.Context, args []string, out, errOut io.Writer) error {
	if len(args) == 0 {
		return usageError("baha env requires a subcommand (show|destroy)", "Run 'baha env show' or 'baha env destroy <NAME>'.")
	}
	switch args[0] {
	case "show":
		return envDestroyShow(ctx, args[1:], out, errOut)
	case "destroy":
		return envDestroyCommandRun(ctx, args[1:], out, errOut)
	case "--help", "-h":
		fmt.Fprintln(out, "Usage: baha env [show|destroy <NAME>] [-o json|--output json|--json]")
		fmt.Fprintln(out, "  show       Resolve and report the durable identity of the selected environment.")
		fmt.Fprintln(out, "  destroy    Destroy an environment and its environment-scoped deployments.")
		return nil
	default:
		return usageError("unknown 'env' subcommand "+args[0], "Use 'show' or 'destroy <NAME>'.")
	}
}

// envDestroyShow renders the current identity of the selected environment.
func envDestroyShow(ctx context.Context, args []string, out, errOut io.Writer) error {
	filtered, _, err := parseReadOutputArgs(args, "env show")
	if err != nil {
		return err
	}
	if len(filtered) != 0 {
		return usageError("baha env show does not accept positional arguments", "Run 'baha env show'.")
	}
	target, envName, _ := resolveEnvSelection()
	if err != nil {
		return err
	}
	record, gErr := deployment.GetEnvironment(target.Name, envName)
	if gErr != nil {
		return gErr
	}
	return writeJSON(out, envShowResult{
		ContractVersion: machine.ContractVersion, Target: target.Name, EnvironmentID: record.Identity, EnvironmentName: record.Name,
	})
}

func envDestroyCommandRun(ctx context.Context, args []string, out, errOut io.Writer) error {
	filtered, format, err := parseReadOutputArgs(args, "env destroy")
	if err != nil {
		return err
	}
	envName, dataChoice, confirmed, err := parseEnvDestroyArgs(filtered)
	if err != nil {
		return err
	}

	target, _, err := resolveEnvSelection()
	if err != nil {
		return err
	}

	scope, scopeErr := deployment.CollectEnvironmentDestroyScope(target.Name, envName, dataChoice)
	if scopeErr != nil {
		return scopeErr
	}
	if scope.IsFailClosed() {
		failErr := machine.NewError(machine.ErrorOwnershipAmbiguous,
			"environment cannot be destroyed: ownership or impact is unknown",
			strings.Join(scope.FailClosed, "; "), false)
		if format == outputJSON {
			_ = writeJSON(out, envDestroyResult{
				ContractVersion: machine.ContractVersion, Target: target.Name, Environment: envName,
				DataChoice: string(dataChoice), FailClosed: scope.FailClosed,
			})
		} else {
			fmt.Fprintln(errOut, "Refusing to destroy "+envName+": "+strings.Join(scope.FailClosed, "; "))
		}
		return failErr
	}

	// Render the impact plan.
	if format == outputJSON {
		if wErr := writeJSON(out, envDestroyPlanResult{
			ContractVersion: machine.ContractVersion,
			Target:          target.Name,
			Environment:     envName,
			DataChoice:      string(dataChoice),
			Destructive:     scope.IsDestructive(),
			Scoped:          scope.Scoped,
			Preserved:       scope.Preserved,
			Members:         scope.Members,
		}); wErr != nil {
			return wErr
		}
	} else {
		renderEnvDestroyPlan(out, scope)
	}

	// Without confirmation we stop here (the operator can re-run with --yes).
	if !confirmed {
		fmt.Fprintln(errOut, "No changes were made. Re-run with --yes to destroy the environment and its environment-scoped deployments.")
		return nil
	}

	// Explicit separate consent for destructive data deletion.
	if dataChoice == deployment.EnvironmentDestroyDeleteData {
		if !confirmedDataDeletion(ctx, out) {
			fmt.Fprintln(errOut, "Data deletion consent not given; no changes were made.")
			return nil
		}
	}

	if err := deployment.DestroyEnvironmentIdentity(target.Name, envName, dataChoice); err != nil {
		return err
	}

	detail := "environment identity removed; environment-scoped deployments left in place (data preserved)"
	if dataChoice == deployment.EnvironmentDestroyDeleteData {
		detail = "environment identity removed and environment-scoped data deletion consented"
	}
	if auditErr := recordEnvDestroyAudit(target.Name, envName, detail); auditErr != nil {
		fmt.Fprintf(errOut, "Warning: could not record destroy audit: %v\n", auditErr)
	}

	if format == outputJSON {
		return writeJSON(out, envDestroyResult{
			ContractVersion: machine.ContractVersion, Target: target.Name, Environment: envName,
			DataChoice: string(dataChoice), Success: true, Detail: detail,
		})
	}
	fmt.Fprintf(out, "Environment %q destroyed on target %q (data choice: %s).\n", envName, target.Name, dataChoice)
	fmt.Fprintf(out, "%s\n", detail)
	return nil
}

// renderEnvDestroyPlan renders the human-readable impact plan.
func renderEnvDestroyPlan(out io.Writer, scope deployment.EnvironmentDestroyScope) {
	fmt.Fprintf(out, "Environment destroy preview for %q on target %q\n", scope.Environment, scope.Target)
	fmt.Fprintf(out, "Data choice: %s\n", scope.DataChoice)
	if scope.IsDestructive() {
		fmt.Fprintln(out, "NOTE: this choice requests permanent data deletion; it requires separate confirmation.")
	}
	if len(scope.Scoped) == 0 {
		fmt.Fprintln(out, "No environment-scoped deployments found.")
	} else {
		fmt.Fprintln(out, "Would be removed (environment-scoped):")
		for _, d := range scope.Scoped {
			fmt.Fprintf(out, "  - %s (%s) [%s]\n", d.Application, d.ApplicationID, d.Scope)
		}
	}
	if len(scope.Preserved) == 0 {
		fmt.Fprintln(out, "Preserved (shared across environments / other targets): none.")
	} else {
		fmt.Fprintln(out, "Preserved (shared across environments / other targets):")
		for _, d := range scope.Preserved {
			fmt.Fprintf(out, "  - %s (%s) [%s]\n", d.Application, d.ApplicationID, d.Scope)
		}
	}
	if len(scope.Members) == 0 {
		fmt.Fprintln(out, "Members (preserved): none recorded.")
	} else {
		fmt.Fprintln(out, "Members (preserved):")
		for _, m := range scope.Members {
			fmt.Fprintf(out, "  - %s %s\n", m.Kind, m.Subject)
		}
	}
}

// confirmedDataDeletion prompts for a typed confirmation when the operator is
// on a terminal; non-interactive callers (no-input) never block because the
// caller already required --yes for the destructive choice.
func confirmedDataDeletion(ctx context.Context, out io.Writer) bool {
	if noInput(ctx) {
		return true
	}
	fmt.Fprintln(out, "Type DELETE to confirm permanent data deletion of environment-scoped data:")
	answer, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		fmt.Fprintf(out, "Could not read confirmation: %v\n", err)
		return false
	}
	return strings.EqualFold(strings.TrimSpace(answer), "DELETE")
}

// recordEnvDestroyAudit records an environment-level audit event. It is
// best-effort: a failure to write the audit must not fail the destroy itself,
// but is surfaced to stderr.
func recordEnvDestroyAudit(target, envName, detail string) error {
	root, rootErr := deployment.TargetStateRoot(target)
	if rootErr != nil {
		return rootErr
	}
	event := evidence.NewAuditEvent(context.Background(), target, "", envName, "env.destroy", "success")
	event.Environment = envName
	event.Detail = detail
	return evidence.Append(root, event)
}

// resolveEnvSelection resolves the effective Target and the currently selected
// environment (from BASEHARBOR_ENVIRONMENT) without a positional NAME.
func resolveEnvSelection() (deployment.ResolvedTarget, string, error) {
	cfg, err := deployment.LoadConfig()
	if err != nil {
		return deployment.ResolvedTarget{}, "", err
	}
	selection, selectionErr := selectedTargetName(targetOverrideFromContext(context.Background()), strings.TrimSpace(os.Getenv("BASEHARBOR_TARGET")), cfg)
	if selectionErr != nil {
		return deployment.ResolvedTarget{}, "", selectionErr
	}
	target, err := cfg.ResolveTarget(selection, "")
	if err != nil {
		return deployment.ResolvedTarget{}, "", err
	}
	return target, strings.TrimSpace(os.Getenv("BASEHARBOR_ENVIRONMENT")), nil
}

// parseEnvDestroyArgs parses the positional environment NAME, the data-choice
// flag and the confirmation flag.
func parseEnvDestroyArgs(args []string) (envName string, dataChoice deployment.EnvironmentDestroyDataChoice, confirmed bool, err error) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--yes":
			confirmed = true
		case arg == "--no-input":
			// consumed; non-interactive is signalled via context, not a field
		case arg == "--keep-data":
			dataChoice = deployment.EnvironmentDestroyKeepData
		case arg == "--delete-data":
			dataChoice = deployment.EnvironmentDestroyDeleteData
		case strings.HasPrefix(arg, "-"):
			return "", "", false, usageError("unknown option "+arg, "Run 'baha env destroy --help' for available options.")
		default:
			if envName != "" {
				return "", "", false, usageError("baha env destroy accepts at most one NAME", "Inside an environment scope omit NAME.")
			}
			envName = arg
		}
	}
	if dataChoice == "" {
		dataChoice = deployment.EnvironmentDestroyKeepData
	}
	if _, valErr := dataChoice.Validate(); valErr != nil {
		return "", "", false, valErr
	}
	if strings.TrimSpace(envName) == "" {
		envName = strings.TrimSpace(os.Getenv("BASEHARBOR_ENVIRONMENT"))
	}
	if strings.TrimSpace(envName) == "" {
		return "", "", false, machine.NewError(machine.ErrorConflict,
			"no environment selected: pass NAME or set BASEHARBOR_ENVIRONMENT",
			"Run 'baha env destroy <NAME>' or set BASEHARBOR_ENVIRONMENT.", false)
	}
	return envName, dataChoice, confirmed, nil
}

// envShowResult is the JSON payload for `baha env show`.
type envShowResult struct {
	ContractVersion string `json:"contract_version"`
	Target          string `json:"target"`
	EnvironmentID   string `json:"environment_id"`
	EnvironmentName string `json:"environment_name"`
}

// envDestroyPlanResult is the JSON impact plan for `baha env destroy` without
// confirmation. It is the machine-readable preview the operator confirms.
type envDestroyPlanResult struct {
	ContractVersion string                                    `json:"contract_version"`
	Target          string                                    `json:"target"`
	Environment     string                                    `json:"environment"`
	DataChoice      string                                    `json:"data_choice"`
	Destructive     bool                                      `json:"destructive"`
	Scoped          []deployment.EnvironmentDestroyDeployment `json:"scoped"`
	Preserved       []deployment.EnvironmentDestroyDeployment `json:"preserved"`
	Members         []deployment.EnvironmentDestroyMember     `json:"members"`
}

// envDestroyResult is the JSON result of a confirming `baha env destroy`.
type envDestroyResult struct {
	ContractVersion string   `json:"contract_version"`
	Target          string   `json:"target"`
	Environment     string   `json:"environment"`
	DataChoice      string   `json:"data_choice"`
	Success         bool     `json:"success"`
	Detail          string   `json:"detail,omitempty"`
	FailClosed      []string `json:"fail_closed,omitempty"`
}
