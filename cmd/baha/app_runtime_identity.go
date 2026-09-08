package main

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/cli"
)

func appRuntimeIdentityCommand(store application.Store) *cli.Command {
	command := &cli.Command{
		Name:    "runtime-identity",
		Summary: "Manage the app-scoped credential used for dynamic secret references",
		Usage:   "baha app runtime-identity <rotate|revoke> [NAME] [--yes]",
		Long:    "Rotates or revokes the generated application runtime credential without changing any stored baseharbor:// secret references. The credential is an application identity only and never grants operator API access.",
	}
	command.Children = []*cli.Command{
		{
			Name:    "rotate",
			Summary: "Rotate the application runtime credential",
			Usage:   "baha app runtime-identity rotate [NAME] [--yes]",
			Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
				resolved, confirmed, err := resolveRuntimeIdentityMutation(store, args, "rotate")
				if err != nil {
					return err
				}
				if !confirmed {
					fmt.Fprintf(out, "Runtime identity for %s would be rotated. No changes were made. Re-run with --yes.\n", resolved.Manifest.Name)
					return nil
				}
				files, err := runtimeIdentityFiles(resolved)
				if err != nil {
					return err
				}
				if err := application.RotateRuntimeIdentity(resolved.Manifest, files); err != nil {
					return err
				}
				fmt.Fprintf(out, "Rotated runtime identity for %s. Stored secret references are unchanged.\n", resolved.Manifest.Name)
				return nil
			},
		},
		{
			Name:    "revoke",
			Summary: "Immediately revoke the application runtime credential",
			Usage:   "baha app runtime-identity revoke [NAME] [--yes]",
			Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
				resolved, confirmed, err := resolveRuntimeIdentityMutation(store, args, "revoke")
				if err != nil {
					return err
				}
				if !confirmed {
					fmt.Fprintf(out, "Runtime identity for %s would be revoked. No changes were made. Re-run with --yes.\n", resolved.Manifest.Name)
					return nil
				}
				files, err := runtimeIdentityFiles(resolved)
				if err != nil {
					return err
				}
				if err := application.RevokeRuntimeIdentity(resolved.Manifest, files); err != nil {
					return err
				}
				fmt.Fprintf(out, "Revoked runtime identity for %s. Rotate it to restore dynamic secret access.\n", resolved.Manifest.Name)
				return nil
			},
		},
	}
	return command
}

func resolveRuntimeIdentityMutation(store application.Store, args []string, action string) (resolvedApplication, bool, error) {
	name, confirmed, err := parseRuntimeIdentityMutationArgs(args)
	if err != nil {
		return resolvedApplication{}, false, err
	}
	var appArgs []string
	if name != "" {
		appArgs = []string{name}
	}
	resolved, err := resolveApplication(store, appArgs, "runtime-identity "+action)
	if err != nil {
		return resolvedApplication{}, false, err
	}
	if !resolved.Manifest.Services.Secrets {
		return resolvedApplication{}, false, errors.New("application does not enable managed secrets")
	}
	return resolved, confirmed, nil
}

func runtimeIdentityFiles(resolved resolvedApplication) (application.RuntimeFiles, error) {
	files, err := application.ExistingRuntimeFiles(resolved.Store, resolved.Manifest)
	if err != nil {
		return application.RuntimeFiles{}, err
	}
	if err := application.CheckRuntimePermissions(files); err != nil {
		return application.RuntimeFiles{}, err
	}
	return files, nil
}

func parseRuntimeIdentityMutationArgs(args []string) (string, bool, error) {
	var name string
	confirmed := false
	for _, arg := range args {
		switch arg {
		case "--yes":
			confirmed = true
		default:
			if len(arg) > 0 && arg[0] == '-' {
				return "", false, usageError("unknown runtime-identity option "+arg, "Use only an optional NAME and --yes.")
			}
			if name != "" {
				return "", false, usageError("runtime-identity accepts at most one NAME", "Run inside a repository or pass one application NAME.")
			}
			name = arg
		}
	}
	return name, confirmed, nil
}
