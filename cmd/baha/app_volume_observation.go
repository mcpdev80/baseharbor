package main

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mcpdev80/baseharbor/internal/application"
)

// Read-only provenance receipt outlives removed app state. It grants no ownership
// or deletion rights: every subsequent inventory re-inspects engine metadata.
func (e *applicationDestroyExecution) repositoryVolumeObservationPath() string {
	project := application.WorkloadProjectNameForRuntime(e.manifest, e.files)
	key := fmt.Sprintf("%x", sha256.Sum256([]byte(project)))
	return filepath.Join(e.resolved.TargetStateRoot, "preserved-compose-volumes", key+".json")
}

func (e *applicationDestroyExecution) previousRepositoryVolumeNames() ([]string, error) {
	data, err := os.ReadFile(e.repositoryVolumeObservationPath())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read preserved Compose volume inventory: %w", err)
	}
	var names []string
	if err := json.Unmarshal(data, &names); err != nil {
		return nil, fmt.Errorf("decode preserved Compose volume inventory: %w", err)
	}
	return names, nil
}

func (e *applicationDestroyExecution) saveRepositoryVolumeObservation() error {
	if len(e.repositoryVolumes) == 0 {
		return nil
	}
	names := make([]string, 0, len(e.repositoryVolumes))
	for _, volume := range e.repositoryVolumes {
		names = append(names, volume.Name)
	}
	data, err := json.Marshal(names)
	if err != nil {
		return err
	}
	path := e.repositoryVolumeObservationPath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".volume-inventory-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if err := file.Chmod(0600); err != nil {
		_ = file.Close()
		return err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}
