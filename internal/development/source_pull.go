package development

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// ErrSourcePullBlocked is returned when preflight found no clean fast-forward
// available for the requested selection. Its repositories carry the blocker
// detail; callers render them as actionable errors, never prompts.
var ErrSourcePullBlocked = errors.New("source pull blocked")

// SourcePullState is the outcome of inspecting one mapped repository for a
// guided source pull. A repository is pullable only when its branch is clean,
// on a named branch with a configured upstream, and strictly behind its upstream
// (a fast-forward is possible). Every other state blocks that repository and,
// per acceptance, never mutates it or the others.
type SourcePullState string

const (
	SourcePullCurrent         SourcePullState = "current"
	SourcePullUpdateAvailable SourcePullState = "update_available"
	SourcePullDirty           SourcePullState = "dirty"
	SourcePullAhead           SourcePullState = "ahead"
	SourcePullDiverged        SourcePullState = "diverged"
	SourcePullDetached        SourcePullState = "detached"
	SourcePullNoUpstream      SourcePullState = "no_upstream"
	SourcePullUnavailable     SourcePullState = "unavailable"
	SourcePullNotRepository   SourcePullState = "not_repository"
)

// SourcePullRepository is the per-repository pull plan plus result.
type SourcePullRepository struct {
	Source          string          `json:"source"`
	Repository      string          `json:"repository"`
	Path            string          `json:"path"`
	State           SourcePullState `json:"state"`
	Branch          string          `json:"branch,omitempty"`
	Upstream        string          `json:"upstream,omitempty"`
	CurrentRevision string          `json:"current_revision,omitempty"`
	TargetRevision  string          `json:"target_revision,omitempty"`
	Dirty           bool            `json:"dirty,omitempty"`
	UpdateAvailable bool            `json:"update_available"`
	Updated         bool            `json:"updated,omitempty"`
	Blocker         string          `json:"blocker,omitempty"`
	Problem         string          `json:"problem,omitempty"`
	NextAction      string          `json:"next_action,omitempty"`
}

// SourcePullPlan is the preflight result: which repositories are pullable and
// whether the whole selection is safe to apply. UpdateAvailable is true only
// when at least one repository wants to advance. Any blocked repository makes
// the selection not applyable.
type SourcePullPlan struct {
	SchemaVersion   string                 `json:"schema_version"`
	Application     string                 `json:"application"`
	Manifest        string                 `json:"manifest"`
	Components      []string               `json:"components,omitempty"`
	UpdateAvailable bool                   `json:"update_available"`
	Applyable       bool                   `json:"applyable"`
	Repositories    []SourcePullRepository `json:"repositories"`
}

// SourcePullResult is the apply result. Successful is true only when every
// repository in the selection advanced; a single partial failure makes the
// whole pull fail so the caller never reports a mixed outcome as success and
// never runs the --up operation.
type SourcePullResult struct {
	SchemaVersion string                 `json:"schema_version"`
	CheckOnly     bool                   `json:"check_only"`
	Successful    bool                   `json:"successful"`
	Repositories  []SourcePullRepository `json:"repositories"`
}

// ResolveSourcePullPlan validates the component request, filters the source
// model and mapping to the requested sources and inspects every mapped
// repository. It never mutates the working tree and never touches a repository
// outside the selection. With no requested components every declared repository
// is inspected.
func ResolveSourcePullPlan(ctx context.Context, manifestPath string, model SourceModel, mapping WorkspaceMapping, requested []string) (SourcePullPlan, error) {
	if err := model.Validate(); err != nil {
		return SourcePullPlan{}, err
	}
	requested = normalizeRequested(requested)

	filteredModel, filteredMapping := model, mapping
	if len(requested) > 0 {
		var err error
		filteredModel, filteredMapping, err = FilterSourcesForComponents(model, mapping, requested)
		if err != nil {
			return SourcePullPlan{}, err
		}
	}

	plan := SourcePullPlan{
		SchemaVersion: SourcePullSchemaVersion,
		Application:   filteredModel.Application,
		Manifest:      manifestPath,
		Components:    requested,
	}
	for _, source := range filteredModel.Sources {
		// OCI sources have no working tree to pull; local directory sources
		// without Git state are skipped exactly like OCI, so a plain
		// development directory never blocks a pull.
		if source.Type != SourceRepository && source.Type != SourceLocal {
			continue
		}
		path := strings.TrimSpace(filteredMapping.Sources[source.ID])
		if source.Type == SourceLocal {
			if path == "" {
				path = strings.TrimSpace(source.Path)
			}
			if path == "" || !workspaceGitRepositoryAt(ctx, path) {
				continue
			}
		}
		item := inspectSourcePullRepository(ctx, path)
		item.Source = source.ID
		item.Repository = strings.TrimSpace(source.Repository)
		item.State = sourcePullState(ctx, item)
		plan.Repositories = append(plan.Repositories, item)
		if item.State == SourcePullUpdateAvailable {
			plan.UpdateAvailable = true
		}
	}
	sort.Slice(plan.Repositories, func(i, j int) bool { return plan.Repositories[i].Source < plan.Repositories[j].Source })
	plan.Applyable = plan.UpdateAvailable && !plan.HasBlocker()
	return plan, nil
}

