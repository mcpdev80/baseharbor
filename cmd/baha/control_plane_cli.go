package main

import (
	"context"
	"github.com/mcpdev80/baseharbor/internal/cli"
	"io"
)

func controlPlaneDownCLI(ctx context.Context, args []string, out, errOut io.Writer) error {
	for _, arg := range args {
		if arg == "--yes" || arg == "-y" {
			return &cli.UsageError{Message: "down does not accept " + arg, Hint: "Use baha down; down preserves persistent data and does not require --yes.", NoSuggestions: true}
		}
	}
	filtered, format, err := parseReadOutputArgs(args, "down")
	if err != nil {
		return err
	}
	if len(filtered) != 0 {
		return usageError("down does not accept positional arguments", "Use --json for structured results.")
	}
	progress := out
	if format == outputJSON {
		progress = io.Discard
	}
	if err := runtimeDown(ctx, progress); err != nil {
		return err
	}
	if format == outputJSON {
		result, err := inspectControlPlane(ctx)
		if err != nil {
			return err
		}
		return writeJSON(out, result)
	}
	return nil
}
