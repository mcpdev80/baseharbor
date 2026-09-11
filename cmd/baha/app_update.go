package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/cli"
)

type gitUpdateState struct {
	RepositoryRoot string
	Branch         string
	Upstream       string
	Remote         string
	Current        string
	Target         string
	Dirty          bool
	Relation       string
}

func appUpdateCommand(store application.Store) *cli.Command {
	return &cli.Command{
		Name:    "update",
		Summary: "Safely inspect or update a Git-backed application",
		Usage:   "baha app update [--check]",
		Long:    "Inspects the current repository Git branch and upstream without changing application source. --check fetches the configured upstream remote, reports current and target revisions, and classifies the working tree as clean or dirty. Automatic mutation remains fail-closed and is added in the next v0.3 update slice; BaseHarbor never resets, stashes, discards changes or switches branches implicitly.",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			check := false
			for _, arg := range args {
				switch arg {
				case "--check":
					check = true
				default:
					return usageError("unknown baha app update option: "+arg, "Run 'baha app update --help' for usage.")
				}
			}
			if !check {
				return usageError("baha app update mutation is not enabled yet", "Run 'baha app update --check' to inspect the safe Git update path.")
			}
			resolved, err := resolveApplication(store, nil, "update")
			if err != nil {
				return err
			}
			if !resolved.FromRepository {
				return errors.New("application update requires a repository-owned baseharbor.yaml")
			}
			state, err := inspectGitApplicationUpdate(ctx, filepath.Dir(resolved.ManifestPath), true)
			if err != nil {
				return err
			}
			formatGitApplicationUpdateCheck(out, resolved.Manifest.Name, resolved.Manifest.Environment, state)
			return nil
		},
	}
}

func inspectGitApplicationUpdate(ctx context.Context, repositoryRoot string, fetch bool) (gitUpdateState, error) {
	root, err := gitOutput(ctx, repositoryRoot, "rev-parse", "--show-toplevel")
	if err != nil {
		return gitUpdateState{}, fmt.Errorf("application repository is not a Git worktree: %w", err)
	}
	root, err = filepath.Abs(strings.TrimSpace(root))
	if err != nil {
		return gitUpdateState{}, fmt.Errorf("resolve Git repository root: %w", err)
	}
	manifestRoot, err := filepath.Abs(repositoryRoot)
	if err != nil {
		return gitUpdateState{}, fmt.Errorf("resolve application repository root: %w", err)
	}
	if filepath.Clean(root) != filepath.Clean(manifestRoot) {
		return gitUpdateState{}, fmt.Errorf("baseharbor.yaml must be at the Git repository root for automatic update; manifest root=%s git root=%s", manifestRoot, root)
	}

	branch, err := gitOutput(ctx, root, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil || strings.TrimSpace(branch) == "" {
		return gitUpdateState{}, errors.New("application update requires a named Git branch; detached HEAD is not updated automatically")
	}
	branch = strings.TrimSpace(branch)

	upstream, err := gitOutput(ctx, root, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{upstream}")
	if err != nil || strings.TrimSpace(upstream) == "" {
		return gitUpdateState{}, fmt.Errorf("Git branch %s has no configured upstream", branch)
	}
	upstream = strings.TrimSpace(upstream)
	remote := strings.SplitN(upstream, "/", 2)[0]
	if remote == "" || remote == upstream {
		return gitUpdateState{}, fmt.Errorf("cannot determine Git remote from upstream %s", upstream)
	}

	if fetch {
		if _, err := gitOutput(ctx, root, "fetch", "--prune", "--", remote); err != nil {
			return gitUpdateState{}, fmt.Errorf("fetch Git remote %s: %w", remote, err)
		}
	}

	current, err := gitOutput(ctx, root, "rev-parse", "HEAD")
	if err != nil {
		return gitUpdateState{}, fmt.Errorf("read current Git revision: %w", err)
	}
	target, err := gitOutput(ctx, root, "rev-parse", "@{upstream}")
	if err != nil {
		return gitUpdateState{}, fmt.Errorf("read upstream Git revision: %w", err)
	}
	status, err := gitOutput(ctx, root, "status", "--porcelain=v1", "--untracked-files=normal")
	if err != nil {
		return gitUpdateState{}, fmt.Errorf("inspect Git working tree: %w", err)
	}

	current = strings.TrimSpace(current)
	target = strings.TrimSpace(target)
	relation := "up-to-date"
	if current != target {
		switch {
		case gitIsAncestor(ctx, root, current, target):
			relation = "update-available"
		case gitIsAncestor(ctx, root, target, current):
			relation = "ahead"
		default:
			relation = "diverged"
		}
	}

	return gitUpdateState{
		RepositoryRoot: root,
		Branch:         branch,
		Upstream:       upstream,
		Remote:         remote,
		Current:        current,
		Target:         target,
		Dirty:          strings.TrimSpace(status) != "",
		Relation:       relation,
	}, nil
}

func formatGitApplicationUpdateCheck(out io.Writer, name, environment string, state gitUpdateState) {
	fmt.Fprintf(out, "Application: %s\n", name)
	fmt.Fprintf(out, "Environment: %s\n", environment)
	fmt.Fprintf(out, "Repository: %s\n", state.RepositoryRoot)
	fmt.Fprintf(out, "Branch: %s\n", state.Branch)
	fmt.Fprintf(out, "Upstream: %s\n", state.Upstream)
	fmt.Fprintf(out, "Current revision: %s\n", state.Current)
	fmt.Fprintf(out, "Target revision: %s\n", state.Target)
	if state.Dirty {
		fmt.Fprintln(out, "Working tree: DIRTY - automatic update blocked; commit or otherwise resolve local changes yourself")
	} else {
		fmt.Fprintln(out, "Working tree: CLEAN")
	}
	switch state.Relation {
	case "up-to-date":
		fmt.Fprintln(out, "Update: up to date")
	case "update-available":
		fmt.Fprintln(out, "Update: available - fast-forward path")
	case "ahead":
		fmt.Fprintln(out, "Update: local branch is ahead of upstream; nothing will be changed automatically")
	case "diverged":
		fmt.Fprintln(out, "Update: branch has diverged; automatic update is blocked")
	default:
		fmt.Fprintf(out, "Update: %s\n", state.Relation)
	}
	fmt.Fprintln(out, "No application source changes were made.")
}

func gitOutput(ctx context.Context, dir string, args ...string) (string, error) {
	path, err := exec.LookPath("git")
	if err != nil {
		return "", errors.New("git executable not found")
	}
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Dir = dir
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = err.Error()
		}
		return "", errors.New(detail)
	}
	return strings.TrimSpace(stdout.String()), nil
}

func gitIsAncestor(ctx context.Context, dir, ancestor, descendant string) bool {
	path, err := exec.LookPath("git")
	if err != nil {
		return false
	}
	cmd := exec.CommandContext(ctx, path, "merge-base", "--is-ancestor", ancestor, descendant)
	cmd.Dir = dir
	return cmd.Run() == nil
}
