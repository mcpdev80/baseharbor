package targetsession

import (
	"context"
	"path"
)

// ResetQuadletGraph is used only after an explicit Core-owned data reset
// decision. Validate every selection and live reset capability before teardown.
func (r *ProjectRuntime) ResetQuadletGraph(ctx context.Context, project *StagedProject, files []string) error {
	selected, err := r.quadletGraph(project, files)
	if err != nil {
		return err
	}
	for _, capability := range []string{"runtime.quadlet.reset-volume", "runtime.quadlet.apply", "runtime.quadlet.remove"} {
		if err := r.requireCapability(capability); err != nil {
			return err
		}
	}
	// Reconcile exact source without starting containers. This also supports
	// an explicit reset after ordinary destroy or an interrupted prior reset.
	if err := r.PublishQuadletGraph(ctx, project, selected); err != nil {
		return err
	}
	if err := r.DestroyQuadletGraph(ctx, project, selected); err != nil {
		return err
	}
	for _, file := range selected {
		if path.Ext(file) != ".volume" {
			continue
		}
		payload := struct {
			Name             string `json:"name"`
			Content          string `json:"content"`
			ProjectDirectory string `json:"project_directory"`
		}{file, string(project.files[file].data), project.directory}
		if err := r.invoke(ctx, "runtime.quadlet.reset-volume", payload, nil); err != nil {
			return err
		}
	}
	return nil
}
