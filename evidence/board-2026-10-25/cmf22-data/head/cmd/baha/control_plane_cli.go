package main

import (
	"context"
	"fmt"
	"github.com/mcpdev80/baseharbor/internal/cli"
	"io"
	"strings"
)

func controlPlaneDownCLI(ctx context.Context, args []string, out, errOut io.Writer) error {
	for _, arg := range args {
		if arg == "--yes" || arg == "-y" {
			return &cli.UsageError{Message: "down does not accept " + arg, Hint: "Review the current Core stop plan and provide both explicit acknowledgements; --yes does not provide them.", NoSuggestions: true}
		}
	}
	filtered, format, err := parseReadOutputArgs(args, "down")
	if err != nil {
		return err
	}
	if len(filtered) != 0 {
		for _, arg := range filtered {
			if strings.HasPrefix(arg, "-") {
				return usageError("down does not accept unknown option "+arg, "Run 'baha down --help'; use --json for structured results.")
			}
		}
		return usageError("down does not accept positional arguments", "Use --json for structured results.")
	}
	progress := out
	if format == outputJSON {
		progress = io.Discard
	}
	result, err := runtimeDownWithResult(ctx, progress)
	if err != nil {
		return err
	}
	if format == outputJSON {
		return writeJSON(out, result)
	}
	fmt.Fprintln(out, "BaseHarbor selected Core services stopped; recovery owners retained. Consumer continuity remains unverified.")
	return nil
}
