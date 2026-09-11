package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

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

type appUpdateOptions struct {
	Check              bool
	BackupPasswordFile string
	NoBackup           bool
}

func appUpdateCommand(store application.Store) *cli.Command {
	return &cli.Command{
		Name:    "update",
		Summary: "Safely inspect or update a Git-backed application",
		Usage:   "baha app update [--check] [--backup-password-file FILE | --no-backup]",
		Long:    "Fetches only the configured upstream remote and verifies repository, branch, revision and working-tree state before any mutation. --check reports the update plan without changing source. For applications with durable managed PostgreSQL or secrets, mutation requires either an encrypted pre-update recovery point through --backup-password-file FILE or an explicit --no-backup acknowledgement. The source update is strict fast-forward-only to the exact fetched target revision, followed by the normal application apply/readiness lifecycle. BaseHarbor never resets, stashes, discards local changes, switches branches, merges divergent history or rebases implicitly.",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			opts, err := parseAppUpdateOptions(args)
			if err != nil {
				return err
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
			if opts.Check {
				return nil
			}
			if state.Dirty {
				return errors.New("automatic application update is blocked because the Git working tree is dirty; commit or otherwise resolve local changes yourself")
			}
			switch state.Relation {
			case "up-to-date":
				fmt.Fprintf(out, "Application %s is already up to date.\n", resolved.Manifest.Name)
				return nil
			case "ahead":
				return errors.New("automatic application update is blocked because the local branch is ahead of its upstream")
			case "diverged":
				return errors.New("automatic application update is blocked because the local branch has diverged from its upstream")
			case "update-available":
			default:
				return fmt.Errorf("automatic application update is blocked for unsupported Git relation %q", state.Relation)
			}

			var backup application.BackupMetadata
			if applicationUpdateHasDurableState(resolved.Manifest) {
				switch {
				case opts.BackupPasswordFile != "":
					backup, err = createApplicationUpdateRecoveryPoint(ctx, store, resolved, opts.BackupPasswordFile, out, errOut)
					if err != nil {
						return fmt.Errorf("create pre-update recovery point: %w", err)
					}
				case opts.NoBackup:
					fmt.Fprintln(out, "WARNING: proceeding without a pre-update recovery point by explicit --no-backup request.")
				default:
					return usageError("application has durable managed state; automatic update requires a pre-update recovery choice", "Use --backup-password-file FILE to create an encrypted recovery point, or explicitly acknowledge the risk with --no-backup.")
				}
			}

			if err := fastForwardGitApplicationUpdate(ctx, state); err != nil {
				return err
			}
			fmt.Fprintf(out, "Source fast-forwarded: %s -> %s\n", state.Current, state.Target)
			fmt.Fprintln(out, "Re-reading application contract and reconciling runtime...")
			if err := appApplyCommand(store).Run(ctx, nil, out, errOut); err != nil {
				metadata := newApplicationUpdateMetadata(resolved, state, "runtime-verification-failed", backup)
				if metadataErr := resolved.Store.RecordLastUpdate(metadata); metadataErr != nil {
					return errors.Join(fmt.Errorf("application source advanced to %s but runtime reconciliation/verification failed: %w", state.Target, err), fmt.Errorf("record failed application update metadata: %w", metadataErr))
				}
				return fmt.Errorf("application source advanced to %s but runtime reconciliation/verification failed: %w", state.Target, err)
			}
			metadata := newApplicationUpdateMetadata(resolved, state, "ready", backup)
			if err := resolved.Store.RecordLastUpdate(metadata); err != nil {
				return fmt.Errorf("application reached READY after update but recording update metadata failed: %w", err)
			}
			fmt.Fprintf(out, "Application %s updated successfully: %s -> %s\n", resolved.Manifest.Name, state.Current, state.Target)
			return nil
		},
	}
}

func parseAppUpdateOptions(args []string) (appUpdateOptions, error) {
	var opts appUpdateOptions
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--check":
			opts.Check = true
		case "--no-backup":
			opts.NoBackup = true
		case "--backup-password-file":
			i++
			if i >= len(args) || strings.TrimSpace(args[i]) == "" {
				return appUpdateOptions{}, usageError("--backup-password-file requires a file path", "Run 'baha app update --help' for usage.")
			}
			opts.BackupPasswordFile = args[i]
		default:
			return appUpdateOptions{}, usageError("unknown baha app update option: "+args[i], "Run 'baha app update --help' for usage.")
		}
	}
	if opts.NoBackup && opts.BackupPasswordFile != "" {
		return appUpdateOptions{}, usageError("--no-backup and --backup-password-file cannot be used together", "Choose exactly one recovery policy for the update.")
	}
	if opts.Check && (opts.NoBackup || opts.BackupPasswordFile != "") {
		return appUpdateOptions{}, usageError("--check does not accept backup mutation options", "Run 'baha app update --check' alone for a read-only update inspection.")
	}
	return opts, nil
}

func applicationUpdateHasDurableState(m application.Manifest) bool {
	return len(application.PostgresInstanceNames(m)) > 0 || m.Services.Secrets
}

