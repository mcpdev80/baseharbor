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
	"github.com/mcpdev80/baseharbor/internal/openbao"
	"github.com/mcpdev80/baseharbor/internal/preflight"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func appDownCommand(store application.Store) *cli.Command {
	return &cli.Command{
		Name:    "down",
		Summary: "Stop an application runtime while preserving persistent data",
		Usage:   "baha app down [NAME]",
		Long:    "Stops a repository application workload and its per-application runtime secret broker first, then removes BaseHarbor-managed backend containers and transient network while preserving persistent data volumes, runtime credentials, application-owned Compose volumes and managed OpenBao scope. Without NAME it resolves the nearest repository baseharbor.yaml.",
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

			if stopped, err := stopRepositoryWorkload(ctx, compose, resolved, files); err != nil {
				return err
			} else if stopped {
				fmt.Fprintln(out, "[OK] workload          repository Compose workload stopped; application-owned volumes preserved")
			}
			if m.Services.Secrets {
				if err := stopRuntimeBroker(ctx, compose, m, files); err != nil {
					return err
				}
				fmt.Fprintln(out, "[OK] secret-broker     per-application runtime secret broker stopped")
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
		Usage:   "baha app destroy [NAME] [--yes]",
		Long:    "Shows an ownership-verified destruction plan. With --yes it stops any repository workload and per-application secret broker, removes BaseHarbor-managed runtime resources, volumes, OpenBao scope and .baseharbor state. Application-owned Compose volumes and a repository-owned baseharbor.yaml are preserved.",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			name, confirmed, err := parseDestroyArgs(args)
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
				fmt.Fprintln(out, "  broker:     per-application mTLS secret broker")
			}
			appDir := filepath.Join(resolved.Store.Root, m.Name)
			fmt.Fprintf(out, "  state:      %s\n", appDir)
			if resolved.FromRepository {
				fmt.Fprintf(out, "  manifest:   %s (preserved)\n", resolved.ManifestPath)
			}
			if !confirmed {
				fmt.Fprintln(out, "No changes were made. Re-run with --yes to permanently delete BaseHarbor-managed resources.")
				return nil
			}

			if runtimeErr == nil {
				if _, err := stopRepositoryWorkload(ctx, compose, resolved, files); err != nil {
					return err
				}
				if m.Services.Secrets {
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
			if m.Services.Secrets {
				identity := openbao.ApplicationIdentity{Name: m.Name, Environment: m.Environment}
				if err := openbao.DestroyVerifiedApplicationScope(ctx, compose, platformFiles, identity); err != nil {
					return fmt.Errorf("destroy OpenBao application scope after runtime removal: %w", err)
				}
			}
			if err := resolved.Store.Delete(m.Name); err != nil {
				return err
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

func parseDestroyArgs(args []string) (string, bool, error) {
	var name string
	confirmed := false
	for _, arg := range args {
		switch {
		case arg == "--yes":
			confirmed = true
		case strings.HasPrefix(arg, "-"):
			return "", false, usageError("unknown option "+arg, "Run 'baha app destroy --help' for available options.")
		default:
			if name != "" {
				return "", false, usageError("baha app destroy accepts at most one NAME", "Inside an application repository omit NAME.")
			}
			name = arg
		}
	}
	return name, confirmed, nil
}
