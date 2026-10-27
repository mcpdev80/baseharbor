package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/cli"
)

func rootCommand() *cli.Command {
	store := application.DefaultStore()
	appCmd := configuredApplicationCommand(store)

	var appPlan *cli.Command
	for _, child := range appCmd.Children {
		if child.Name == "plan" {
			appPlan = child
			break
		}
	}

	root := &cli.Command{
		Name:    "baha",
		Summary: "BaseHarbor command-line interface",
		Usage:   "baha <command> [options]",
		Long:    "Manage BaseHarbor and isolated application backend runtimes from one binary. Commands are fail-closed: validation and preflight happen before mutation.",
	}

	root.Children = []*cli.Command{
		humanNewCommand(),
		humanOpenCommand(store),
		{
			Name:    "init",
			Summary: "Initialize or adopt the application in the current repository",
			Usage:   "baha init --agents [--json] | baha init [--quick] [--json] [--input NAME=VALUE]... [--yes]",
			Long:    "Canonical application initialization. Reuses the same repository inspection and guided input resolution as application initialization; explicit flags remain available for non-interactive use. Providers are resolved from selected intent during apply.",
			Run:     appInitWithInputResolverCommand(store).Run,
		},
		{
			Name:    "list",
			Summary: "List managed applications on the selected Target",
			Usage:   "baha list [options]",
			Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
				for _, child := range appCmd.Children {
					if child.Name == "list" && child.Run != nil {
						return child.Run(ctx, args, out, errOut)
					}
				}
				return errors.New("application discovery is unavailable")
			},
		},
		{
			Name:    "inspect",
			Summary: "Inspect the current repository and its application requirements",
			Usage:   "baha inspect [options]",
			Run:     appInspectCommand().Run,
		},
		{
			Name:    "backup",
			Summary: "Back up the current application through its managed lifecycle",
			Usage:   "baha backup [options]",
			Run:     appGuidedBackupCommand(store).Run,
		},
		{
			Name:    "restore",
			Summary: "Restore an application through the ownership-safe managed lifecycle",
			Usage:   "baha restore [options]",
			Run:     appGuidedRestoreCommandWithRecoveryMetadata(store).Run,
		},
		{
			Name:    "up",
			Summary: "Converge selected application intent, or explicitly start the management Core",
			Usage:   "baha up [-e ENV|--environment ENV] [--yes] [--skip-memory-preflight] [--control-plane-only] [--ha] [--trust-host-ca] [--postgres-port PORT] [--openbao-port PORT] [--recovery-file PATH]",
			Long:    "Inside an application repository, converges selected capabilities and genuine dependencies through the same current-plan acknowledgement, authorization and verification as app apply. It does not install a management Core merely because an application exists. Unresolved deployment inputs use the existing guided resolver; --yes does not replace provider-consequence or data-deletion consent. In a detected project without baseharbor.yaml, interactive use initializes the application through the existing guided flow; --yes uses unambiguous safe defaults. Directories without application signals retain explicit management setup. --control-plane-only explicitly selects the independent SQL/Secrets/Identity management installation and its HA, port, recovery and host-trust options. --trust-host-ca remains a separate explicit opt-in; --yes never changes host trust. -e/--environment selects resolved deployment state without rewriting authored intent.",
			Run:     runtimeUpCommandWithInputResolver,
		},
		{
			Name:    "down",
			Summary: "Stop the current application, or the local control plane outside a repository",
			Usage:   "baha down [options]",
			Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
				if inApplicationRepository() {
					return appDownCommand(store).Run(ctx, args, out, errOut)
				}
				return controlPlaneDownCLI(ctx, args, out, errOut)
			},
		},
		{
			Name:    "destroy",
			Summary: "Remove owned runtime resources with explicit consequence consent",
			Usage:   "baha destroy [--yes] [--delete-data] | baha destroy --all [--yes]",
			Long:    "Outside a repository, remove only canonical Core containers/startup units after two current-plan acknowledgements, including deliberately accepted outages for active consumers. Preserve data, networks, protected owners, bindings, workloads and other providers by default. --delete-data additionally selects owned Core volumes and requires separate deletion consent. --yes replaces no consequence or deletion consent. --all is the explicit installation cleanup path across all Targets; it selects owned data and installation state for deletion while preserving application source repositories and external application-owned data.",
			Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
				if inApplicationRepository() && !containsExactArgument(args, "--all") {
					return appDestroyCommand(store).Run(ctx, args, out, errOut)
				}
				return runtimeDestroyCommand(ctx, args, out, errOut)
			},
		},
		{
			Name:    "plan",
			Summary: "Show the application plan in the current repository",
			Usage:   "baha plan [NAME] [-o json|--output json]",
			Long:    "Repository-aware shorthand for 'baha app plan'. It is read-only and uses the same application planning core.",
			Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
				if appPlan == nil {
					return errors.New("application plan command is unavailable")
				}
				return appPlan.Run(ctx, args, out, errOut)
			},
		},
		{
			Name:    "status",
			Summary: "Show application status in a repository, otherwise control-plane status",
			Usage:   "baha status [-o json|--output json]",
			Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
				if inApplicationRepository() {
					return appStatusCommandWithTLS(store).Run(ctx, args, out, errOut)
				}
				filtered, format, err := parseReadOutputArgs(args, "status")
				if err != nil {
					return err
				}
				if len(filtered) != 0 {
					return usageError("status accepts output options only", "Use --json for structured control-plane state.")
				}
				if format == outputJSON {
					result, err := inspectControlPlane(ctx)
					if err != nil {
						return err
					}
					return writeJSON(out, result)
				}
				target, err := effectiveTarget(ctx)
				if err != nil {
					return err
				}
				fmt.Fprintf(out, "Selection source: %s (%s)\n", target.Name, targetSelectionOrigin(ctx))
				return runtimeStatus(ctx, out)
			},
		},
		{
			Name:    "doctor",
			Summary: "Diagnose the current application repository, otherwise the control plane",
			Usage:   "baha doctor [--fix --yes] [-o json|--output json]",
			Long:    "Inside an application repository, runs the same application doctor used by 'baha app doctor'. Outside a repository it keeps the control-plane doctor behavior. Structured output is read-only and cannot be combined with --fix.",
			Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
				if inApplicationRepository() {
					return appDoctorRepairCommandWithTLS(store).Run(ctx, args, out, errOut)
				}

				return doctorCommand(ctx, args, out, errOut)
			},
		},
		providerCommand(),
		stackCommand(),
		workspaceNamespaceCommand(),
		targetCommand(),
		useTargetCommand(),
		nodeCommand(),
		devCommand(),
		configCommand(),
		shellInitCommand(),
		promptCommand(),
		policyCommand(store),
		agentCommand(),
		mcpCommand(store),
		serveCommand(store),
		appCmd,
		connectCommand(),
		disconnectCommand(),
		connectionsCommand(),
		openBaoCommand(),
		trustCommand(),
		operatorLoginCommand(),
		operatorLogoutCommand(),
		operatorWhoAmICommand(),
		updateCommand(),
		versionCommand(),
	}
	root.Children = append(root.Children, tuiCommand(store), completionCommand(root, store), internalCompletionCommand(root, store))
	if inApplicationRepository() {
		root.HelpGroups = []cli.HelpGroup{
			{Title: "Application", Commands: []string{"init", "up", "down", "status", "open", "inspect", "plan", "doctor", "backup", "restore", "update", "destroy"}},
			{Title: "Create and discover", Commands: []string{"new", "list"}},
		}
	} else {
		root.HelpGroups = []cli.HelpGroup{
			{Title: "Get started", Commands: []string{"new", "init", "list", "up"}},
			{Title: "Understand and maintain", Commands: []string{"status", "inspect", "plan", "doctor", "update", "backup", "restore", "destroy", "down"}},
		}
	}
	return root
}

