package main

import (
	"context"
	"io"
)

func controlPlaneDownCLI(ctx context.Context, args []string, out, errOut io.Writer) error {
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
