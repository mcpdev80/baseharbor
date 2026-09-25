package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	logsprovider "github.com/mcpdev80/baseharbor/internal/logs"
	metricsprovider "github.com/mcpdev80/baseharbor/internal/metrics"
	"github.com/mcpdev80/baseharbor/internal/objectstorage"
	"github.com/mcpdev80/baseharbor/internal/openbao"
	"github.com/mcpdev80/baseharbor/internal/preflight"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
	tracesprovider "github.com/mcpdev80/baseharbor/internal/traces"
)

func appDownCommand(store application.Store) *cli.Command {
	return &cli.Command{
		Name:    "down",
		Summary: "Stop an application runtime while preserving persistent data",
		Usage:   "baha app down [NAME]",
		Long:    "Stops a repository application workload and its per-application Application Runtime Broker first, then removes BaseHarbor-managed backend containers and transient network while preserving persistent data volumes, runtime credentials, application-owned Compose volumes and managed OpenBao scope. Without NAME it resolves the nearest repository baseharbor.yaml.",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			resolved, err := resolveApplication(ctx, store, args, "down")
			if err != nil {
				return err
			}
			m := resolved.Manifest
			runtimeProject := application.RuntimeProjectNameForStore(resolved.Store, m)
			term := cli.NewTerminal(ctx, out, errOut)
			term.Header(m.Name, m.Environment)
			term.Info("target", resolved.Target.Name)
			files, err := application.ExistingRuntimeFiles(resolved.Store, m)
			if err != nil {
				return err
			}
			var compose bhruntime.Compose
			var before []bhruntime.ProjectResource
			checks := []preflight.Check{
				{Name: "manifest", Run: func(context.Context) error { return m.Validate() }},
				{Name: "supported desired services", Run: func(context.Context) error { return application.CheckSupportedRuntimeServices(m) }},
				{Name: "manifest permissions", Run: func(context.Context) error {
					return checkManifestPermissions(resolved.ManifestPath, resolved.FromRepository)
				}},
				{Name: "application workload", Run: func(context.Context) error { return preflightRepositoryWorkload(resolved) }},
				{Name: "runtime permissions", Run: func(context.Context) error { return application.CheckRuntimePermissions(files) }},
				{Name: "managed runtime definition", Run: func(context.Context) error { return application.CheckManagedRuntimeDefinition(files, m) }},
				{Name: "runtime orchestration", Run: func(ctx context.Context) error {
					var err error
					compose, err = detectComposeForApplication(ctx, resolved, bhruntime.CapabilityWorkloadLifecycle, bhruntime.CapabilityResourceOwnership)
					return err
				}},
				{Name: "runtime configuration", Run: func(ctx context.Context) error {
					return compose.ConfigProject(ctx, runtimeProject, files.Compose, files.Env)
				}},
				{Name: "runtime ownership", Run: func(ctx context.Context) error {
					var err error
					before, err = compose.InspectProjectResources(ctx, runtimeProject, application.ExpectedRuntimeResourcesForProject(m, runtimeProject))
					return err
				}},
			}
			results, ok := preflight.RunWithTimeout(ctx, checks, 30*time.Second)
			renderPreflightUX(term, results)
			if !ok {
				return errors.New("application down preflight failed")
			}

			if err := suspendConnectivityForManifest(ctx, compose, resolved); err != nil {
				return fmt.Errorf("suspend cross-application connectivity: %w", err)
			}
			if len(m.Exposures) > 0 {
				if err := stopManagedExposure(ctx, compose, m, files); err != nil {
					return err
				}
				term.Result("STOPPED", "managed-exposure", "application exposure provider stopped")
			}
			if err := metricsprovider.StopProvider(ctx, compose, m); err != nil {
				return fmt.Errorf("stop application-scoped metrics provider: %w", err)
			}
			if tracePlacement, found, err := application.RegisteredProviderPlacementAt(resolved.TargetStateRoot, m, capability.ProviderTempo); err != nil {
				return err
			} else if found && tracePlacement.Scope == capability.ScopeApplication {
				if err := tracesprovider.StopProvider(ctx, compose, m); err != nil {
					return fmt.Errorf("stop application-scoped traces provider: %w", err)
				}
			}
			if stopped, err := stopRepositoryWorkload(ctx, compose, resolved, files); err != nil {
				return err
			} else if stopped {
				term.Result("STOPPED", "workload", "repository workload stopped; application-owned volumes preserved")
			}
			if err := logsprovider.StopProviderAt(ctx, compose, resolved.TargetStateRoot, resolved.Target.Name, m); err != nil {
				return fmt.Errorf("stop application-scoped logs provider: %w", err)
			}
			if application.RequiresRuntimeBroker(m) {
				if err := stopRuntimeBroker(ctx, compose, m, files); err != nil {
					return err
				}
				term.Result("STOPPED", "runtime-broker", "application runtime broker stopped")
			}

			project := runtimeProject
			if err := compose.DownProject(ctx, project, files.Compose, files.Env); err != nil {
				return err
			}
			after, err := compose.InspectProjectResources(ctx, runtimeProject, application.ExpectedRuntimeResourcesForProject(m, runtimeProject))
			if err != nil {
				return fmt.Errorf("verify application down: %w", err)
			}
			if application.ResourceExists(after, "container") || application.ResourceExists(after, "network") {
				return errors.New("verify application down: container or network still exists")
			}
			for _, volume := range application.ExpectedPersistentRuntimeResourcesForProject(m, runtimeProject) {
				if application.ResourceNamedExists(before, volume) && !application.ResourceNamedExists(after, volume) {
					return fmt.Errorf("verify application down: persistent volume %s was not preserved", volume.Name)
				}
			}
			term.Section("Application")
			term.Result("STOPPED", "application", "persistent data preserved")
			return nil
		},
	}
}