// ApplySourcePull performs a strict fast-forward on every repository in the
// plan. It is applyable-checked by the caller: it refuses to run when the plan
// is not applyable and mutates nothing when any repository fails, so a partial
// failure never leaves a half-pulled selection. checkOnly inspects only.
func ApplySourcePull(ctx context.Context, plan SourcePullPlan, checkOnly bool) (SourcePullResult, error) {
	if plan.SchemaVersion != SourcePullSchemaVersion {
		return SourcePullResult{}, fmt.Errorf("invalid source pull plan schema %q", plan.SchemaVersion)
	}
	result := SourcePullResult{SchemaVersion: SourcePullSchemaVersion, CheckOnly: checkOnly}
	applyable := plan.UpdateAvailable && !plan.HasBlocker()
	if checkOnly || !applyable {
		result.Repositories = append(result.Repositories, plan.Repositories...)
		result.Successful = plan.Applyable
		return result, nil
	}
	for i := range plan.Repositories {
		item := plan.Repositories[i]
		if _, err := workspaceGitOutput(ctx, item.Path, "merge", "--ff-only", item.TargetRevision); err != nil {
			item.State = SourcePullUnavailable
			item.UpdateAvailable = false
			item.Blocker = "fast_forward_failed"
			item.Problem = "fast-forward update failed: " + err.Error()
			item.NextAction = "review the repository with native Git and run the pull again"
			result.Repositories = append(result.Repositories, item)
			continue
		}
		head, err := workspaceGitOutput(ctx, item.Path, "rev-parse", "HEAD")
		if err != nil || strings.TrimSpace(head) != item.TargetRevision {
			item.State = SourcePullUnavailable
			item.UpdateAvailable = false
			item.Blocker = "post_update_revision_unexpected"
			item.Problem = "repository changed but did not reach the target revision"
			item.NextAction = "review the repository with native Git"
			result.Repositories = append(result.Repositories, item)
			continue
		}
		item.Updated = true
		item.State = SourcePullCurrent
		item.UpdateAvailable = false
		result.Repositories = append(result.Repositories, item)
	}
	// Successful is true only when every repository in the selection advanced or
	// was already current with no blocker. Any single failed repository makes the
	// whole pull fail so --up never runs on a partially pulled selection.
	result.Successful = true
	for _, repo := range result.Repositories {
		if repo.State != SourcePullCurrent || repo.Blocker != "" {
			result.Successful = false
			break
		}
	}
	return result, nil
}

// SourcePullHasFailure reports whether the pull result contained a blocked or
// failed repository. Partial failures are surfaced as a single actionable error
// by the CLI, never as a mixed-success report.
func (r SourcePullResult) SourcePullHasFailure() bool {
	for _, repo := range r.Repositories {
		if repo.State != SourcePullCurrent {
			return true
		}
	}
	return false
}

// HasBlocker reports whether any repository in the plan is blocked from being
// pulled. A blocked repository fails closed: it never mutates and never lets a
// partial selection through.
func (p SourcePullPlan) HasBlocker() bool {
	for _, repo := range p.Repositories {
		if repo.State != SourcePullUpdateAvailable && repo.State != SourcePullCurrent {
			return true
		}
	}
	return false
}

func sourcePullState(ctx context.Context, item SourcePullRepository) SourcePullState {
	if item.Dirty {
		return SourcePullDirty
	}
	if item.CurrentRevision == item.TargetRevision {
		return SourcePullCurrent
	}
	if workspaceGitIsAncestor(ctx, item.Path, item.CurrentRevision, item.TargetRevision) {
		return SourcePullUpdateAvailable
	}
	if workspaceGitIsAncestor(ctx, item.Path, item.TargetRevision, item.CurrentRevision) {
		return SourcePullAhead
	}
	return SourcePullDiverged
}

func normalizeRequested(requested []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(requested))
	for _, value := range requested {
		name := strings.TrimSpace(value)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	return out
}

