package development

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Incremental source-model maintenance keeps the portable source identity in
// .baseharbor/sources.yaml and the developer-local checkout mapping separate.
// Detaching a source removes only its local mapping entry; no local file or
// checkout is ever deleted here.

// UpsertSourceDefinition adds or replaces one source by identity while
// preserving declaration order for deterministic machine output. Replacing a
// source that components still reference with a different kind is refused so
// ownership of a component mapping can never change silently.
func UpsertSourceDefinition(model *SourceModel, source SourceDefinition) error {
	source.ID = strings.TrimSpace(source.ID)
	if source.ID == "" {
		return fmt.Errorf("source id is required")
	}
	for _, component := range model.Components {
		if strings.TrimSpace(component.Source) != source.ID {
			continue
		}
		var current SourceDefinition
		found := false
		for _, existing := range model.Sources {
			if strings.TrimSpace(existing.ID) == source.ID {
				current, found = existing, true
				break
			}
		}
		if !found {
			// A brand-new source id has no previous kind to consult; the
			// candidate validation below rejects unsupported types. Reject the
			// write anyway so components never end up pointing at a source
			// that was not already owned by the model.
			return fmt.Errorf("component %q references source %q which is not declared yet", component.Component, source.ID)
		}
		if current.Type != source.Type {
			return fmt.Errorf("source %q is referenced by component %q; remove the component mapping before switching it from %s to %s", source.ID, component.Component, current.Type, source.Type)
		}
	}
	replaced := false
	merged := make([]SourceDefinition, 0, len(model.Sources)+1)
	for _, existing := range model.Sources {
		if !replaced && strings.TrimSpace(existing.ID) == source.ID {
			merged = append(merged, source)
			replaced = true
			continue
		}
		merged = append(merged, existing)
	}
	if !replaced {
		merged = append(merged, source)
	}
	candidate := SourceModel{
		SchemaVersion: model.SchemaVersion,
		Application:   model.Application,
		Sources:       merged,
		Components:    model.Components,
	}
	if err := candidate.Validate(); err != nil {
		return err
	}
	model.Sources = merged
	return nil
}

