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
	"github.com/mcpdev80/baseharbor/internal/preflight"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func appDownCommand(store application.Store) *cli.Command {
	return &cli.Command{
		Name:    "down",
		Summary: "Stop an application runtime while preserving persistent data",
		Usage:   "baha app down NAME",
		Long:    "Stops and removes the application's managed containers and transient network while preserving its managed data volumes, manifest, runtime definition and credentials. Ownership is verified before mutation.",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			if len(args) != 1 {
				return usageError("baha app down requires exactly one NAME", "Example: baha app down demo")
			}
			m, manifestPath, err := store.Load(args[0])
			if err != nil {
				return err
			}
			files, err := application.ExistingRuntimeFiles(store, m)
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
				{Name: "manifest permissions", Run: func(context.Context) error { return ownerOnly(manifestPath) }},
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
		Summary: "Permanently remove a BaseHarbor-managed application runtime and state",
		Usage:   "baha app destroy NAME [--yes]",
		Long:    "Performs a read-only ownership and safety preflight, then shows the exact BaseHarbor-managed resources that would be removed. Without --yes no changes are made. With --yes, owned runtime resources, persistent volumes and application state are permanently deleted and absence is verified.\n\nOptions:\n  --yes  Confirm permanent deletion after the safety preflight",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			name, confirmed, err := parseDestroyArgs(args)
			if err != nil {
				return err
			}
			m, manifestPath, err := store.Load(name)
			if err != nil {
				return err
			}
			if err := application.CheckSupportedRuntimeServices(m); err != nil {
				return err
			}

			files, runtimeErr := application.ExistingRuntimeFiles(store, m)
			if runtimeErr != nil && !errors.Is(runtimeErr, application.ErrRuntimeNotApplied) {
				return runtimeErr
			}

			checkCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			var compose bhruntime.Compose
			var existing []bhruntime.ProjectResource
			checks := []preflight.Check{
				{Name: "manifest", Run: func(context.Context) error { return m.Validate() }},
				{Name: "manifest permissions", Run: func(context.Context) error { return ownerOnly(manifestPath) }},
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
			appDir := filepath.Join(store.Root, m.Name)
			fmt.Fprintf(out, "  state:      %s\n", appDir)
			if !confirmed {
				fmt.Fprintln(out, "No changes were made. Re-run with --yes to permanently delete these BaseHarbor-managed resources.")
				return nil
			}

			if runtimeErr == nil {
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
			if err := store.Delete(m.Name); err != nil {
				return err
			}
			if _, err := os.Stat(appDir); !errors.Is(err, os.ErrNotExist) {
				if err == nil {
					return errors.New("verify application destruction: application state still exists")
				}
				return fmt.Errorf("verify application destruction: %w", err)
			}
			fmt.Fprintf(out, "Application %s was permanently destroyed.\n", m.Name)
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
				return "", false, usageError("baha app destroy accepts exactly one NAME", "Example: baha app destroy demo --yes")
			}
			name = arg
		}
	}
	if name == "" {
		return "", false, usageError("baha app destroy requires NAME", "Example: baha app destroy demo")
	}
	return name, confirmed, nil
}
