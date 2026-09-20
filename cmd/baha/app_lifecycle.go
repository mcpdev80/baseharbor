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
	logsprovider "github.com/mcpdev80/baseharbor/internal/logs"
	metricsprovider "github.com/mcpdev80/baseharbor/internal/metrics"
	"github.com/mcpdev80/baseharbor/internal/objectstorage"
	"github.com/mcpdev80/baseharbor/internal/openbao"
	"github.com/mcpdev80/baseharbor/internal/preflight"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	tracesprovider "github.com/mcpdev80/baseharbor/internal/traces"
)

func appDownCommand(store application.Store) *cli.Command {
	return &cli.Command{
		Name:    "down",
		Summary: "Stop an application runtime while preserving persistent data",
		Usage:   "baha app down [NAME]",
		Long:    "Stops a repository application workload and its per-application Application Runtime Broker first, then removes BaseHarbor-managed backend containers and transient network while preserving persistent data volumes, runtime credentials, application-owned Compose volumes and managed OpenBao scope. Without NAME it resolves the nearest repository baseharbor.yaml.",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			resolved, err := resolveApplication(store, args, "down")
			if err != nil {
				return err
			}
			m := resolved.Manifest
			term := cli.NewTerminal(ctx, out, errOut)
			term.Header(m.Name, m.Environment)
			files, err := application.ExistingRuntimeFiles(resolved.Store, m)
			if err != nil {
				return err
			}

			checkCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
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
				{Name: "container runtime + compose", Run: func(ctx context.Context) error {
					var err error
					compose, err = bhruntime.DetectCompose(ctx)
					return err
				}},
				{Name: "compose configuration", Run: func(ctx context.Context) error {
					return compose.ConfigProject(ctx, application.RuntimeProjectName(m), files.Compose, files.Env)
				}},
				{Name: "runtime ownership", Run: func(ctx context.Context) error {
					var err error
					before, err = application.InspectOwnedRuntimeResources(ctx, compose, m)
					return err
				}},
			}
			results, ok := preflight.Run(checkCtx, checks)
			renderPreflightUX(term, results)
			if !ok {
				return errors.New("application down preflight failed")
			}

			if err := suspendConnectivityForManifest(ctx, compose, m); err != nil {
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
			if tracePlacement, found, err := application.RegisteredProviderPlacement(m, capability.ProviderTempo); err != nil {
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
			if err := logsprovider.StopProvider(ctx, compose, m); err != nil {
				return fmt.Errorf("stop application-scoped logs provider: %w", err)
			}
			if application.RequiresRuntimeBroker(m) {
				if err := stopRuntimeBroker(ctx, compose, m, files); err != nil {
					return err
				}
				term.Result("STOPPED", "runtime-broker", "application runtime broker stopped")
			}

			project := application.RuntimeProjectName(m)
			if err := compose.DownProject(ctx, project, files.Compose, files.Env); err != nil {
				return err
			}
			after, err := application.InspectOwnedRuntimeResources(ctx, compose, m)
			if err != nil {
				return fmt.Errorf("verify application down: %w", err)
			}
			if application.ResourceExists(after, "container") || application.ResourceExists(after, "network") {
				return errors.New("verify application down: container or network still exists")
			}
			for _, volume := range application.ExpectedPersistentRuntimeResources(m) {
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
			name, confirmed, fullReset, err := parseDestroyArgs(args)
			if err != nil {
				return err
			}
			var appArgs []string
			if name != "" {
				appArgs = []string{name}
			}
			resolved, err := resolveApplication(store, appArgs, "destroy")
			if err != nil {
				return err
			}
			m := resolved.Manifest
			term := cli.NewTerminal(ctx, out, errOut)
			term.Header(m.Name, m.Environment)
			if err := application.CheckSupportedRuntimeServices(m); err != nil {
				return err
			}

			files, runtimeErr := application.ExistingRuntimeFiles(resolved.Store, m)
			partialRuntime := false
			if errors.Is(runtimeErr, application.ErrRuntimeNotApplied) {
				files = application.RuntimeFilesFor(resolved.Store, m)
				partialRuntime = true
			} else if runtimeErr != nil {
				return runtimeErr
			}
			composeRequired := runtimeErr == nil || resolved.FromRepository || m.Services.Secrets || application.HasManagedRuntimeServices(m)

			checkCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			var compose bhruntime.Compose
			var existing []bhruntime.ProjectResource
			var platformFiles bhruntime.Files
			destroyOpenBaoScope := false
			checks := []preflight.Check{
				{Name: "manifest", Run: func(context.Context) error { return m.Validate() }},
				{Name: "connectivity policy", Run: func(context.Context) error {
					return application.CheckApplicationConnectivityReleased(m)
				}},
				{Name: "manifest permissions", Run: func(context.Context) error {
					return checkManifestPermissions(resolved.ManifestPath, resolved.FromRepository)
				}},
				{Name: "application workload", Run: func(context.Context) error { return preflightRepositoryWorkload(resolved) }},
			}
			if composeRequired {
				checks = append(checks, preflight.Check{Name: "container runtime + compose", Run: func(ctx context.Context) error {
					var err error
					compose, err = bhruntime.DetectCompose(ctx)
					return err
				}})
			}
			if runtimeErr == nil {
				checks = append(checks,
					preflight.Check{Name: "runtime permissions", Run: func(context.Context) error { return application.CheckRuntimePermissions(files) }},
					preflight.Check{Name: "compose configuration", Run: func(ctx context.Context) error {
						return compose.ConfigProject(ctx, application.RuntimeProjectName(m), files.Compose, files.Env)
					}},
				)
			}
			if application.HasManagedRuntimeServices(m) {
				checks = append(checks, preflight.Check{Name: "runtime ownership", Run: func(ctx context.Context) error {
					var err error
					existing, err = application.InspectOwnedRuntimeResources(ctx, compose, m)
					return err
				}})
			}
			if m.Services.Secrets {
				identity := openbao.ApplicationIdentity{Name: m.Name, Environment: m.Environment}
				checks = append(checks,
					preflight.Check{Name: "OpenBao cleanup state", Run: func(ctx context.Context) error {
						var err error
						platformFiles, err = bhruntime.ExistingFiles("")
						if err != nil {
							return err
						}
						state, err := openbao.Inspect(ctx, compose, platformFiles)
						if err != nil {
							return err
						}
						var required bool
						required, err = openBaoDestroyScopeRequired(state)
						if err != nil {
							return err
						}
						destroyOpenBaoScope = required
						return nil
					}},
					preflight.Check{Name: "OpenBao application scope ownership", Run: func(ctx context.Context) error {
						if !destroyOpenBaoScope {
							return nil
						}
						return openbao.CheckApplicationScopeOwnership(ctx, compose, platformFiles, identity)
					}},
				)
			}
			results, ok := preflight.Run(checkCtx, checks)
			renderPreflightUX(term, results)
			if !ok {
				return errors.New("application destroy preflight failed; nothing was deleted")
			}

			term.Section("Delete plan")
			if partialRuntime {
				fmt.Fprintln(out, "  recovery:   generated runtime definition is incomplete; using ownership-verified cleanup")
			}
			if len(existing) == 0 {
				fmt.Fprintln(out, "  runtime resources: none currently present")
			} else {
				for _, resource := range existing {
					fmt.Fprintf(out, "  %-10s %s\n", resource.Kind+":", resource.Name)
				}
			}
			if resolved.FromRepository {
				if workload, found, err := materializeRepositoryWorkload(resolved, files); err == nil && found {
					fmt.Fprintf(out, "  workload:   %s (containers stopped; application-owned volumes preserved)\n", workload.Compose)
				}
			}
			if m.Services.Secrets {
				if destroyOpenBaoScope {
					fmt.Fprintf(out, "  secrets:    baseharbor/apps/%s/%s\n", m.Name, m.Environment)
				} else {
					fmt.Fprintln(out, "  secrets:    current OpenBao is uninitialized; no application scope exists to delete")
				}
				fmt.Fprintln(out, "  broker:     per-application mTLS Application Runtime Broker")
			}
			if len(m.Exposures) > 0 {
				fmt.Fprintf(out, "  exposure:   %d BaseHarbor-managed HTTP route(s) via application-scoped Caddy provider\n", len(m.Exposures))
			}
			if resolved.FromRepository {
				if policy, policyErr := application.LogsPolicy(m); policyErr == nil && policy.Enabled && policy.Collect[application.LogsSourceApplication] {
					fmt.Fprintln(out, "  logs:       application log registration and BaseHarbor-owned collector state removed according to placement")
				}
			}
			if len(m.Metrics.Sources) > 0 || application.HasRuntimeMetricsPermissions(m) {
				metricsPlacement, found, placementErr := application.RegisteredProviderPlacement(m, capability.ProviderPrometheus)
				if placementErr != nil {
					return placementErr
				}
				if found {
					fmt.Fprintf(out, "  metrics:    registered provider placement %s; application-owned metrics state/trust edges removed according to placement\n", metricsPlacement.Scope)
				} else {
					fmt.Fprintln(out, "  metrics:    no registered provider placement; no provider lifecycle ownership will be assumed")
				}
			}
			appDir := filepath.Join(resolved.Store.Root, m.Name)
			fmt.Fprintf(out, "  state:      %s\n", appDir)
			if resolved.FromRepository {
				fmt.Fprintf(out, "  manifest:   %s (preserved)\n", resolved.ManifestPath)
				repoRoot := filepath.Dir(resolved.ManifestPath)
				if fullReset {
					fmt.Fprintf(out, "  deployment: %s (removed by --full-reset)\n", repositoryInitEnvPath(repoRoot))
					fmt.Fprintf(out, "  local TLS:  %s (removed by --full-reset; external certificate source is never touched)\n", filepath.Join(repoRoot, ".baseharbor", repositoryTLSDirName))
				} else {
					fmt.Fprintf(out, "  deployment: %s (preserved)\n", repositoryInitEnvPath(repoRoot))
					fmt.Fprintf(out, "  local TLS:  %s (preserved)\n", filepath.Join(repoRoot, ".baseharbor", repositoryTLSDirName))
				}
			}
			if !confirmed {
				fmt.Fprintln(out, "No changes were made. Re-run with --yes to permanently delete BaseHarbor-managed resources.")
				return nil
			}

			if runtimeErr == nil {
				if err := destroyManagedExposure(ctx, compose, m, files); err != nil {
					return err
				}
			}
			if resolved.FromRepository {
				if _, err := stopRepositoryWorkload(ctx, compose, resolved, files); err != nil {
					return err
				}
			}
			if runtimeErr == nil && application.RequiresRuntimeBroker(m) {
				if err := stopRuntimeBroker(ctx, compose, m, files); err != nil {
					return err
				}
			}
			if runtimeErr == nil {
				if err := compose.DestroyProject(ctx, application.RuntimeProjectName(m), files.Compose, files.Env); err != nil {
					return err
				}
			} else if partialRuntime && len(existing) != 0 {
				if err := compose.DestroyOwnedProjectResources(ctx, application.RuntimeProjectName(m), application.ExpectedRuntimeResources(m)); err != nil {
					return fmt.Errorf("recover incomplete application runtime destruction: %w", err)
				}
			}
			if application.HasManagedRuntimeServices(m) {
				remaining, err := application.InspectOwnedRuntimeResources(ctx, compose, m)
				if err != nil {
					return fmt.Errorf("verify application runtime destruction: %w", err)
				}
				if len(remaining) != 0 {
					return fmt.Errorf("verify application runtime destruction: %d managed resources remain", len(remaining))
				}
			}
			if application.HasObjectStorage(m) {
				driver := objectstorage.NewDriver(compose, m, files)
				for _, bucket := range application.ObjectStorageBucketNames(m) {
					if err := driver.DestroyBucket(ctx, bucket); err != nil {
						return fmt.Errorf("destroy managed S3 bucket %s: %w", bucket, err)
					}
				}
			}
			if m.Services.Secrets && destroyOpenBaoScope {
				identity := openbao.ApplicationIdentity{Name: m.Name, Environment: m.Environment}
				if err := openbao.DestroyVerifiedApplicationScope(ctx, compose, platformFiles, identity); err != nil {
					return fmt.Errorf("destroy OpenBao application scope after runtime removal: %w", err)
				}
			}
			if resolved.FromRepository {
				if err := logsprovider.UnregisterApplication(ctx, compose, m); err != nil {
					return fmt.Errorf("remove application log collector registration: %w", err)
				}
				if err := logsprovider.RemoveWorkloadOverride(files); err != nil {
					return fmt.Errorf("remove workload logging override: %w", err)
				}
			}
			tracePlacement, traceFound, err := application.RegisteredProviderPlacement(m, capability.ProviderTempo)
			if err != nil {
				return err
			}
			if traceFound && tracePlacement.Scope == capability.ScopeApplication {
				if err := tracesprovider.DestroyProvider(ctx, compose, m); err != nil {
					return fmt.Errorf("destroy application-scoped traces provider: %w", err)
				}
			}
			metricsPlacement, found, err := application.RegisteredProviderPlacement(m, capability.ProviderPrometheus)
			if err != nil {
				return err
			}
			if found {
				switch metricsPlacement.Scope {
				case capability.ScopeShared:
					if err := metricsprovider.PruneRegisteredApplicationTargets(m, nil); err != nil {
						return fmt.Errorf("remove application metrics targets: %w", err)
					}
					if err := metricsprovider.UnregisterSharedApplication(ctx, compose, m); err != nil {
						return fmt.Errorf("remove application metrics trust edges: %w", err)
					}
				case capability.ScopeApplication:
					if err := metricsprovider.DestroyProvider(ctx, compose, m); err != nil {
						return fmt.Errorf("destroy application-scoped metrics provider: %w", err)
					}
				case capability.ScopeExternal:
				}
			}
			if err := resolved.Store.Delete(m.Name); err != nil {
				return err
			}
			if fullReset && resolved.FromRepository {
				repoRoot := filepath.Dir(resolved.ManifestPath)
				if err := os.Remove(repositoryInitEnvPath(repoRoot)); err != nil && !errors.Is(err, os.ErrNotExist) {
					return fmt.Errorf("remove repository deployment state: %w", err)
				}
				if err := os.RemoveAll(filepath.Join(repoRoot, ".baseharbor", repositoryTLSDirName)); err != nil {
					return fmt.Errorf("remove normalized repository TLS state: %w", err)
				}
			}
			if err := application.ReleaseApplicationProviderRegistry(m); err != nil {
				return fmt.Errorf("application resources were destroyed but provider registry cleanup failed: %w", err)
			}
			if _, err := os.Stat(appDir); !errors.Is(err, os.ErrNotExist) {
				if err == nil {
					return errors.New("verify application destruction: application state still exists")
				}
				return fmt.Errorf("verify application destruction: %w", err)
			}
			term.Section("Application")
			term.Result("DELETED", "application", m.Name+" permanently deleted")
			if resolved.FromRepository {
				term.Info("repository", "baseharbor.yaml and application-owned Compose data preserved; use 'baha app apply' to recreate")
			}
			return nil
		},
	}
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
