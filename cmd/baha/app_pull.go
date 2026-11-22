package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/development"
)

// appPullOptions carries the flags for 'baha app pull'.
type appPullOptions struct {
	Components   []string
	Check        bool
	Up           bool
	ManifestArg  string
	OutputFormat cliOutputFormat
}

// appPullCommand registers 'baha app pull', the unified guided source pull entry
// point. It selects repository/local sources by component, pulls each selected
// source entirely (whole repository, strict fast-forward), and, with --up,
// reuses the same app-up operation that 'baha app update' runs. It never
// stashes, resets, switches branches, rebases, merges divergent history or
// resolves conflicts.
func appPullCommand(store application.Store) *cli.Command {
	return &cli.Command{
		Name:    "pull",
		Summary: "Safely pull Git source for the application, optionally bringing it up",
		Usage:   "baha app pull [-c COMPONENT[,COMPONENT...]] [--up] [--check] [--manifest PATH] [-o json|--output json]",
		Long:    "Selects repository or local sources by component and pulls each selected source as a strict fast-forward to its upstream, inspecting working-tree, branch, upstream and divergence first. The working tree must be clean; a dirty, ahead or diverged source blocks that source without touching others. --check previews the pull plan without changing source. With --up the application is brought up using the same app-up operation as 'baha app update', only after every selected source pulled successfully. Non-interactive: a blocker is an actionable error, never a prompt. No secret values are printed.",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			opts, err := parseAppPullOptions(args)
			if err != nil {
				return err
			}
			return runAppPull(ctx, store, opts, out, errOut)
		},
	}
}

func parseAppPullOptions(args []string) (appPullOptions, error) {
	var opts appPullOptions
	opts.ManifestArg = "."
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--check":
			opts.Check = true
		case arg == "--up":
			opts.Up = true
		case arg == "--component" || strings.HasPrefix(arg, "--component="):
			value := strings.TrimPrefix(arg, "--component=")
			if arg == "--component" {
				i++
				if i >= len(args) || strings.TrimSpace(args[i]) == "" {
					return appPullOptions{}, usageError("--component requires a value", "Pass one or more comma-separated component names, e.g. --component api,web.")
				}
				value = args[i]
			}
			for _, component := range strings.Split(value, ",") {
				if strings.TrimSpace(component) == "" {
					continue
				}
				opts.Components = append(opts.Components, strings.TrimSpace(component))
			}
		case strings.HasPrefix(arg, "-c"):
			value := strings.TrimPrefix(arg, "-c")
			if value == "" {
				if i+1 >= len(args) || strings.TrimSpace(args[i+1]) == "" {
					return appPullOptions{}, usageError("-c requires a value", "Pass one or more comma-separated component names, e.g. -c api,web.")
				}
				i++
				value = args[i]
			}
			for _, component := range strings.Split(value, ",") {
				if strings.TrimSpace(component) == "" {
					continue
				}
				opts.Components = append(opts.Components, strings.TrimSpace(component))
			}
		case arg == "-o" || arg == "--output":
			if i+1 >= len(args) || args[i+1] != "json" {
				return appPullOptions{}, usageError("--output supports only json", "Use --output json.")
			}
			i++
			opts.OutputFormat = "json"
		case strings.HasPrefix(arg, "--output="):
			value := strings.TrimPrefix(arg, "--output=")
			if value != "json" {
				return appPullOptions{}, usageError("--output supports only json", "Use --output json.")
			}
			opts.OutputFormat = "json"
		case arg == "--manifest":
			if i+1 >= len(args) || strings.TrimSpace(args[i+1]) == "" {
				return appPullOptions{}, usageError("--manifest requires a path", "Provide a baseharbor.yaml path or repository directory.")
			}
			i++
			opts.ManifestArg = args[i]
		case strings.HasPrefix(arg, "--manifest="):
			opts.ManifestArg = strings.TrimPrefix(arg, "--manifest=")
		default:
			if strings.HasPrefix(arg, "-") {
				return appPullOptions{}, unknownOptionUsage("baha app pull", arg, "-c", "--component", "--up", "--check", "--manifest", "-o", "--output")
			}
			return appPullOptions{}, usageError("baha app pull accepts no positional arguments", "Select sources with -c/--component, bring the app up with --up, or preview with --check.")
		}
	}
	if opts.Check && opts.Up {
		return appPullOptions{}, usageError("--check and --up cannot be used together", "--check previews; --up starts the application.")
	}
	if opts.OutputFormat != "" && opts.OutputFormat != "json" {
		return appPullOptions{}, usageError("--output supports only json", "Use --output json.")
	}
	return opts, nil
}

