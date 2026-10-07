package targetsession

import (
	"context"
	"errors"
	"path"
	"sort"
)

// A graph is validated completely before its first mutation. Resource units are
// published before containers; interrupted calls are never retried implicitly.
func (r *ProjectRuntime) quadletGraph(project *StagedProject, files []string) ([]string, error) {
	if r == nil || r.scope.Runtime != "podman" || project == nil || project.scope != r.scope || len(files) == 0 || len(files) > 64 {
		return nil, ErrUnavailable
	}
	selected := append([]string(nil), files...)
	seen := map[string]bool{}
	for _, file := range selected {
		ext := path.Ext(file)
		if path.Base(file) != file || !projectBundleID.MatchString(file[:len(file)-len(ext)]) ||
			(ext != ".container" && ext != ".network" && ext != ".volume") || seen[file] {
			return nil, errors.New("invalid staged Quadlet graph")
		}
		if _, ok := project.files[file]; !ok {
			return nil, errors.New("unstaged Quadlet graph member")
		}
		seen[file] = true
	}
	sort.Slice(selected, func(i, j int) bool {
		iContainer, jContainer := path.Ext(selected[i]) == ".container", path.Ext(selected[j]) == ".container"
		if iContainer != jContainer {
			return !iContainer
		}
		return selected[i] < selected[j]
	})
	return selected, nil
}

func (r *ProjectRuntime) ApplyQuadletGraph(ctx context.Context, project *StagedProject, files []string) error {
	selected, err := r.quadletGraph(project, files)
	if err != nil {
		return err
	}
	if err := r.requireCapability("runtime.quadlet.apply"); err != nil {
		return err
	}
	for _, file := range selected {
		payload := struct {
			Name             string `json:"name"`
			Content          string `json:"content"`
			Enable           bool   `json:"enable"`
			ProjectDirectory string `json:"project_directory"`
		}{file, string(project.files[file].data), path.Ext(file) == ".container", project.directory}
		if err := r.invoke(ctx, "runtime.quadlet.apply", payload, nil); err != nil {
			return err
		}
	}
	return nil
}

// DestroyQuadletGraph removes only the selected, receipt-bound units. Provider
// data volumes remain intact; this is not an implicit destructive reset.
func (r *ProjectRuntime) DestroyQuadletGraph(ctx context.Context, project *StagedProject, files []string) error {
	selected, err := r.quadletGraph(project, files)
	if err != nil {
		return err
	}
	if err := r.requireCapability("runtime.quadlet.remove"); err != nil {
		return err
	}
	for i := len(selected) - 1; i >= 0; i-- {
		if err := r.invoke(ctx, "runtime.quadlet.remove", struct {
			Name string `json:"name"`
		}{selected[i]}, nil); err != nil {
			return err
		}
	}
	return nil
}
