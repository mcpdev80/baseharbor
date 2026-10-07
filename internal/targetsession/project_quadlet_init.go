package targetsession

import (
	"context"
	"errors"
	"path"
	"strings"
	"time"
)

// ApplyQuadletInitGraph keeps completion sequencing in Core. The complete graph
// is published first; an init unit's current source must finish successfully
// before its dependent can activate. No admitted mutation is retried.
func (r *ProjectRuntime) ApplyQuadletInitGraph(ctx context.Context, project *StagedProject, files, initFiles []string) error {
	selected, err := r.quadletGraph(project, files)
	if err != nil {
		return err
	}
	init := map[string]bool{}
	selectedUnits := map[string]bool{}
	for _, file := range selected {
		selectedUnits[file] = true
	}
	for _, file := range initFiles {
		if !selectedUnits[file] || path.Ext(file) != ".container" || init[file] {
			return errors.New("invalid staged init unit selection")
		}
		init[file] = true
	}
	if err := refuseImplicitInitActivation(project, selected, init); err != nil {
		return err
	}
	if len(init) != 0 {
		if err := r.requireCapability("runtime.quadlet.verify-completion"); err != nil {
			return err
		}
	}
	bounded, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	if err := r.PublishQuadletGraph(bounded, project, selected); err != nil {
		return err
	}
	for _, file := range selected {
		if path.Ext(file) != ".container" {
			continue
		}
		if err := r.applyQuadletFile(bounded, project, file, true); err != nil {
			return err
		}
		if init[file] {
			if err := r.waitQuadletCompletion(bounded, project, file); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *ProjectRuntime) waitQuadletCompletion(ctx context.Context, project *StagedProject, file string) error {
	for {
		if err := r.VerifyQuadletCompletion(ctx, project, file); err == nil {
			return nil
		} else if errors.Is(err, ErrUnavailable) {
			return err
		}
		timer := time.NewTimer(200 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

// An inactive successful init must not be implicitly rerun by systemd when its
// dependent activates. After preserves ordering; Wants/Requires would start it.
func refuseImplicitInitActivation(project *StagedProject, files []string, init map[string]bool) error {
	for _, file := range files {
		section := ""
		for _, line := range strings.Split(string(project.files[file].data), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
				section = line
			}
			key, value, ok := strings.Cut(line, "=")
			if section != "[Unit]" || !ok || (key != "Wants" && key != "Requires" && key != "BindsTo") {
				continue
			}
			for _, unit := range strings.Fields(value) {
				if init[strings.TrimSuffix(unit, ".service")+".container"] {
					return errors.New("init dependency would activate implicitly")
				}
			}
		}
	}
	return nil
}