// runAppPull resolves the application, loads the source model and workspace
// mapping, and drives the guided pull. With --up it only calls the app-up
// lifecycle after every selected source pulled successfully, so a partial pull
// never triggers an apply.
func runAppPull(ctx context.Context, store application.Store, opts appPullOptions, out, errOut io.Writer) error {
	resolved, err := resolveApplication(ctx, store, nil, "pull")
	if err != nil {
		return err
	}
	if !resolved.FromRepository {
		return fmt.Errorf("%w: 'baha app pull' requires a repository-owned baseharbor.yaml at the Git repository root", development.ErrSourcePullBlocked)
	}
	model, mapping, err := loadSourceModelAndMapping(resolved)
	if err != nil {
		return err
	}
	manifestPath := resolved.ManifestPath
	plan, err := development.ResolveSourcePullPlan(ctx, manifestPath, model, mapping, opts.Components)
	if err != nil {
		return err
	}
	if err != nil {
		return err
	}
	if opts.Check {
		if opts.OutputFormat == outputJSON {
			return writeJSON(out, plan)
		}
		writeSourcePullPlanCheck(out, &plan)
		return nil
	}
	result, err := development.ApplySourcePull(ctx, plan, false)
	if err != nil {
		return err
	}
	if !result.Successful {
		return fmt.Errorf("%w: %s", development.ErrSourcePullBlocked, sourcePullFailureText(&result))
	}
	if opts.Up {
		applyOut, applyErrOut := &bytes.Buffer{}, &bytes.Buffer{}
		if err := bringApplicationUpAfterPull(ctx, store, resolved, applyOut, applyErrOut); err != nil {
			return err
		}
		// With --up there is exactly one JSON document to emit: the composite
		// (pull result + apply output under "up"). Emitting the bare result
		// first (as the non--up path does) would concatenate a second JSON
		// value onto the same writer and break the single-document contract
		// that structured clients parse; the MCP handler unmarshals the combined
		// buffer as one value, so a double write fails decoding there too.
		if opts.OutputFormat == outputJSON {
			return emitSourcePullUpComposite(out, result, applyOut.String())
		}
		if err := writeSourcePullResult(out, &result); err != nil {
			return err
		}
		fmt.Fprintln(out, "")
		return writeSourcePullUpHuman(out, &result, applyOut.String())
	}
	if opts.OutputFormat == outputJSON {
		return writeJSON(out, result)
	}
	return writeSourcePullResult(out, &result)
}

// emitSourcePullUpComposite returns one valid JSON document for `baha app pull
// --up --output json`: the pull result plus the apply output under "up". It
// keeps the two JSON values the apply lifecycle and the pull result each emit
// in separate buffers and merges them, so the structured output always decodes
// as a single object instead of two concatenated JSON values.
func emitSourcePullUpComposite(out io.Writer, result development.SourcePullResult, upOutput string) error {
	composite := map[string]any{
		"schema_version": development.SourcePullSchemaVersion,
		"pull":           result,
		"up": map[string]any{
			"ran":    true,
			"output": strings.TrimSpace(upOutput),
		},
	}
	return writeJSON(out, composite)
}
