package development

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

const WorkspaceGitStatusVersion = "baseharbor.workspace-git-status/v1"

type WorkspaceGitState string

const (
	WorkspaceGitCurrent         WorkspaceGitState = "current"
	WorkspaceGitUpdateAvailable WorkspaceGitState = "update_available"
	WorkspaceGitDirty           WorkspaceGitState = "dirty"
	WorkspaceGitAhead           WorkspaceGitState = "ahead"
	WorkspaceGitDiverged        WorkspaceGitState = "diverged"
	WorkspaceGitDetached        WorkspaceGitState = "detached"
	WorkspaceGitNoUpstream      WorkspaceGitState = "no_upstream"
	WorkspaceGitUnavailable     WorkspaceGitState = "unavailable"
	WorkspaceGitNotRepository   WorkspaceGitState = "not_repository"
)

type WorkspaceGitRepositoryStatus struct {
	Source           string            `json:"source"`
	Path             string            `json:"path"`
	State            WorkspaceGitState `json:"state"`
	Branch           string            `json:"branch,omitempty"`
	Upstream         string            `json:"upstream,omitempty"`
	Remote           string            `json:"remote,omitempty"`
	Relation         string            `json:"relation,omitempty"`
	Ahead            int               `json:"ahead,omitempty"`
	Behind           int               `json:"behind,omitempty"`
	CurrentRevision  string            `json:"current_revision,omitempty"`
	TargetRevision   string            `json:"target_revision,omitempty"`
	DeclaredRef      string            `json:"declared_ref,omitempty"`
	DeclaredRevision string            `json:"declared_revision,omitempty"`
	Dirty            bool              `json:"dirty,omitempty"`
	UpdateSafe       bool              `json:"update_safe"`
	Updated          bool              `json:"updated,omitempty"`
	Blocker          string            `json:"blocker,omitempty"`
	Problem          string            `json:"problem,omitempty"`
	NextAction       string            `json:"next_action,omitempty"`
}

type WorkspaceGitStatus struct {
	SchemaVersion string                         `json:"schema_version"`
	Repositories  []WorkspaceGitRepositoryStatus `json:"repositories"`
}

type WorkspaceGitUpdateResult struct {
	SchemaVersion string                         `json:"schema_version"`
	CheckOnly     bool                           `json:"check_only"`
	Repositories  []WorkspaceGitRepositoryStatus `json:"repositories"`
}

func InspectWorkspaceGit(ctx context.Context, model SourceModel, mapping WorkspaceMapping, fetch bool) (WorkspaceGitStatus, error) {
	if err := model.Validate(); err != nil {
		return WorkspaceGitStatus{}, err
	}
	status := WorkspaceGitStatus{SchemaVersion: WorkspaceGitStatusVersion}
	for _, source := range model.Sources {
		if source.Type != SourceRepository {
			continue
		}
		path := strings.TrimSpace(mapping.Sources[source.ID])
		item := WorkspaceGitRepositoryStatus{Source: source.ID, Path: path}
		if path == "" {
			item.State = WorkspaceGitUnavailable
			item.Blocker = "unmapped"
			item.Problem = "repository source is not mapped to a local worktree"
			item.NextAction = "map the source with 'baha app workspace map " + source.ID + " PATH'"
			status.Repositories = append(status.Repositories, item)
			continue
		}
		abs, err := filepath.Abs(path)
		if err != nil {
			return WorkspaceGitStatus{}, err
		}
		item.Path = abs
		inspected := inspectWorkspaceRepository(ctx, abs, fetch)
		inspected.Source = source.ID
		inspected.DeclaredRef = strings.TrimSpace(source.Ref)
		if inspected.DeclaredRef != "" {
			inspected.DeclaredRevision = workspaceGitResolveDeclaredRevision(ctx, abs, inspected.DeclaredRef, inspected.Upstream)
		}
		status.Repositories = append(status.Repositories, inspected)
	}
	sort.Slice(status.Repositories, func(i, j int) bool { return status.Repositories[i].Source < status.Repositories[j].Source })
	return status, nil
}

