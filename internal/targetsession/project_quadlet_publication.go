package targetsession

import "context"

// PublishQuadletGraph publishes validated, receipt-bound sources without
// activating containers. Completion dependencies can be staged before execution.
func (r *ProjectRuntime) PublishQuadletGraph(ctx context.Context, project *StagedProject, files []string) error {
	selected, err := r.quadletGraph(project, files)
	if err != nil {
		return err
	}
	if err := r.requireCapability("runtime.quadlet.apply"); err != nil {
		return err
	}
	for _, file := range selected {
		if err := r.applyQuadletFile(ctx, project, file, false); err != nil {
			return err
		}
	}
	return nil
}