// RemoveSourceDefinition removes a source from the model. While any component
// still references it the source stays owned by that mapping: with
// preserveComponents the call reports how many references remain, without it
// every referencing component is detached first. The final source of a model
// cannot be removed (ErrWorkspaceLastSource).
func RemoveSourceDefinition(model *SourceModel, sourceID string, preserveComponents bool) (detached []string, err error) {
	sourceID = strings.TrimSpace(sourceID)
	if sourceID == "" {
		return nil, fmt.Errorf("source id is required")
	}
	found := false
	for _, source := range model.Sources {
		if strings.TrimSpace(source.ID) == sourceID {
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("unknown source %q", sourceID)
	}
	var referenced []string
	kept := model.Components[:0:0]
	for _, component := range model.Components {
		if strings.TrimSpace(component.Source) == sourceID {
			referenced = append(referenced, component.Component)
			continue
		}
		kept = append(kept, component)
	}
	if len(referenced) > 0 && preserveComponents {
		return nil, fmt.Errorf("source %q is referenced by component(s) %s; remove the component mapping first", sourceID, strings.Join(referenced, ", "))
	}
	remaining := make([]SourceDefinition, 0, len(model.Sources))
	for _, source := range model.Sources {
		if strings.TrimSpace(source.ID) == sourceID {
			continue
		}
		remaining = append(remaining, source)
	}
	if len(remaining) == 0 {
		return nil, ErrWorkspaceLastSource
	}
	model.Sources = remaining
	model.Components = kept
	return referenced, nil
}

// UpsertComponentMapping adds or replaces the unique mapping of one component.
func UpsertComponentMapping(model *SourceModel, component ComponentSource) error {
	component.Component = strings.TrimSpace(component.Component)
	if component.Component == "" {
		return fmt.Errorf("component source mapping requires component")
	}
	for i, existing := range model.Components {
		if strings.TrimSpace(existing.Component) == component.Component {
			model.Components[i] = component
			return model.Validate()
		}
	}
	model.Components = append(model.Components, component)
	return model.Validate()
}

// RemoveComponentMapping detaches one component from the source model.
func RemoveComponentMapping(model *SourceModel, component string) error {
	component = strings.TrimSpace(component)
	if component == "" {
		return fmt.Errorf("component source mapping requires component")
	}
	kept := model.Components[:0:0]
	found := false
	for _, existing := range model.Components {
		if strings.TrimSpace(existing.Component) == component {
			found = true
			continue
		}
		kept = append(kept, existing)
	}
	if !found {
		return fmt.Errorf("component %q is not mapped in the source model", component)
	}
	model.Components = kept
	return nil
}

// SaveRemovableSource detaches one source from the model and its local
// workspace mapping in one pass: components referencing the source are
// removed from the portable model unless that would leave a dangling mapping.
// The mapping entry is removed last so a failed model write never orphans a
// local path. No local file or checkout is deleted.
func SaveRemovableSource(manifestPath, application string, model SourceModel, sourceID string) (sourceModelPath string, mappingPath string, detachedComponents []string, removedMapping bool, err error) {
	return saveRemovableSource(manifestPath, application, model, sourceID, SaveWorkspaceMapping)
}

func saveRemovableSource(manifestPath, application string, model SourceModel, sourceID string, saveMapping func(string, WorkspaceMapping) (string, error)) (sourceModelPath string, mappingPath string, detachedComponents []string, removedMapping bool, err error) {
	sourceID = strings.TrimSpace(sourceID)
	detached, err := RemoveSourceDefinition(&model, sourceID, false)
	if err != nil {
		return "", "", nil, false, err
	}
	manifestPath, err = filepath.Abs(strings.TrimSpace(manifestPath))
	if err != nil {
		return "", "", nil, false, err
	}
	sourceModelPath = filepath.Join(filepath.Dir(manifestPath), filepath.FromSlash(SourceModelRelativePath))
	originalModel, err := readRollbackSnapshot(sourceModelPath)
	if err != nil {
		return "", "", nil, false, fmt.Errorf("snapshot source model before removal: %w", err)
	}
	// Validate the developer-local mapping before changing the portable model.
	// A malformed or mismatched mapping must not turn a failed remove into a
	// partial model update that leaves the stale mapping behind.
	mapping, mappingPath, mapErr := LoadWorkspaceMapping(manifestPath, application)
	if mapErr != nil && !os.IsNotExist(mapErr) {
		return "", mappingPath, nil, false, mapErr
	}
	var originalMapping fileRollbackSnapshot
	if mapErr == nil {
		originalMapping, err = readRollbackSnapshot(mappingPath)
		if err != nil {
			return "", mappingPath, nil, false, fmt.Errorf("snapshot workspace mapping before removal: %w", err)
		}
	}
	sourceModelPath, err = WriteSourceModel(manifestPath, model)
	if err != nil {
		return "", "", nil, false, err
	}
	if mapErr != nil {
		return sourceModelPath, mappingPath, detached, false, nil
	}
	if _, exists := mapping.Sources[sourceID]; exists {
		delete(mapping.Sources, sourceID)
		if _, err := saveMapping(manifestPath, mapping); err != nil {
			rollbackErr := restoreFileSnapshot(sourceModelPath, originalModel)
			if rollbackErr != nil {
				err = errors.Join(err, fmt.Errorf("restore source model after failed mapping save: %w", rollbackErr))
			}
			rollbackErr = restoreFileSnapshot(mappingPath, originalMapping)
			if rollbackErr != nil {
				err = errors.Join(err, fmt.Errorf("restore workspace mapping after failed mapping save: %w", rollbackErr))
			}
			return sourceModelPath, mappingPath, nil, false, err
		}
		removedMapping = true
	}
	return sourceModelPath, mappingPath, detached, removedMapping, nil
}

type fileRollbackSnapshot struct {
	data []byte
	mode os.FileMode
}

func readRollbackSnapshot(path string) (fileRollbackSnapshot, error) {
	info, err := os.Stat(path)
	if err != nil {
		return fileRollbackSnapshot{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fileRollbackSnapshot{}, err
	}
	return fileRollbackSnapshot{data: data, mode: info.Mode().Perm()}, nil
}

func restoreFileSnapshot(path string, snapshot fileRollbackSnapshot) error {
	tmp := path + ".rollback.tmp"
	if err := os.WriteFile(tmp, snapshot.data, snapshot.mode); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// SourceComponentNames lists, sorted and deterministic, the components
// currently mapped to the given source.
func SourceComponentNames(model SourceModel, sourceID string) []string {
	sourceID = strings.TrimSpace(sourceID)
	var names []string
	for _, component := range model.Components {
		if strings.TrimSpace(component.Source) == sourceID {
			names = append(names, component.Component)
		}
	}
	sort.Strings(names)
	return names
}