func UpdateWorkspaceGit(ctx context.Context, model SourceModel, mapping WorkspaceMapping, check bool) (WorkspaceGitUpdateResult, error) {
	status, err := InspectWorkspaceGit(ctx, model, mapping, true)
	if err != nil {
		return WorkspaceGitUpdateResult{}, err
	}
	result := WorkspaceGitUpdateResult{SchemaVersion: WorkspaceGitStatusVersion, CheckOnly: check}
	for _, item := range status.Repositories {
		if check || !item.UpdateSafe {
			result.Repositories = append(result.Repositories, item)
			continue
		}
		before := item.CurrentRevision
		if _, err := workspaceGitOutput(ctx, item.Path, "merge", "--ff-only", item.TargetRevision); err != nil {
			item.State = WorkspaceGitUnavailable
			item.UpdateSafe = false
			item.Blocker = "fast_forward_failed"
			item.Problem = "fast-forward update failed: " + err.Error()
			item.NextAction = "review the repository with native Git and run the workspace update again"
			result.Repositories = append(result.Repositories, item)
			continue
		}
		after := inspectWorkspaceRepository(ctx, item.Path, false)
		after.Source = item.Source
		after.DeclaredRef = item.DeclaredRef
		after.DeclaredRevision = item.DeclaredRevision
		if after.State != WorkspaceGitCurrent {
			after.Blocker = "post_update_state_unexpected"
			after.Problem = "repository changed but did not reach a clean current state"
			after.NextAction = "review the repository with native Git"
			result.Repositories = append(result.Repositories, after)
			continue
		}
		after.CurrentRevision = strings.TrimSpace(after.CurrentRevision)
		if before == after.CurrentRevision {
			after.Problem = "fast-forward reported success but revision did not change"
			after.NextAction = "review the repository with native Git"
		} else {
			after.Updated = true
		}
		result.Repositories = append(result.Repositories, after)
	}
	return result, nil
}