func appDestroyCommand(store application.Store) *cli.Command {
	return &cli.Command{
		Name:    "destroy",
		Summary: "Permanently remove BaseHarbor-managed runtime resources and state",
		Usage:   "baha app destroy [NAME] [--yes] [--full-reset]",
		Long:    "Shows an ownership-verified destruction plan. With --yes it stops any repository workload and per-application Application Runtime Broker, removes BaseHarbor-managed runtime resources, volumes, OpenBao scope and application state. Repository deployment/TLS settings are preserved by default for recreate. --full-reset also removes BaseHarbor-owned repository deployment settings and normalized TLS copies, while preserving baseharbor.yaml, application-owned Compose data and any external certificate source directory.",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			return executeApplicationDestroyLifecycle(ctx, store, args, out, errOut)
		},
	}
}

func executeApplicationDestroyLifecycle(ctx context.Context, store application.Store, args []string, out, errOut io.Writer) error {
	execution, err := newApplicationDestroyExecution(ctx, store, args, out, errOut)
	if err != nil {
		return err
	}
	if err := execution.runPreflight(ctx); err != nil {
		return err
	}
	if err := execution.renderDeletePlan(); err != nil {
		return err
	}
	if !execution.confirmed {
		fmt.Fprintln(out, "No changes were made. Re-run with --yes to permanently delete BaseHarbor-managed resources.")
		return nil
	}
	if err := execution.destroyRuntimeResources(ctx); err != nil {
		return err
	}
	if err := execution.cleanupProviderState(ctx); err != nil {
		return err
	}
	return execution.removeApplicationState()
}

func parseDestroyArgs(args []string) (string, bool, bool, error) {
	var name string
	confirmed := false
	fullReset := false
	for _, arg := range args {
		switch {
		case arg == "--yes":
			confirmed = true
		case arg == "--full-reset":
			fullReset = true
		case strings.HasPrefix(arg, "-"):
			return "", false, false, usageError("unknown option "+arg, "Run 'baha app destroy --help' for available options.")
		default:
			if name != "" {
				return "", false, false, usageError("baha app destroy accepts at most one NAME", "Inside an application repository omit NAME.")
			}
			name = arg
		}
	}
	return name, confirmed, fullReset, nil
}

func openBaoDestroyScopeRequired(state openbao.State) (bool, error) {
	if !state.Initialized {
		return false, nil
	}
	if state.Sealed {
		return false, openbao.ErrSealed
	}
	return true, nil
}
