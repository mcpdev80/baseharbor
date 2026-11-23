package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/evidence"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/operatorauth"
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
				Usage:   "baha env destroy [NAME] [--keep-data | --delete-data [--confirm-delete-data]] [--yes | --no-input] [-o json|--output json|--json]",
				Long:    "Retires the selected environment's deployments before removing its identity. KEEP stops each selected runtime while retaining persistent data, recovery material and deployment owners; --delete-data permanently destroys selected deployments and requires separate explicit consent. Deployments in other environments, shared services and node hosts are preserved.",
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
		fmt.Fprintln(out, "Usage: baha env [show|destroy <NAME> [--keep-data|--delete-data [--confirm-delete-data]]] [-o json|--output json|--json]")
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
	target, envName, err := resolveEnvSelection()
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
	envName, dataChoice, confirmed, deleteDataConfirmed, err := parseEnvDestroyArgs(filtered)
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
		recordEnvDestroyAudit(ctx, target.Name, envName, "ownership or impact unknown: "+strings.Join(scope.FailClosed, "; "), "denial")
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

	// A confirmed JSON invocation emits one result object. The plan is returned
	// on its own only for a preview, so machine consumers never receive two
	// concatenated JSON documents for one command.
	if format == outputJSON && !confirmed {
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
		return nil
	}
	if format != outputJSON {
		renderEnvDestroyPlan(out, scope)
	}

	// Without confirmation we stop here (the operator can re-run with --yes).
	if !confirmed {
		fmt.Fprintln(errOut, "No changes were made. Re-run with --yes to retire the scoped deployments and remove the environment identity after all steps succeed.")
		return nil
	}

	// Explicit separate consent for destructive data deletion.
	if dataChoice == deployment.EnvironmentDestroyDeleteData {
		consentOutput := out
		if format == outputJSON {
			consentOutput = errOut
		}
		if !confirmedDataDeletion(ctx, consentOutput, deleteDataConfirmed) {
			if format == outputJSON {
				return writeJSON(out, envDestroyResult{
					ContractVersion: machine.ContractVersion, Target: target.Name, Environment: envName,
					DataChoice: string(dataChoice), Detail: "data deletion consent not given",
				})
			}
			fmt.Fprintln(errOut, "Data deletion consent not given; no changes were made.")
			return nil
		}
	}

	ctx = withApplicationInput(ctx, runtimeInput)
	ctx = withTargetOverride(ctx, target.Name)
	lifecycleOut, lifecycleErr := out, errOut
	if format == outputJSON {
		lifecycleOut, lifecycleErr = io.Discard, io.Discard
	}
	apply := func(applyCtx context.Context, dep deployment.EnvironmentDestroyDeployment, choice deployment.EnvironmentDestroyDataChoice) error {
		return executeEnvironmentScopedDestroy(applyCtx, dep, choice, lifecycleOut, lifecycleErr)
	}
	if err := applyAndRemoveEnvironmentIdentity(ctx, target.Name, envName, scope, apply); err != nil {
		fmt.Fprintf(errOut, "Environment destroy incomplete; identity retained so the operation can be retried: %v\n", err)
		return err
	}

	detail := "environment identity removed; scoped runtimes retired; persistent data, recovery material and deployment owners retained"
	if dataChoice == deployment.EnvironmentDestroyDeleteData {
		detail = "environment-scoped deployments and owned data destroyed; environment identity removed"
	}
	recordEnvDestroyAudit(ctx, target.Name, envName, detail, "success")

	if format == outputJSON {
		return writeJSON(out, envDestroyResult{
			ContractVersion: machine.ContractVersion, Target: target.Name, Environment: envName,
			DataChoice: string(dataChoice), Success: true, Detail: detail,
			Scoped: scope.Scoped, Preserved: scope.Preserved, Members: scope.Members,
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
		if scope.DataChoice == string(deployment.EnvironmentDestroyKeepData) {
			fmt.Fprintln(out, "Would be stopped (environment-scoped; data and deployment owners retained):")
		} else {
			fmt.Fprintln(out, "Would be destroyed (environment-scoped; owned data and deployment registrations removed):")
		}
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

// executeEnvironmentScopedDestroy delegates each exact deployment identity to
// the existing application lifecycle. KEEP uses app-stop semantics so its
// canonical deployment owner and persistent/recovery data remain available;
// DELETE uses full destruction. UUID selection avoids routing by a mutable app
// name and confines each operation to the authorized deployment.
func executeEnvironmentScopedDestroy(ctx context.Context, dep deployment.EnvironmentDestroyDeployment, choice deployment.EnvironmentDestroyDataChoice, out, errOut io.Writer) error {
	args := []string{"--uuid", dep.DeploymentID, "--yes"}
	if choice == deployment.EnvironmentDestroyKeepData {
		ctx = context.WithValue(ctx, applicationStopModeKey{}, true)
	}
	return executeApplicationDestroyLifecycle(ctx, application.Store{}, args, out, errOut)
}

// applyEnvironmentDestroyScope applies only verified scoped deployments. It
// verifies each lifecycle's durable result before moving to the next member;
// on any error the caller retains the environment identity, allowing a retry
// to recompute the remaining/current scope safely.
func applyEnvironmentDestroyScope(ctx context.Context, scope deployment.EnvironmentDestroyScope, apply func(context.Context, deployment.EnvironmentDestroyDeployment, deployment.EnvironmentDestroyDataChoice) error) error {
	if apply == nil {
		return fmt.Errorf("environment destroy lifecycle is unavailable")
	}
	choice, err := deployment.EnvironmentDestroyDataChoice(scope.DataChoice).Validate()
	if err != nil {
		return err
	}
	if scope.IsFailClosed() {
		return fmt.Errorf("environment destroy scope is fail-closed: %s", strings.Join(scope.FailClosed, "; "))
	}
	for _, dep := range scope.Scoped {
		if !dep.OwnerVerified || dep.DeploymentID == "" || dep.Environment != scope.Environment || dep.Scope != "scoped" {
			return fmt.Errorf("deployment ownership is not verified for %s; refusing environment destroy", dep.Application)
		}
		if err := apply(ctx, dep, choice); err != nil {
			return fmt.Errorf("process scoped deployment %s (%s): %w", dep.Application, dep.DeploymentID, err)
		}
		present, err := environmentDestroyDeploymentPresent(scope.Target, dep.DeploymentID)
		if err != nil {
			return fmt.Errorf("verify scoped deployment %s (%s): %w", dep.Application, dep.DeploymentID, err)
		}
		if choice == deployment.EnvironmentDestroyKeepData && !present {
			return fmt.Errorf("verify retained deployment owner %s (%s): deployment record is missing", dep.Application, dep.DeploymentID)
		}
		if choice == deployment.EnvironmentDestroyDeleteData && present {
			return fmt.Errorf("verify destroyed deployment %s (%s): deployment record remains", dep.Application, dep.DeploymentID)
		}
	}
	return nil
}

func applyAndRemoveEnvironmentIdentity(ctx context.Context, target, envName string, scope deployment.EnvironmentDestroyScope, apply func(context.Context, deployment.EnvironmentDestroyDeployment, deployment.EnvironmentDestroyDataChoice) error) error {
	if scope.Target != target || scope.Environment != envName {
		return fmt.Errorf("environment destroy scope binding changed; expected %s/%s, got %s/%s", target, envName, scope.Target, scope.Environment)
	}
	if _, err := deployment.EnvironmentDestroyDataChoice(scope.DataChoice).Validate(); err != nil {
		return err
	}
	if err := applyEnvironmentDestroyScope(ctx, scope, apply); err != nil {
		return err
	}
	if err := deployment.DestroyEnvironmentIdentity(target, envName, deployment.EnvironmentDestroyDataChoice(scope.DataChoice)); err != nil {
		return fmt.Errorf("remove environment identity after verified deployment processing: %w", err)
	}
	return nil
}

func environmentDestroyDeploymentPresent(target, deploymentID string) (bool, error) {
	records, err := deployment.ListDeployments(target)
	if err != nil {
		return false, err
	}
	for _, record := range records {
		if record.Identity.DeploymentID == deploymentID {
			return true, nil
		}
	}
	return false, nil
}

// confirmedDataDeletion prompts for a typed confirmation when the operator is
// on a terminal. A non-interactive invocation must provide the separate
// --confirm-delete-data flag in addition to --yes.
func confirmedDataDeletion(ctx context.Context, out io.Writer, explicitFlag bool) bool {
	if noInput(ctx) {
		return explicitFlag
	}
	if explicitFlag {
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

// recordEnvDestroyAudit records an environment-level audit event for the
// env.destroy seam. It is best-effort: a failure to write the audit must not
// fail the destroy itself, but is surfaced to stderr. The outcome (success /
// denial / failure), exact environment and target, and the human vs runtime
// actor are recorded without any environment-scoped data values.
func recordEnvDestroyAudit(ctx context.Context, target, envName, detail, outcome string) {
	root, rootErr := deployment.TargetStateRoot(target)
	if rootErr != nil {
		fmt.Fprintln(os.Stderr, "Warning: could not resolve target state root for env destroy audit:", rootErr)
		return
	}
	event := evidence.NewAuditEvent(ctx, target, "", envName, "env.destroy", outcome)
	event.Environment = envName
	event.Resource = envName
	event.Detail = detail
	if decision, ok := operatorauth.AuthorizationDecisionFromContext(ctx); ok {
		// A present AuthorizationDecision takes precedence over any principal:
		// a verified principal can coexist with a policy denial, so a present
		// decision must never fall back to principal-allow.
		if decision.Allowed {
			event.AuthorizationResult = "allow"
		} else {
			event.AuthorizationResult = "deny"
		}
	} else if _, ok := operatorauth.PrincipalFromContext(ctx); ok {
		event.AuthorizationResult = "allow"
	} else {
		// No verified operator on the enforcement seam: attribute the runtime
		// actor rather than a person. trusted-local is not a person.
		event.AuthorizationResult = "deny"
	}
	if err := evidence.Append(root, event); err != nil {
		fmt.Fprintln(os.Stderr, "Warning: could not record env destroy audit:", err)
	}
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
func parseEnvDestroyArgs(args []string) (envName string, dataChoice deployment.EnvironmentDestroyDataChoice, confirmed, deleteDataConfirmed bool, err error) {
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
		case arg == "--confirm-delete-data":
			deleteDataConfirmed = true
		case strings.HasPrefix(arg, "-"):
			return "", "", false, false, usageError("unknown option "+arg, "Run 'baha env destroy --help' for available options.")
		default:
			if envName != "" {
				return "", "", false, false, usageError("baha env destroy accepts at most one NAME", "Inside an environment scope omit NAME.")
			}
			envName = arg
		}
	}
	if dataChoice == "" {
		dataChoice = deployment.EnvironmentDestroyKeepData
	}
	if _, valErr := dataChoice.Validate(); valErr != nil {
		return "", "", false, false, valErr
	}
	if deleteDataConfirmed && dataChoice != deployment.EnvironmentDestroyDeleteData {
		return "", "", false, false, usageError("--confirm-delete-data requires --delete-data", "Specify both flags to explicitly authorize non-interactive data deletion.")
	}
	if strings.TrimSpace(envName) == "" {
		envName = strings.TrimSpace(os.Getenv("BASEHARBOR_ENVIRONMENT"))
	}
	if strings.TrimSpace(envName) == "" {
		return "", "", false, false, machine.NewError(machine.ErrorConflict,
			"no environment selected: pass NAME or set BASEHARBOR_ENVIRONMENT",
			"Run 'baha env destroy <NAME>' or set BASEHARBOR_ENVIRONMENT.", false)
	}
	return envName, dataChoice, confirmed, deleteDataConfirmed, nil
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
	ContractVersion string                                    `json:"contract_version"`
	Target          string                                    `json:"target"`
	Environment     string                                    `json:"environment"`
	DataChoice      string                                    `json:"data_choice"`
	Success         bool                                      `json:"success"`
	Detail          string                                    `json:"detail,omitempty"`
	FailClosed      []string                                  `json:"fail_closed,omitempty"`
	Scoped          []deployment.EnvironmentDestroyDeployment `json:"scoped,omitempty"`
	Preserved       []deployment.EnvironmentDestroyDeployment `json:"preserved,omitempty"`
	Members         []deployment.EnvironmentDestroyMember     `json:"members,omitempty"`
}
