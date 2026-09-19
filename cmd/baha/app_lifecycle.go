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
	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/capability"
	metricsprovider "github.com/mcpdev80/baseharbor/internal/metrics"
	"github.com/mcpdev80/baseharbor/internal/objectstorage"
	"github.com/mcpdev80/baseharbor/internal/openbao"
	"github.com/mcpdev80/baseharbor/internal/preflight"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
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
			preflight.Format(out, results)
			if !ok {
				return errors.New("application down preflight failed")
			}

			if len(m.Exposures) > 0 {
				if err := stopManagedExposure(ctx, compose, m, files); err != nil {
					return err
				}
				fmt.Fprintln(out, "[OK] managed-exposure  Caddy exposure provider stopped")
			}
			if err := metricsprovider.StopProvider(ctx, compose, m); err != nil {
				return fmt.Errorf("stop application-scoped metrics provider: %w", err)
			}
			if stopped, err := stopRepositoryWorkload(ctx, compose, resolved, files); err != nil {
				return err
			} else if stopped {
				fmt.Fprintln(out, "[OK] workload          repository Compose workload stopped; application-owned volumes preserved")
			}
			if application.RequiresRuntimeBroker(m) {
				if err := stopRuntimeBroker(ctx, compose, m, files); err != nil {
					return err
				}
				fmt.Fprintln(out, "[OK] runtime-broker    per-application Application Runtime Broker stopped")
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
			fmt.Fprintf(out, "Application %s is stopped. Persistent data is preserved.\n", m.Name)
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
			if err := application.CheckSupportedRuntimeServices(m); err != nil {
				return err
			}

			files, runtimeErr := application.ExistingRuntimeFiles(resolved.Store, m)
			if runtimeErr != nil && !errors.Is(runtimeErr, application.ErrRuntimeNotApplied) {
				return runtimeErr
			}
			if m.Services.Secrets && runtimeErr != nil {
				return errors.New("managed OpenBao secrets are enabled but the application runtime state is incomplete; refusing destroy")
			}

			checkCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			var compose bhruntime.Compose
			var existing []bhruntime.ProjectResource
			var platformFiles bhruntime.Files
			checks := []preflight.Check{
				{Name: "manifest", Run: func(context.Context) error { return m.Validate() }},
				{Name: "manifest permissions", Run: func(context.Context) error {
					return checkManifestPermissions(resolved.ManifestPath, resolved.FromRepository)
				}},
				{Name: "application workload", Run: func(context.Context) error { return preflightRepositoryWorkload(resolved) }},
			}
			if runtimeErr == nil {
				checks = append(checks,
					preflight.Check{Name: "runtime permissions", Run: func(context.Context) error { return application.CheckRuntimePermissions(files) }},
					preflight.Check{Name: "managed runtime definition", Run: func(context.Context) error { return application.CheckManagedRuntimeDefinition(files, m) }},
					preflight.Check{Name: "container runtime + compose", Run: func(ctx context.Context) error {
						var err error
						compose, err = bhruntime.DetectCompose(ctx)
						return err
					}},
					preflight.Check{Name: "compose configuration", Run: func(ctx context.Context) error {
						return compose.ConfigProject(ctx, application.RuntimeProjectName(m), files.Compose, files.Env)
					}},
					preflight.Check{Name: "runtime ownership", Run: func(ctx context.Context) error {
						var err error
						existing, err = application.InspectOwnedRuntimeResources(ctx, compose, m)
						return err
					}},
				)
			}
			if m.Services.Secrets {
				identity := openbao.ApplicationIdentity{Name: m.Name, Environment: m.Environment}
				checks = append(checks,
					preflight.Check{Name: "OpenBao control-plane runtime", Run: func(context.Context) error {
						var err error
						platformFiles, err = bhruntime.ExistingFiles("")
						return err
					}},
					preflight.Check{Name: "OpenBao application scope", Run: func(ctx context.Context) error {
						if platformFiles.Compose == "" {
							return errors.New("BaseHarbor OpenBao runtime is not materialized")
						}
						return openbao.InspectApplicationScope(ctx, compose, platformFiles, identity, openbao.ApplicationCredentialsPath(files.Dir))
					}},
				)
			}
			results, ok := preflight.Run(checkCtx, checks)
			preflight.Format(out, results)
			if !ok {
				return errors.New("application destroy preflight failed; nothing was deleted")
			}

			fmt.Fprintf(out, "Destroy plan for %s (%s)\n", m.Name, m.Environment)
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
				fmt.Fprintf(out, "  secrets:    baseharbor/apps/%s/%s\n", m.Name, m.Environment)
				fmt.Fprintln(out, "  broker:     per-application mTLS Application Runtime Broker")
			}
			if len(m.Exposures) > 0 {
				fmt.Fprintf(out, "  exposure:   %d BaseHarbor-managed HTTP route(s) via application-scoped Caddy provider\n", len(m.Exposures))
			}
			if len(m.Metrics.Sources) > 0 {
				fmt.Fprintf(out, "  metrics:    %d application metrics target(s) removed from shared provider state\n", len(m.Metrics.Sources))
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
				if _, err := stopRepositoryWorkload(ctx, compose, resolved, files); err != nil {
					return err
				}
				if application.RequiresRuntimeBroker(m) {
					if err := stopRuntimeBroker(ctx, compose, m, files); err != nil {
						return err
					}
				}
				if err := compose.DestroyProject(ctx, application.RuntimeProjectName(m), files.Compose, files.Env); err != nil {
					return err
				}
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
			if m.Services.Secrets {
				identity := openbao.ApplicationIdentity{Name: m.Name, Environment: m.Environment}
				if err := openbao.DestroyVerifiedApplicationScope(ctx, compose, platformFiles, identity); err != nil {
					return fmt.Errorf("destroy OpenBao application scope after runtime removal: %w", err)
				}
			}
			metricsPolicy, err := application.MetricsPolicy(m)
			if err != nil {
				return err
			}
			switch metricsPolicy.ProviderScope {
			case capability.ScopeShared:
				if err := metricsprovider.PruneApplicationTargets(m, nil); err != nil {
					return fmt.Errorf("remove application metrics targets: %w", err)
				}
			case capability.ScopeApplication:
				if err := metricsprovider.DestroyProvider(ctx, compose, m); err != nil {
					return fmt.Errorf("destroy application-scoped metrics provider: %w", err)
				}
			case capability.ScopeExternal:
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
			fmt.Fprintf(out, "Application %s was permanently destroyed.\n", m.Name)
			if resolved.FromRepository {
				fmt.Fprintln(out, "Repository baseharbor.yaml and application-owned Compose data were preserved; run 'baha app apply' to recreate the backend.")
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
