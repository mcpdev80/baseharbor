package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/development"
)

// loadSourceModelAndMapping reads the source model and workspace mapping for a
// resolved repository application. The pull operates on the whole repository,
// so the manifest must live at the Git worktree root.
func loadSourceModelAndMapping(resolved resolvedApplication) (development.SourceModel, development.WorkspaceMapping, error) {
	if resolved.repositoryRoot() == "" {
		return development.SourceModel{}, development.WorkspaceMapping{}, fmt.Errorf("cannot resolve application repository root")
	}
	model, _, err := development.LoadSourceModel(resolved.ManifestPath)
	if err != nil {
		return development.SourceModel{}, development.WorkspaceMapping{}, err
	}
	mapping, _, err := development.LoadWorkspaceMapping(resolved.ManifestPath, resolved.Manifest.Name)
	if err != nil {
		return development.SourceModel{}, development.WorkspaceMapping{}, err
	}
	return model, mapping, nil
}

// bringApplicationUpAfterPull reuses the same app-up operation 'baha app update'
// runs. It is only called after a fully successful pull, so the workspace is
// already at the target revisions when the application is brought up.
func bringApplicationUpAfterPull(ctx context.Context, store application.Store, resolved resolvedApplication, out, errOut io.Writer) error {
	return executeApplicationApplyLifecycle(ctx, store, nil, out, errOut)
}

func writeSourcePullPlanCheck(out io.Writer, plan *development.SourcePullPlan) {
	fmt.Fprintln(out, "Source pull preview")
	fmt.Fprintln(out)
	fmt.Fprintf(out, "Application: %s\n", plan.Application)
	fmt.Fprintf(out, "Manifest: %s\n", plan.Manifest)
	if len(plan.Components) > 0 {
		fmt.Fprintf(out, "Components: %s\n", joinComponents(plan.Components))
	}
	for _, repo := range plan.Repositories {
		fmt.Fprintln(out, repo.Source)
		switch repo.State {
		case development.SourcePullUpdateAvailable:
			fmt.Fprintln(out, "  UPDATE AVAILABLE")
			fmt.Fprintf(out, "  %s -> %s\n", repo.CurrentRevision, repo.TargetRevision)
		case development.SourcePullCurrent:
			fmt.Fprintln(out, "  CURRENT")
		default:
			fmt.Fprintln(out, "  BLOCKED")
			if repo.Problem != "" {
				fmt.Fprintf(out, "  %s\n", repo.Problem)
			}
			if repo.NextAction != "" {
				fmt.Fprintf(out, "  Next: %s\n", repo.NextAction)
			}
		}
	}
	if !plan.Applyable {
		fmt.Fprintln(out)
		fmt.Fprintln(out, "Pull is blocked; fix the blocked sources or select them with -c/--component.")
	}
}

func writeSourcePullResult(out io.Writer, result *development.SourcePullResult) error {
	fmt.Fprintln(out, "Source pull")
	fmt.Fprintln(out)
	for _, repo := range result.Repositories {
		fmt.Fprintln(out, repo.Source)
		switch {
		case repo.Updated:
			fmt.Fprintln(out, "  UPDATED")
			fmt.Fprintf(out, "  %s -> %s\n", repo.CurrentRevision, repo.TargetRevision)
		case repo.State == development.SourcePullCurrent:
			fmt.Fprintln(out, "  CURRENT")
		default:
			fmt.Fprintln(out, "  BLOCKED")
			if repo.Problem != "" {
				fmt.Fprintf(out, "  %s\n", repo.Problem)
			}
			if repo.NextAction != "" {
				fmt.Fprintf(out, "  Next: %s\n", repo.NextAction)
			}
		}
	}
	if !result.Successful {
		fmt.Fprintln(out)
		fmt.Fprintf(out, "Source pull failed: %s\n", sourcePullFailureText(result))
	} else {
		fmt.Fprintln(out, "Source pull succeeded; all selected sources are current.")
	}
	return nil
}

// writeSourcePullUpHuman renders the human (non-JSON) --up document: the pull
// result followed by the apply output under an "up" heading. Unlike the JSON
// path, which is a single composite object, the human document is the pull
// result and the up output concatenated as two human-readable sections.
func writeSourcePullUpHuman(out io.Writer, result *development.SourcePullResult, upOutput string) error {
	if err := writeSourcePullResult(out, result); err != nil {
		return err
	}
	upTrimmed := strings.TrimSpace(upOutput)
	if upTrimmed == "" {
		upTrimmed = "no apply output"
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Up")
	fmt.Fprintln(out)
	fmt.Fprintln(out, upTrimmed)
	return nil
}

func sourcePullFailureText(result *development.SourcePullResult) string {
	parts := make([]string, 0, len(result.Repositories))
	for _, repo := range result.Repositories {
		if repo.State != development.SourcePullCurrent {
			blocker := repo.Blocker
			if blocker == "" {
				blocker = repo.Problem
			}
			if blocker == "" {
				blocker = string(repo.State)
			}
			parts = append(parts, repo.Source+" ("+blocker+")")
		}
	}
	if len(parts) == 0 {
		return "no repository reached the target revision"
	}
	return joinStrings(parts, ", ")
}

func joinStrings(parts []string, sep string) string {
	out := ""
	for i, part := range parts {
		if i > 0 {
			out += sep
		}
		out += part
	}
	return out
}

func joinComponents(components []string) string {
	out := ""
	for i, component := range components {
		if i > 0 {
			out += ", "
		}
		out += component
	}
	return out
}
