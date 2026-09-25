package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/openbao"
	"github.com/mcpdev80/baseharbor/internal/preflight"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func appCreateCommand() *cli.Command {
	return &cli.Command{
		Name:    "create",
		Summary: "Create an application manifest in BaseHarbor state",
		Usage:   "baha app create NAME [--environment ENV] [--sql] [--sql-instance NAME]... [--cache] [--cache-instance NAME]... [--s3] [--s3-bucket NAME]... [--secrets] [--require-secret NAME]...",
		Long:    "Creates BaseHarbor-managed declarative application source on the effective Target; it does not start containers. For a repository-owned source-of-truth manifest prefer 'baha app init'. If no service flag is supplied, one default SQL service is enabled; PostgreSQL is the current default provider.",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			m, err := manifestFromCreateArgs(args)
			if err != nil {
				return err
			}
			path, err := createTargetManagedApplication(ctx, m)
			if err != nil {
				return err
			}
			term := cli.NewTerminal(ctx, out, errOut)
			term.Header(m.Name, m.Environment)
			term.Section("Application")
			term.Result("CREATED", "application", m.Name)
			term.Info("manifest", path)
			fmt.Fprintln(out, "\nNext:")
			fmt.Fprintln(out, "  baha app plan "+m.Name)
			fmt.Fprintln(out, "  baha app preflight "+m.Name)
			return nil
		},
	}
}

func appListCommand() *cli.Command {
	return &cli.Command{
		Name:    "list",
		Summary: "List registered deployments for the effective target",
		Usage:   "baha app list [--all-targets]",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			allTargets := false
			for _, arg := range args {
				switch arg {
				case "--all-targets":
					allTargets = true
				default:
					return unknownOptionUsage("baha app list", arg, "--all-targets")
				}
			}
			var (
				items []deployment.DeploymentRecord
				err   error
			)
			if allTargets {
				items, err = deployment.ListAllDeployments()
			} else {
				target, resolveErr := effectiveTarget(ctx)
				if resolveErr != nil {
					return resolveErr
				}
				items, err = deployment.ListDeployments(target.Name)
			}
			if err != nil {
				return err
			}
			if len(items) == 0 {
				fmt.Fprintln(out, "No deployments registered.")
				return nil
			}
			if allTargets {
				fmt.Fprintf(out, "%-20s %-20s %-12s %-12s %-10s %s\n", "TARGET", "APPLICATION", "ENVIRONMENT", "RUNTIME", "STATE", "SOURCE")
				for _, item := range items {
					source := "MISSING"
					if deployment.SourceAvailable(item.Source) {
						source = "OK"
					}
					fmt.Fprintf(out, "%-20s %-20s %-12s %-12s %-10s %s\n", item.Identity.Target, item.Identity.Application, item.Identity.Environment, item.Applied.RuntimeProvider, item.Observed.State, source)
				}
				return nil
			}
			fmt.Fprintf(out, "%-20s %-12s %-10s %s\n", "APPLICATION", "ENVIRONMENT", "STATE", "SOURCE")
			for _, item := range items {
				source := "MISSING"
				if deployment.SourceAvailable(item.Source) {
					source = "OK"
				}
				fmt.Fprintf(out, "%-20s %-12s %-10s %s\n", item.Identity.Application, item.Identity.Environment, item.Observed.State, source)
			}
			return nil
		},
	}
}

func appShowCommand(store application.Store) *cli.Command {
	return &cli.Command{
		Name:    "show",
		Summary: "Show the resolved application manifest",
		Usage:   "baha app show [NAME]",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			resolved, err := resolveApplication(ctx, store, args, "show")
			if err != nil {
				return err
			}
			fmt.Fprint(out, resolved.Manifest.YAML())
			return nil
		},
	}
}

func appPlanCommand(store application.Store) *cli.Command {
	return &cli.Command{
		Name:    "plan",
		Summary: "Show desired resources without changing anything",
		Usage:   "baha app plan [NAME] [-o json|--output json]",
		Long:    "Builds a deterministic desired-state plan, including required secret readiness gates. Without NAME it resolves the nearest baseharbor.yaml from the current repository.",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			filtered, format, err := parseReadOutputArgs(args, "app plan")
			if err != nil {
				return err
			}
			resolved, err := resolveApplication(ctx, store, filtered, "plan")
			if err != nil {
				return err
			}
			plan, err := application.BuildPlan(resolved.Manifest)
			if err != nil {
				return err
			}
			if format == outputJSON {
				return writeJSON(out, plan)
			}
			fmt.Fprintf(out, "Plan for %s (%s)\n", plan.Application, plan.Environment)
			for i, action := range plan.Actions {
				fmt.Fprintf(out, "%d. %s %s - %s\n", i+1, action.Kind, action.Resource, action.Description)
			}
			fmt.Fprintln(out, "No changes were made.")
			return nil
		},
	}
}

