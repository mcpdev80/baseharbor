package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/applicationlifecycle"
	"github.com/mcpdev80/baseharbor/internal/cli"
)

func appRestoreCommandWithRecoveryMetadata(store application.Store) *cli.Command {
	command := appRestoreCommand(store)
	restoreRun := command.Run
	command.Run = func(ctx context.Context, args []string, out, errOut io.Writer) error {
		filtered, format, err := parseReadOutputArgs(args, "app restore")
		if err != nil {
			return err
		}
		selection, environment, err := extractApplicationEnvironment(filtered, "restore")
		if err != nil {
			return err
		}
		_, name, _, err := parseAppRestoreArgs(selection)
		if err != nil {
			return err
		}
		presentation := out
		if format == outputJSON {
			ctx = machineNoninteractiveContext(ctx)
			presentation = io.Discard
		}
		if err := restoreRun(ctx, filtered, presentation, errOut); err != nil {
			return err
		}
		if format == outputJSON {
			status, err := collectApplicationStatusResult(ctx, store, machineApplicationArgs(name, environment))
			if err != nil {
				return err
			}
			return writeJSON(out, machineLifecycleStatusResult{Result: applicationlifecycle.NewResult("restore", status.Application, status.Environment, status.State, status.Ready).WithTarget(status.Target), Status: status})
		}
		fmt.Fprintln(out, "Status: READY")
		return nil
	}
	return command
}

func appShowCommandWithRecoveryMetadata(store application.Store) *cli.Command {
	command := appShowCommand(store)
	showRun := command.Run
	command.Long = "Shows application identity, backend readiness, repository workload state, secret readiness, the last recorded successful backup and the last verified recovery without revealing secret values or credential-bearing URLs. It uses the same repository workload readiness model as app status and app doctor."
	command.Run = func(ctx context.Context, args []string, out, errOut io.Writer) error {
		filtered, format, err := parseReadOutputArgs(args, "app show")
		if err != nil {
			return err
		}
		if format == outputJSON {
			return showRun(ctx, args, out, errOut)
		}
		args = filtered
		resolved, err := resolveApplication(ctx, store, args, "show")
		if err != nil {
			return err
		}
		if err := showRun(ctx, args, out, errOut); err != nil {
			return err
		}
		fmt.Fprintln(out, "\nLast recovery")
		metadata, err := resolved.Store.LastRecovery(resolved.Manifest.Name)
		if errors.Is(err, application.ErrNoRecoveryMetadata) {
			fmt.Fprintln(out, "  not recorded yet")
			return nil
		}
		if err != nil {
			return err
		}
		if metadata.Environment != resolved.Manifest.Environment {
			fmt.Fprintln(out, "  not recorded for this environment")
			return nil
		}
		fmt.Fprintf(out, "  restored             %s\n", metadata.RestoredAt.UTC().Format(time.RFC3339))
		fmt.Fprintf(out, "  backup created       %s\n", metadata.BackupCreatedAt.UTC().Format(time.RFC3339))
		fmt.Fprintf(out, "  archive              %s\n", metadata.ArchivePath)
		fmt.Fprintln(out, "  verification         READY")
		return nil
	}
	return command
}
