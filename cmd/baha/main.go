package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/mcpdev80/baseharbor/internal/cli"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if os.Getenv("BASEHARBOR_RUNTIME_IMAGE") == "" {
		_ = os.Setenv("BASEHARBOR_RUNTIME_IMAGE", defaultRuntimeImage(version))
	}

	err := runWithIO(ctx, os.Args[1:], os.Stdout, os.Stderr)
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

func defaultRuntimeImage(buildVersion string) string {
	versionTag := strings.TrimSpace(strings.TrimPrefix(buildVersion, "v"))
	if versionTag == "" || versionTag == "dev" || strings.HasPrefix(versionTag, "dev-") || strings.Contains(versionTag, "dirty") {
		versionTag = "edge"
	}
	return "ghcr.io/mcpdev80/baseharbor-runtime:" + versionTag
}

func run(args []string) error {
	return runWithIO(context.Background(), args, os.Stdout, os.Stderr)
}

func runWithIO(ctx context.Context, args []string, out, errOut io.Writer) error {
	return rootCommand().Execute(ctx, args, out, errOut)
}