func appPreflightCommand(store application.Store) *cli.Command {
	return &cli.Command{
		Name:    "preflight",
		Summary: "Validate an application before mutation",
		Usage:   "baha app preflight [NAME]",
		Long:    "Checks manifest integrity, supported desired services, local state, the container runtime, OpenBao application-provisioning prerequisites and required-secret readiness. Without NAME it resolves the nearest repository baseharbor.yaml.",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			resolved, err := resolveApplication(ctx, store, args, "preflight")
			if err != nil {
				return err
			}
			m := resolved.Manifest
			var compose bhruntime.Compose
			var platformFiles bhruntime.Files
			var requiredStatuses []openbao.RequiredSecretStatus
			var workloadSecurity application.WorkloadSecurityReport
			requiredKnown := false
			checks := []preflight.Check{
				{Name: "manifest", Run: func(context.Context) error { return m.Validate() }},
				{Name: "supported desired services", Run: func(context.Context) error { return application.CheckSupportedRuntimeServices(m) }},
				{Name: "manifest permissions", Run: func(context.Context) error {
					return checkManifestPermissions(resolved.ManifestPath, resolved.FromRepository)
				}},
				{Name: "runtime orchestration", Run: func(ctx context.Context) error {
					var err error
					compose, err = detectComposeForApplication(ctx, resolved, bhruntime.CapabilityWorkloadLifecycle)
					return err
				}},
				{Name: "desired-state plan", Run: func(context.Context) error {
					_, err := application.BuildPlan(m)
					return err
				}},
				{Name: "workload security", Run: func(ctx context.Context) error {
					var err error
					workloadSecurity, err = preflightRepositoryWorkloadSecurity(ctx, compose, resolved)
					return err
				}},
			}
			if m.Services.Secrets {
				identity := openbao.ApplicationIdentity{Name: m.Name, Environment: m.Environment}
				checks = append(checks,
					preflight.Check{Name: "OpenBao control-plane runtime", Run: func(context.Context) error {
						var err error
						platformFiles, err = existingTargetRuntimeFiles(ctx)
						return err
					}},
					preflight.Check{Name: "OpenBao application provisioning", Run: func(ctx context.Context) error {
						if platformFiles.Compose == "" {
							return errors.New("BaseHarbor control-plane runtime is not materialized; run 'baha up' first")
						}
						return openbao.CheckApplicationProvisioning(ctx, compose, platformFiles, identity)
					}},
				)
				if len(application.RequiredSecretNames(m)) > 0 {
					checks = append(checks, preflight.Check{Name: "required application secrets", Run: func(ctx context.Context) error {
						files, err := application.ExistingRuntimeFiles(resolved.Store, m)
						if errors.Is(err, application.ErrRuntimeNotApplied) {
							return nil
						}
						if err != nil {
							return err
						}
						requiredStatuses, err = inspectRequiredApplicationSecrets(ctx, compose, platformFiles, m, files)
						if err != nil {
							return err
						}
						requiredKnown = true
						return openbao.RequireApplicationSecrets(requiredStatuses)
					}})
				}
			}
			results, ok := preflight.RunWithTimeout(ctx, checks, 30*time.Second)
			preflight.Format(out, results)
			if cli.NewTerminal(ctx, out, errOut).Verbose() {
				printWorkloadSecurityFindings(out, workloadSecurity)
			}
			if len(application.RequiredSecretNames(m)) > 0 {
				if requiredKnown {
					printRequiredSecretStatus(out, requiredStatuses)
				} else {
					fmt.Fprintf(out, "  %-24s %-38s %s\n", "REQUIRED SECRET", "STATUS", "ACTION")
					for _, name := range application.RequiredSecretNames(m) {
						fmt.Fprintf(out, "  %-24s %-38s %s\n", name, "unknown - secret scope not materialized yet", "baha app secret set "+name)
					}
					fmt.Fprintln(out, "Secret presence becomes verifiable after the application secret scope is materialized by 'baha app apply'.")
				}
			}
			if !ok {
				return errors.New("application preflight failed")
			}
			fmt.Fprintln(out, "Preflight passed. No changes were made.")
			return nil
		},
	}
}
