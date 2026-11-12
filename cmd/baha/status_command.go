package main

import (
	"context"
	"fmt"
	"io"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/cli"
)

func rootStatusCommand(store application.Store) *cli.Command {
	return &cli.Command{
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
			target, err := effectiveTarget(ctx)
			if err != nil {
				return err
			}
			if remoteCoreTarget(target) {
				result, err := remoteControlPlaneStatus(ctx, target)
				if err != nil {
					return err
				}
				if format == outputJSON {
					return writeJSON(out, result)
				}
				return renderRemoteControlPlaneStatus(ctx, target, result, out)
			}
			if format == outputJSON {
				result, err := inspectControlPlane(ctx)
				if err != nil {
					return err
				}
				return writeJSON(out, result)
			}
			fmt.Fprintf(out, "Selection source: %s (%s)\n", target.Name, targetSelectionOrigin(ctx))
			return runtimeStatus(ctx, out)
		},
	}
}
