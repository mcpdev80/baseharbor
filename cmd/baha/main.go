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
	signal.Ignore(syscall.SIGPIPE)
	ctx, stop := interruptibleProcessContext()
	defer stop()

	if os.Getenv("BASEHARBOR_RUNTIME_IMAGE") == "" {
		_ = os.Setenv("BASEHARBOR_RUNTIME_IMAGE", defaultRuntimeImage(version))
	}

	err := runWithIO(ctx, os.Args[1:], os.Stdout, os.Stderr)
	if err == nil {
		return
	}
	if errors.Is(err, context.Canceled) {
		fmt.Fprintln(os.Stderr, "Interrupted.")
		os.Exit(130)
	}
	formatCLIError(os.Stderr, err)
	os.Exit(cli.ExitCode(err))
}

func interruptibleProcessContext() (context.Context, func()) {
	ctx, cancel := context.WithCancel(context.Background())
	signals := make(chan os.Signal, 2)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)

	go func() {
		seen := false
		for sig := range signals {
			if !seen {
				seen = true
				cancel()
				continue
			}
			signal.Stop(signals)
			if sig == syscall.SIGTERM {
				os.Exit(143)
			}
			os.Exit(130)
		}
	}()

	stop := func() {
		signal.Stop(signals)
		cancel()
	}
	return ctx, stop
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
	filtered, opts, showVersion, err := extractGlobalOutputOptions(args)
	if err != nil {
		return err
	}
	if showVersion {
		if len(filtered) != 0 {
			return usageError("--version cannot be combined with a command", "Run 'baha --version' by itself.")
		}
		fmt.Fprintf(out, "BaseHarbor %s\ncommit %s\nbuilt %s\n", version, commit, date)
		return nil
	}
	ctx = cli.WithOutputOptions(ctx, opts)
	return rootCommand().Execute(ctx, filtered, out, errOut)
}

func extractGlobalOutputOptions(args []string) ([]string, cli.OutputOptions, bool, error) {
	opts := cli.OutputOptions{}
	filtered := make([]string, 0, len(args))
	showVersion := false
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
		case "--plain":
			opts.Plain = true
		case "--no-input", "--non-interactive":
			opts.NonInteractive = true
		case "--version":
			showVersion = true
		default:
			filtered = append(filtered, arg)
		}
	}
	if opts.Quiet && opts.Verbose {
		return nil, opts, showVersion, usageError("--quiet and --verbose cannot be used together", "Choose concise output or diagnostic output, not both.")
	}
	if value := strings.TrimSpace(os.Getenv("BASEHARBOR_REDUCED_MOTION")); value != "" && value != "0" && !strings.EqualFold(value, "false") {
		opts.ReducedMotion = true
	}
	return filtered, opts, showVersion, nil
}

func formatCLIError(w io.Writer, err error) {
	if errors.Is(err, syscall.EPIPE) {
		return
	}
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
