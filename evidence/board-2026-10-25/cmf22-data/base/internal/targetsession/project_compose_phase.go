package targetsession

import (
	"context"
	"errors"
	"regexp"

	"go.yaml.in/yaml/v3"
)

var composePhaseService = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`)

// ApplyComposeSelected activates only a Core-selected phase of one immutable
// project. The node disables implicit dependency activation and source builds;
// Core must explicitly converge and verify prerequisites before the next phase.
func (r *ProjectRuntime) ApplyComposeSelected(ctx context.Context, project *StagedProject, files []string, envFile string, services []string, repair bool) error {
	if _, _, err := r.composeSelection(project, files, envFile); err != nil {
		return err
	}
	if len(services) == 0 || len(services) > 64 {
		return errors.New("Compose phase requires bounded service selection")
	}
	declared := map[string]bool{}
	for _, file := range files {
		var document struct {
			Services map[string]any `yaml:"services"`
		}
		if yaml.Unmarshal(project.files[file].data, &document) != nil {
			return errors.New("invalid immutable Compose phase source")
		}
		for name := range document.Services {
			declared[name] = true
		}
	}
	seen := map[string]bool{}
	for _, service := range services {
		if !composePhaseService.MatchString(service) || !declared[service] || seen[service] {
			return errors.New("Compose phase differs from declared project services")
		}
		seen[service] = true
	}
	return r.applyCompose(ctx, project, files, envFile, append([]string(nil), services...), repair)
}
