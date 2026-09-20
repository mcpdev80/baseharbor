package main

import (
	"context"
	"fmt"
	"io"

	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/preflight"
)

func renderPreflightUX(term *cli.Terminal, results []preflight.Result) {
	term.Section("Preflight")
	for _, result := range results {
		if result.OK {
			detail := ""
			if term.Verbose() {
				detail = result.Detail
			}
			term.Success(result.Name, detail)
			continue
		}
		term.Fail(result.Name, result.Detail)
	}
}

func activity(ctx context.Context, term *cli.Terminal, label string, fn func(io.Writer) error) error {
	if err := term.Activity(ctx, label, fn); err != nil {
		return fmt.Errorf("%s: %w", label, err)
	}
	return nil
}

func noInput(ctx context.Context) bool {
	return cli.OutputOptionsFromContext(ctx).NonInteractive
}