func inspectSourcePullRepository(ctx context.Context, root string) SourcePullRepository {
	if _, err := exec.LookPath("git"); err != nil {
		return SourcePullRepository{Path: root, State: SourcePullNotRepository, Blocker: "git_not_found", Problem: "git executable not found"}
	}
	top, _ := workspaceGitOutput(ctx, root, "rev-parse", "--show-toplevel")
	topAbs, topErr := filepath.Abs(strings.TrimSpace(top))
	if topErr == nil && filepath.Clean(topAbs) != filepath.Clean(root) {
		return SourcePullRepository{Path: root, State: SourcePullNotRepository, Blocker: "not_repository_root", Problem: "mapped path is not the Git worktree root"}
	}
	item := SourcePullRepository{Path: root}
	if _, err := workspaceGitOutput(ctx, root, "rev-parse", "--show-toplevel"); err != nil {
		item.State = SourcePullNotRepository
		item.Blocker = "not_repository"
		item.Problem = "mapped path is not a Git worktree"
		item.NextAction = "verify the workspace mapping or initialize/clone the repository with Git"
		return item
	}
	status, err := workspaceGitOutput(ctx, root, "status", "--porcelain=v1", "--untracked-files=normal")
	if err != nil {
		item.State = SourcePullUnavailable
		item.Blocker = "status_failed"
		item.Problem = "cannot inspect Git working tree: " + err.Error()
		item.NextAction = "run 'git status' in the repository"
		return item
	}
	item.Dirty = strings.TrimSpace(status) != ""

	branch, err := workspaceGitOutput(ctx, root, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil || strings.TrimSpace(branch) == "" {
		item.State = SourcePullDetached
		item.Blocker = "detached_head"
		item.Problem = "repository is not on a named branch"
		item.NextAction = "use Git to switch to the intended branch"
		return item
	}
	item.Branch = strings.TrimSpace(branch)

	upstream, err := workspaceGitOutput(ctx, root, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{upstream}")
	if err != nil || strings.TrimSpace(upstream) == "" {
		item.State = SourcePullNoUpstream
		item.Blocker = "missing_upstream"
		item.Problem = "current branch has no configured upstream"
		item.NextAction = "review 'git branch -vv' and configure the intended upstream with Git"
		return item
	}
	item.Upstream = strings.TrimSpace(upstream)

	if _, ferr := workspaceGitOutput(ctx, root, "fetch", "--prune", "--", item.Remote()); ferr != nil {
		item.State = SourcePullUnavailable
		item.Blocker = "fetch_failed"
		item.Problem = "git fetch failed: " + ferr.Error()
		item.NextAction = "run 'git fetch' directly to review connectivity or authentication"
		return item
	}

	current, cerr := workspaceGitOutput(ctx, root, "rev-parse", "HEAD")
	if cerr != nil {
		item.State = SourcePullUnavailable
		item.Blocker = "current_revision_unavailable"
		item.Problem = "cannot read current revision: " + cerr.Error()
		item.NextAction = "run 'git status' in the repository"
		return item
	}
	target, terr := workspaceGitOutput(ctx, root, "rev-parse", "@{upstream}")
	if terr != nil {
		item.State = SourcePullUnavailable
		item.Blocker = "upstream_revision_unavailable"
		item.Problem = "cannot read upstream revision: " + terr.Error()
		item.NextAction = "run 'git fetch' and 'git branch -vv'"
		return item
	}
	item.CurrentRevision = strings.TrimSpace(current)
	item.TargetRevision = strings.TrimSpace(target)
	if item.CurrentRevision == item.TargetRevision {
		return item
	}
	item.UpdateAvailable = workspaceGitIsAncestor(ctx, root, item.CurrentRevision, item.TargetRevision)
	if !item.UpdateAvailable {
		if workspaceGitIsAncestor(ctx, root, item.TargetRevision, item.CurrentRevision) {
			item.State = SourcePullAhead
			item.Blocker = "ahead_only"
			item.Problem = "local branch contains commits not present upstream"
			item.NextAction = "review 'git status' and your normal team Git workflow"
		} else {
			item.State = SourcePullDiverged
			item.Blocker = "diverged"
			item.Problem = "local and upstream history have diverged"
			item.NextAction = "review 'git log --oneline --left-right HEAD...@{u}' and resolve with your normal Git workflow"
		}
	}
	return item
}

// Remote returns the remote name parsed from the configured upstream.
func (i SourcePullRepository) Remote() string {
	if i.Upstream == "" {
		return ""
	}
	return strings.SplitN(i.Upstream, "/", 2)[0]
}

const SourcePullSchemaVersion = "baseharbor.source-pull/v1"
