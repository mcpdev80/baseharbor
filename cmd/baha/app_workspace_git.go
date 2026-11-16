package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/development"
)

func appWorkspaceStatusCommand() *cli.Command {
	return &cli.Command{
		Name:    "status",
		Summary: "Inspect Git state for all mapped repository sources",
		Usage:   "baha app workspace status [--manifest PATH] [--fetch] [-o json|--output json]",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			manifestArg, format, fetch, err := parseWorkspaceGitArgs(args, false)
			if err != nil {
				return err
			}
			manifestPath, manifest, err := resolveWorkspaceManifest(manifestArg)
			if err != nil {
				return err
			}
			model, _, err := development.LoadSourceModel(manifestPath)
			if err != nil {
				return err
			}
			mapping, _, err := development.LoadWorkspaceMapping(manifestPath, manifest.Name)
			if err != nil {
				if os.IsNotExist(err) {
					return usageError("workspace mapping is missing", "Map repository sources with 'baha app workspace map SOURCE PATH'.")
				}
				return err
			}
			status, err := development.InspectWorkspaceGit(ctx, model, mapping, fetch)
			if err != nil {
				return err
			}
			if format == outputJSON {
				return writeJSON(out, status)
			}
			return writeWorkspaceGitStatus(out, status.Repositories, false, cli.OutputOptionsFromContext(ctx).Verbose)
		},
	}
}

func appWorkspaceUpdateCommand() *cli.Command {
	return &cli.Command{
		Name:    "update",
		Summary: "Safely fast-forward mapped Git repositories",
		Usage:   "baha app workspace update [--manifest PATH] [--check] [-o json|--output json]",
		Long:    "Fetches each mapped repository using native Git. Clean branches with a configured upstream are updated only when a strict fast-forward is possible. BaseHarbor never stashes, resets, switches branches, rebases, merges divergent history or resolves conflicts.",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			manifestArg, format, check, err := parseWorkspaceGitArgs(args, true)
			if err != nil {
				return err
			}
			manifestPath, manifest, err := resolveWorkspaceManifest(manifestArg)
			if err != nil {
				return err
			}
			model, _, err := development.LoadSourceModel(manifestPath)
			if err != nil {
				return err
			}
			mapping, _, err := development.LoadWorkspaceMapping(manifestPath, manifest.Name)
			if err != nil {
				if os.IsNotExist(err) {
					return usageError("workspace mapping is missing", "Map repository sources with 'baha app workspace map SOURCE PATH'.")
				}
				return err
			}
			result, err := development.UpdateWorkspaceGit(ctx, model, mapping, check)
			if err != nil {
				return err
			}
			if format == outputJSON {
				return writeJSON(out, result)
			}
			return writeWorkspaceGitStatus(out, result.Repositories, check, cli.OutputOptionsFromContext(ctx).Verbose)
		},
	}
}

func parseWorkspaceGitArgs(args []string, update bool) (manifestArg string, format cliOutputFormat, flag bool, err error) {
	manifestArg = "."
	format = outputHuman
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--manifest":
			if i+1 >= len(args) || strings.TrimSpace(args[i+1]) == "" {
				return "", "", false, usageError("--manifest requires a path", "Provide a baseharbor.yaml path or repository directory.")
			}
			i++
			manifestArg = args[i]
		case "-o", "--output":
			if i+1 >= len(args) || args[i+1] != "json" {
				return "", "", false, usageError("--output supports only json", "Use --output json.")
			}
			i++
			format = outputJSON
		case "--fetch":
			if update {
				return "", "", false, unknownOptionUsage("baha app workspace update", args[i], "--manifest", "--check", "--output")
			}
			flag = true
		case "--check":
			if !update {
				return "", "", false, unknownOptionUsage("baha app workspace status", args[i], "--manifest", "--fetch", "--output")
			}
			flag = true
		default:
			command := "baha app workspace status"
			allowed := []string{"--manifest", "--fetch", "--output"}
			if update {
				command = "baha app workspace update"
				allowed = []string{"--manifest", "--check", "--output"}
			}
			return "", "", false, unknownOptionUsage(command, args[i], allowed...)
		}
	}
	return manifestArg, format, flag, nil
}

func writeWorkspaceGitStatus(out io.Writer, repositories []development.WorkspaceGitRepositoryStatus, check, verbose bool) error {
	title := "Workspace status"
	if check {
		title = "Workspace update check"
	} else {
		title = "Workspace update/status"
	}
	fmt.Fprintln(out, title)
	fmt.Fprintln(out)

	current := 0
	updated := 0
	actionRequired := 0
	for _, repo := range repositories {
		fmt.Fprintln(out, repo.Source)
		switch repo.State {
		case development.WorkspaceGitCurrent:
			current++
			fmt.Fprintln(out, "  CURRENT")
			if repo.Updated {
				updated++
				fmt.Fprintln(out, "  New changes were applied.")
			} else if !check {
				fmt.Fprintln(out, "  No new changes.")
			}
		case development.WorkspaceGitUpdateAvailable:
			if check {
				fmt.Fprintln(out, "  UPDATE AVAILABLE")
			} else {
				fmt.Fprintln(out, "  UPDATE AVAILABLE")
			}
		default:
			actionRequired++
			fmt.Fprintln(out, "  ACTION REQUIRED")
		}
		if repo.Problem != "" {
			fmt.Fprintf(out, "  %s\n", repo.Problem)
		}
		if repo.NextAction != "" {
			fmt.Fprintf(out, "  Next: %s\n", repo.NextAction)
		}
		if verbose {
			if repo.Path != "" {
				fmt.Fprintf(out, "  Path: %s\n", repo.Path)
			}
			if repo.Branch != "" {
				fmt.Fprintf(out, "  Branch: %s\n", repo.Branch)
			}
			if repo.Upstream != "" {
				fmt.Fprintf(out, "  Upstream: %s\n", repo.Upstream)
			}
			if repo.CurrentRevision != "" {
				fmt.Fprintf(out, "  Current revision: %s\n", repo.CurrentRevision)
			}
			if repo.TargetRevision != "" {
				fmt.Fprintf(out, "  Upstream revision: %s\n", repo.TargetRevision)
			}
			if repo.DeclaredRef != "" {
				fmt.Fprintf(out, "  Declared ref: %s\n", repo.DeclaredRef)
			}
			if repo.DeclaredRevision != "" {
				fmt.Fprintf(out, "  Declared revision: %s\n", repo.DeclaredRevision)
			}
		}
		fmt.Fprintln(out)
	}
	fmt.Fprintln(out, "Summary")
	if updated > 0 {
		fmt.Fprintf(out, "  %d repositories updated\n", updated)
	}
	fmt.Fprintf(out, "  %d repositories are current\n", current)
	if actionRequired > 0 {
		fmt.Fprintf(out, "  %d repositories need your attention\n", actionRequired)
		if updated > 0 {
			fmt.Fprintln(out, "  Changes already applied were not rolled back.")
		}
	}
	return nil
}