func createApplicationUpdateRecoveryPoint(ctx context.Context, store application.Store, resolved resolvedApplication, passwordFile string, out, errOut io.Writer) (application.BackupMetadata, error) {
	if strings.TrimSpace(passwordFile) == "" {
		return application.BackupMetadata{}, errors.New("backup password file is required")
	}
	backupDir := filepath.Join(filepath.Dir(resolved.Store.Root), "backups")
	if err := os.MkdirAll(backupDir, 0o700); err != nil {
		return application.BackupMetadata{}, fmt.Errorf("create pre-update backup directory: %w", err)
	}
	outputPath := filepath.Join(backupDir, fmt.Sprintf("%s-%s-pre-update-%s.bhbackup", resolved.Manifest.Name, resolved.Manifest.Environment, time.Now().UTC().Format("20060102T150405Z")))
	if err := appBackupCommandWithMetadata(store).Run(ctx, []string{"--output", outputPath, "--password-file", passwordFile}, out, errOut); err != nil {
		return application.BackupMetadata{}, err
	}
	metadata, err := resolved.Store.LastBackup(resolved.Manifest.Name)
	if err != nil {
		return application.BackupMetadata{}, fmt.Errorf("load pre-update backup metadata: %w", err)
	}
	absolute, err := filepath.Abs(outputPath)
	if err == nil && filepath.Clean(metadata.ArchivePath) != filepath.Clean(absolute) {
		return application.BackupMetadata{}, fmt.Errorf("pre-update backup metadata path mismatch: expected %s, recorded %s", absolute, metadata.ArchivePath)
	}
	fmt.Fprintf(out, "Pre-update recovery point: %s\n", metadata.ArchivePath)
	return metadata, nil
}

func newApplicationUpdateMetadata(resolved resolvedApplication, state gitUpdateState, result string, backup application.BackupMetadata) application.UpdateMetadata {
	metadata := application.UpdateMetadata{
		Version:      application.LastUpdateMetadataVersion,
		Application:  resolved.Manifest.Name,
		Environment:  resolved.Manifest.Environment,
		UpdatedAt:    time.Now().UTC(),
		Branch:       state.Branch,
		Upstream:     state.Upstream,
		FromRevision: state.Current,
		ToRevision:   state.Target,
		Result:       result,
	}
	if backup.ArchivePath != "" {
		metadata.BackupPath = backup.ArchivePath
		metadata.BackupCreatedAt = backup.CreatedAt
	}
	return metadata
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

	return gitUpdateState{RepositoryRoot: root, Branch: branch, Upstream: upstream, Remote: remote, Current: current, Target: target, Dirty: strings.TrimSpace(status) != "", Relation: relation}, nil
}

func fastForwardGitApplicationUpdate(ctx context.Context, state gitUpdateState) error {
	if state.Dirty {
		return errors.New("refusing Git fast-forward with a dirty working tree")
	}
	if state.Relation != "update-available" {
		return fmt.Errorf("refusing Git fast-forward for relation %q", state.Relation)
	}
	if state.Current == "" || state.Target == "" {
		return errors.New("refusing Git fast-forward without explicit current and target revisions")
	}
	current, err := gitOutput(ctx, state.RepositoryRoot, "rev-parse", "HEAD")
	if err != nil {
		return fmt.Errorf("verify current Git revision immediately before update: %w", err)
	}
	if strings.TrimSpace(current) != state.Current {
		return fmt.Errorf("Git HEAD changed after preflight; expected %s, found %s", state.Current, strings.TrimSpace(current))
	}
	status, err := gitOutput(ctx, state.RepositoryRoot, "status", "--porcelain=v1", "--untracked-files=normal")
	if err != nil {
		return fmt.Errorf("verify Git working tree immediately before update: %w", err)
	}
	if strings.TrimSpace(status) != "" {
		return errors.New("Git working tree changed after preflight; automatic update aborted")
	}
	if !gitIsAncestor(ctx, state.RepositoryRoot, state.Current, state.Target) {
		return errors.New("fetched target is no longer a strict fast-forward descendant of the current revision")
	}
	if _, err := gitOutput(ctx, state.RepositoryRoot, "merge", "--ff-only", state.Target); err != nil {
		return fmt.Errorf("fast-forward application source to %s: %w", state.Target, err)
	}
	head, err := gitOutput(ctx, state.RepositoryRoot, "rev-parse", "HEAD")
	if err != nil {
		return fmt.Errorf("verify Git revision after fast-forward: %w", err)
	}
	if strings.TrimSpace(head) != state.Target {
		return fmt.Errorf("Git fast-forward ended at unexpected revision %s; expected %s", strings.TrimSpace(head), state.Target)
	}
	status, err = gitOutput(ctx, state.RepositoryRoot, "status", "--porcelain=v1", "--untracked-files=normal")
	if err != nil {
		return fmt.Errorf("verify Git working tree after fast-forward: %w", err)
	}
	if strings.TrimSpace(status) != "" {
		return errors.New("Git working tree is not clean after fast-forward")
	}
	return nil
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
	fmt.Fprintln(out, "No application source changes were made during preflight.")
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
