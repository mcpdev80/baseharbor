package runtime

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// runtimeLister is the narrow engine surface the conflict gate needs: the
// running containers of a single local engine. It is satisfied by the full
// runtime RuntimeProvider without importing that contract here.
type runtimeLister interface {
	ListRuntimeContainers(context.Context) ([]RuntimeContainer, error)
}

// SharedCoreOwnerConflict reports that a BaseHarbor Core for a *different*
// owning target is already running on the local engine. Two same-host Targets
// that bind the same local engine share exactly one Core; starting a second
// Target's control-plane would silently reuse that Core instead of isolating on
// it. The project name is keyed by target name (SharedProjectName), but the
// engine runs a single Core per local engine, so per-target project names do
// not partition it. This gate makes the reuse fail closed and names the owning
// target instead of letting the second Target's up resolve to the first.
type SharedCoreOwnerConflict struct {
	// OwningTargets lists the owning target names whose running Core the engine
	// currently exposes, excluding the effective target itself.
	OwningTargets []string
	// RunningProjects lists the conflicting Core container projects observed on
	// the engine, for an unambiguous operator message.
	RunningProjects []string
}

func (err *SharedCoreOwnerConflict) Error() string {
	owners := expandSharedProjectOwners(err.RunningProjects)
	return fmt.Sprintf(
		"BaseHarbor Core is already running on this engine for target %s; a second same-host Target cannot silently share it. Select the owning target explicitly (e.g. 'baha use %s' or 'baha up --target %s'), or give this Target a separate engine/reference so its Core is isolated.",
		strings.Join(owners, ", "), strings.Join(owners, ","), strings.Join(owners, ","),
	)
}

// Unwrap lets machine.Classify treat this as a conflict.
func (err *SharedCoreOwnerConflict) Unwrap() error { return errors.New("shared core owner conflict") }

// SharedCoreOwnerConflictForEngine inspects the running containers of the given
// engine and returns a *SharedCoreOwnerConflict when a Core belonging to a
// target other than owningTarget is already running. A missing engine or an
// engine with no other Core reports no conflict.
func SharedCoreOwnerConflictForEngine(ctx context.Context, runtimeOwner runtimeLister, ownProject string) (*SharedCoreOwnerConflict, error) {
	containers, err := runtimeOwner.ListRuntimeContainers(ctx)
	if err != nil {
		return nil, err
	}
	ownProject = strings.TrimSpace(ownProject)
	seenProject := map[string]bool{}
	for _, container := range containers {
		if !container.Running {
			continue
		}
		project := strings.TrimSpace(container.Project)
		if !isSharedCoreProject(project) {
			continue
		}
		// Managed Identity uses a sibling <target>-core-shared project.
		// It belongs to this same Target, not to a foreign Core.
		if ownProject != "" && (project == ownProject || project == strings.TrimSuffix(ownProject, "-shared")+"-core-shared") {
			continue
		}
		seenProject[project] = true
	}
	if len(seenProject) == 0 {
		return nil, nil
	}
	var projects []string
	for project := range seenProject {
		projects = append(projects, project)
	}
	sort.Strings(projects)
	owners := expandSharedProjectOwners(projects)
	return &SharedCoreOwnerConflict{OwningTargets: owners, RunningProjects: projects}, nil
}

// isSharedCoreProject reports whether a container's compose-project label is a
// target-wide BaseHarbor Core project (bh-<target>-shared) rather than an
// application project (bh-<target>-<app>-<env>).
func isSharedCoreProject(project string) bool {
	project = strings.TrimSpace(project)
	if !strings.HasPrefix(project, "bh-") || !strings.HasSuffix(project, "-shared") {
		return false
	}
	middle := strings.TrimSuffix(strings.TrimPrefix(project, "bh-"), "-shared")
	middle = strings.Trim(middle, "-")
	return middle != ""
}

// sharedCoreOwnerFromProject inverts SharedProjectName to recover the owning
// target name from a bh-<target>-shared project label.
func sharedCoreOwnerFromProject(project string) string {
	if isSharedCoreProject(project) {
		return strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(project, "bh-"), "-shared"))
	}
	return ""
}

// expandSharedProjectOwners resolves each observed project to its owning target
// name, including baseharbor-<owner> resource prefixes, and returns the sorted
// unique owner list.
func expandSharedProjectOwners(projects []string) []string {
	set := map[string]bool{}
	for _, project := range projects {
		if owner := sharedCoreOwnerFromProject(project); owner != "" {
			set[owner] = true
			continue
		}
		// baseharbor-<owner> resource prefix: strip the prefix to recover the
		// owning target name rather than exposing the physical resource prefix.
		if strings.HasPrefix(project, "baseharbor-") {
			owner := strings.TrimPrefix(project, "baseharbor-")
			if owner != "" {
				set[owner] = true
			}
			continue
		}
	}
	return sortedKeys(set)
}

func sortedKeys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for key := range set {
		if key != "" {
			out = append(out, key)
		}
	}
	sort.Strings(out)
	return out
}
