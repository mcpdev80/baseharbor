package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/mcpdev80/baseharbor/internal/cli"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	err := runWithIO(context.Background(), os.Args[1:], os.Stdout, os.Stderr)
	if err == nil {
		return
	}
	fmt.Fprintln(os.Stderr, "error:", err)
	var usage *cli.UsageError
	if errors.As(err, &usage) && usage.Hint != "" {
		fmt.Fprintln(os.Stderr, "hint:", usage.Hint)
	}
	os.Exit(cli.ExitCode(err))
}

func run(args []string) error {
	return runWithIO(context.Background(), args, os.Stdout, os.Stderr)
}

func runWithIO(ctx context.Context, args []string, out, errOut io.Writer) error {
	return rootCommand().Execute(ctx, args, out, errOut)
}