func versionCommand() *cli.Command {
	return &cli.Command{
		Name:    "version",
		Aliases: nil,
		Summary: "Print build version",
		Usage:   "baha version [-o json|--output json|--json]",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			filtered, format, err := parseReadOutputArgs(args, "version")
			if err != nil {
				return err
			}
			if len(filtered) != 0 {
				return usageError("baha version does not accept positional arguments", "Use --json or -o json for structured output.")
			}
			if format == outputJSON {
				return writeJSON(out, map[string]string{
					"version": version,
					"commit":  commit,
					"built":   date,
				})
			}
			fmt.Fprintf(out, "baha %s (commit %s, built %s)\n", version, commit, date)
			return nil
		},
	}
}

func configuredApplicationCommand(store application.Store) *cli.Command {
	appCmd := appCommand(store)
	for i, child := range appCmd.Children {
		switch child.Name {
		case "init":
			initCmd := appInitWithInputResolverCommand(store)
			initCmd.Usage = "baha app init --agents [--json] | baha app init [--quick] [--json] | baha app init [--input NAME=VALUE]... [--hostname HOST] [--tls acme|existing|local] [--cert-dir DIR] [--yes] | baha app init [NAME] [-e ENV|--environment ENV] [--sql|--sql-instance NAME] [--cache|--cache-instance NAME] [--key-value|--key-value-instance NAME] [--document-db|--document-db-instance NAME] [--messaging-queue|--messaging-queue-instance NAME] [--messaging-pubsub|--messaging-pubsub-instance NAME] [--messaging-stream|--messaging-stream-instance NAME] [--s3|--s3-bucket NAME] [--secrets|--require-secret NAME] [--workload-component NAME]... [--workload-source KIND:PATH]"
			initCmd.Long += " Without baseharbor.yaml, the existing manifest flags remain available for deterministic repository-contract creation."
			appCmd.Children[i] = initCmd
		case "show":
			appCmd.Children[i] = appShowCommandWithRecoveryMetadata(store)
		}
	}
	appCmd.Children = append(appCmd.Children,
		appEditRegisteredCommand(store),
		appFootprintRegisteredCommand(store),
		appInspectCommand(),
		appApplyCommand(store),
		appGuidedBackupCommand(store),
		appGuidedRestoreCommandWithRecoveryMetadata(store),
		appEnvCommand(store),
		appPSQLCommand(store),
		appRedisCommand(store),
		appCredsCommand(store),
		appLogsCommand(store),
		appShellCommand(store),
		appExecCommand(store),
		appUpdateCommand(store),
		appTLSCommand(store),
		appStatusCommandWithTLS(store),
		appDoctorRepairCommandWithTLS(store),
		appDownCommand(store),
		appUpCommand(store),
		appDestroyCommand(store),
		appSecretCommand(store),
		appRuntimeIdentityCommand(store),
		appEvidenceCommand(store),
	)
	applyRemainingApplicationRuntimeProviderGuards(store, appCmd)

	return appCmd
}

func usageError(message, hint string) error {
	return &cli.UsageError{Message: message, Hint: hint}
}

type noArgsHandler func(io.Writer) error

func noArgs(name string, fn noArgsHandler) cli.RunFunc {
	return func(ctx context.Context, args []string, out, errOut io.Writer) error {
		if len(args) != 0 {
			return usageError(name+" does not accept arguments", "Run '"+name+" --help' for usage.")
		}
		return fn(out)
	}
}

type noArgsCtxHandler func(context.Context, io.Writer) error

func noArgsCtx(name string, fn noArgsCtxHandler) cli.RunFunc {
	return func(ctx context.Context, args []string, out, errOut io.Writer) error {
		if len(args) != 0 {
			return usageError(name+" does not accept arguments", "Run '"+name+" --help' for usage.")
		}
		return fn(ctx, out)
	}
}

func inApplicationRepository() bool {
	cwd, err := os.Getwd()
	if err != nil {
		return false
	}
	found, err := application.HasRepositoryApplication(cwd)
	return err == nil && found
}

// containsExactArgument avoids guessing intent from substrings of option values.
func containsExactArgument(args []string, expected string) bool {
	for _, arg := range args {
		if arg == expected {
			return true
		}
	}
	return false
}