func inspectWorkspaceRepository(ctx context.Context, root string, fetch bool) WorkspaceGitRepositoryStatus {
	item := WorkspaceGitRepositoryStatus{Path: root}
	top, err := workspaceGitOutput(ctx, root, "rev-parse", "--show-toplevel")
	if err != nil {
		item.State = WorkspaceGitNotRepository
		item.Blocker = "not_repository"
		item.Problem = "mapped path is not a Git worktree"
		item.NextAction = "verify the workspace mapping or initialize/clone the repository with Git"
		return item
	}
	topAbs, err := filepath.Abs(strings.TrimSpace(top))
	if err != nil || filepath.Clean(topAbs) != filepath.Clean(root) {
		item.State = WorkspaceGitNotRepository
		item.Blocker = "not_repository_root"
		item.Problem = "workspace mapping must point to the Git worktree root"
		item.NextAction = "map the repository root rather than a nested component directory"
		return item
	}
	status, err := workspaceGitOutput(ctx, root, "status", "--porcelain=v1", "--untracked-files=normal")
	if err != nil {
		item.State = WorkspaceGitUnavailable
		item.Blocker = "status_failed"
		item.Problem = "cannot inspect Git working tree: " + err.Error()
		item.NextAction = "run 'git status' in the repository"
		return item
	}
	item.Dirty = strings.TrimSpace(status) != ""

	branch, err := workspaceGitOutput(ctx, root, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil || strings.TrimSpace(branch) == "" {
		item.State = WorkspaceGitDetached
		item.Blocker = "detached_head"
		item.Problem = "repository is not on a named branch"
		item.NextAction = "use Git to switch to the intended branch"
		return item
	}
	item.Branch = strings.TrimSpace(branch)

	upstream, err := workspaceGitOutput(ctx, root, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{upstream}")
	if err != nil || strings.TrimSpace(upstream) == "" {
		item.State = WorkspaceGitNoUpstream
		item.Blocker = "missing_upstream"
		item.Problem = "current branch has no configured upstream"
		item.NextAction = "review 'git branch -vv' and configure the intended upstream with Git"
		return item
	}
	item.Upstream = strings.TrimSpace(upstream)
	item.Remote = strings.SplitN(item.Upstream, "/", 2)[0]

	if fetch {
		remote := item.Remote
		if remote == "" || remote == item.Upstream {
			item.State = WorkspaceGitUnavailable
			item.Blocker = "invalid_upstream"
			item.Problem = "cannot determine remote from configured upstream"
			item.NextAction = "review 'git branch -vv' and 'git remote -v'"
			return item
		}
		if _, err := workspaceGitOutput(ctx, root, "fetch", "--prune", "--", remote); err != nil {
			item.State = WorkspaceGitUnavailable
			item.Blocker = classifyGitFetchBlocker(err)
			if item.Blocker == "authentication_failed" {
				item.Problem = "Git could not authenticate with the configured remote"
				item.NextAction = "run 'git fetch' directly and review your normal SSH agent, Git credential helper or platform credential manager"
			} else {
				item.Problem = "Git fetch failed: " + err.Error()
				item.NextAction = "run 'git fetch' directly to review connectivity or authentication"
			}
			return item
		}
	}

	current, err := workspaceGitOutput(ctx, root, "rev-parse", "HEAD")
	if err != nil {
		item.State = WorkspaceGitUnavailable
		item.Blocker = "current_revision_unavailable"
		item.Problem = "cannot read current revision: " + err.Error()
		item.NextAction = "run 'git status' in the repository"
		return item
	}
	target, err := workspaceGitOutput(ctx, root, "rev-parse", "@{upstream}")
	if err != nil {
		item.State = WorkspaceGitUnavailable
		item.Blocker = "upstream_revision_unavailable"
		item.Problem = "cannot read upstream revision: " + err.Error()
		item.NextAction = "run 'git fetch' and 'git branch -vv'"
		return item
	}
	item.CurrentRevision = strings.TrimSpace(current)
	item.TargetRevision = strings.TrimSpace(target)
	item.Ahead, item.Behind = workspaceGitAheadBehind(ctx, root)
	switch {
	case item.CurrentRevision == item.TargetRevision:
		item.Relation = "current"
	case item.Ahead > 0 && item.Behind > 0:
		item.Relation = "diverged"
	case item.Ahead > 0:
		item.Relation = "ahead"
	case item.Behind > 0:
		item.Relation = "behind"
	default:
		item.Relation = "unknown"
	}

	if item.Dirty {
		item.State = WorkspaceGitDirty
		item.Blocker = "dirty_worktree"
		item.Problem = "repository contains local changes"
		item.NextAction = "review 'git status' and decide with Git whether to commit, stash or discard them"
		return item
	}
	if item.CurrentRevision == item.TargetRevision {
		item.State = WorkspaceGitCurrent
		return item
	}
	switch {
	case workspaceGitIsAncestor(ctx, root, item.CurrentRevision, item.TargetRevision):
		item.State = WorkspaceGitUpdateAvailable
		item.UpdateSafe = true
	case workspaceGitIsAncestor(ctx, root, item.TargetRevision, item.CurrentRevision):
		item.State = WorkspaceGitAhead
		item.Blocker = "ahead_only"
		item.Problem = "local branch contains commits not present upstream"
		item.NextAction = "review 'git status' and your normal team Git workflow"
	default:
		item.State = WorkspaceGitDiverged
		item.Blocker = "diverged"
		item.Problem = "local and upstream history have diverged"
		item.NextAction = "review 'git log --oneline --left-right HEAD...@{u}' and resolve with your normal Git workflow"
	}
	return item
}

func classifyGitFetchBlocker(err error) string {
	if err == nil {
		return ""
	}
	text := strings.ToLower(err.Error())
	for _, signal := range []string{
		"authentication failed",
		"permission denied",
		"publickey",
		"could not read username",
		"terminal prompts disabled",
		"repository not found",
		"access denied",
	} {
		if strings.Contains(text, signal) {
			return "authentication_failed"
		}
	}
	return "fetch_failed"
}

func workspaceGitAheadBehind(ctx context.Context, dir string) (ahead, behind int) {
	value, err := workspaceGitOutput(ctx, dir, "rev-list", "--left-right", "--count", "HEAD...@{upstream}")
	if err != nil {
		return 0, 0
	}
	fields := strings.Fields(value)
	if len(fields) != 2 {
		return 0, 0
	}
	fmt.Sscanf(fields[0], "%d", &ahead)
	fmt.Sscanf(fields[1], "%d", &behind)
	return ahead, behind
}

func workspaceGitResolveDeclaredRevision(ctx context.Context, dir, ref, upstream string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ""
	}
	candidates := []string{ref}
	if !strings.Contains(ref, "/") && strings.Contains(upstream, "/") {
		remote := strings.SplitN(upstream, "/", 2)[0]
		if remote != "" {
			candidates = append([]string{remote + "/" + ref}, candidates...)
		}
	}
	for _, candidate := range candidates {
		value, err := workspaceGitOutput(ctx, dir, "rev-parse", "--verify", candidate+"^{commit}")
		if err == nil && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func workspaceGitOutput(ctx context.Context, dir string, args ...string) (string, error) {
	path, err := exec.LookPath("git")
	if err != nil {
		return "", errors.New("git executable not found")
	}
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = err.Error()
		}
		return "", fmt.Errorf("%s", detail)
	}
	return strings.TrimSpace(stdout.String()), nil
}

func workspaceGitIsAncestor(ctx context.Context, dir, ancestor, descendant string) bool {
	path, err := exec.LookPath("git")
	if err != nil {
		return false
	}
	cmd := exec.CommandContext(ctx, path, "merge-base", "--is-ancestor", ancestor, descendant)
	cmd.Dir = dir
	return cmd.Run() == nil
}
