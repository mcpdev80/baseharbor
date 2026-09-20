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
	formatCLIError(os.Stderr, err)
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
	filtered, opts, err := extractGlobalOutputOptions(args)
	if err != nil {
		return err
	}
	ctx = cli.WithOutputOptions(ctx, opts)
	return rootCommand().Execute(ctx, filtered, out, errOut)
}

func extractGlobalOutputOptions(args []string) ([]string, cli.OutputOptions, error) {
	opts := cli.OutputOptions{}
	filtered := make([]string, 0, len(args))
	passthrough := false
	for _, arg := range args {
		if passthrough {
			filtered = append(filtered, arg)
			continue
		}
		if arg == "--" {
			passthrough = true
			filtered = append(filtered, arg)
			continue
		}
		switch arg {
		case "-q", "--quiet", "--silent":
			opts.Quiet = true
		case "-v", "--verbose":
			opts.Verbose = true
		case "--no-color":
			opts.NoColor = true
		default:
			filtered = append(filtered, arg)
		}
	}
	if opts.Quiet && opts.Verbose {
		return nil, opts, usageError("--quiet and --verbose cannot be used together", "Choose concise output or diagnostic output, not both.")
	}
	if value := strings.TrimSpace(os.Getenv("BASEHARBOR_REDUCED_MOTION")); value != "" && value != "0" && !strings.EqualFold(value, "false") {
		opts.ReducedMotion = true
	}
	return filtered, opts, nil
}

func formatCLIError(w io.Writer, err error) {
	if cli.IsPresented(err) {
		return
	}
	fmt.Fprintf(w, "Error: %v\n", err)
	var usage *cli.UsageError
	if errors.As(err, &usage) {
		if strings.TrimSpace(usage.Hint) != "" {
			fmt.Fprintln(w, "\nNext:")
			fmt.Fprintf(w, "  %s\n", usage.Hint)
		}
		return
	}
	fmt.Fprintln(w, "\nNext:")
	fmt.Fprintln(w, "  baha doctor")
	fmt.Fprintln(w, "  Retry with --verbose for diagnostic runtime details.")
}
