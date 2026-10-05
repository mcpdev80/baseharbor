package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/repositoryinspect"
	"os"
	"path/filepath"
)

type applicationAdoptionResult struct {
	Application     string `json:"application"`
	ApplicationID   string `json:"application_id"`
	Environment     string `json:"environment"`
	Manifest        string `json:"manifest"`
	SourceSelection string `json:"source_selection,omitempty"`
}

func persistRepositoryApplication(ctx context.Context, root string, m application.Manifest, selected *repositoryinspect.WorkloadSourceCandidate) (applicationAdoptionResult, error) {
	if err := application.ValidateApplicationID(m.ApplicationID); err != nil {
		return applicationAdoptionResult{}, err
	}
	if err := m.Validate(); err != nil {
		return applicationAdoptionResult{}, err
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return applicationAdoptionResult{}, err
	}
	path := filepath.Join(absolute, application.RepositoryManifestName)
	if err := authorizeMCPOperation(ctx, "app.adopt", "", m.Environment, m.ApplicationID, path); err != nil {
		return applicationAdoptionResult{}, err
	}
	inspection, err := repositoryinspect.Inspect(ctx, absolute)
	if err != nil {
		return applicationAdoptionResult{}, err
	}
	if selected != nil {
		if _, err := selectExplicitWorkloadSource(inspection.WorkloadSourceCandidates, *selected); err != nil {
			return applicationAdoptionResult{}, err
		}
	} else if len(m.Workload.Components) > 0 && len(inspection.WorkloadSourceCandidates) > 1 && inspection.SelectedWorkloadSource == nil {
		return applicationAdoptionResult{}, machine.Wrap(machine.ErrorValidationFailed, errors.New("repository workload source is ambiguous"), "Inspect the repository and provide an explicit workload source choice.", false)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return applicationAdoptionResult{}, fmt.Errorf("%s already exists; edit the existing application contract instead", path)
		}
		return applicationAdoptionResult{}, err
	}
	if _, err := file.WriteString(m.YAML()); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return applicationAdoptionResult{}, err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return applicationAdoptionResult{}, err
	}
	result := applicationAdoptionResult{Application: m.Name, ApplicationID: m.ApplicationID, Environment: m.Environment, Manifest: path}
	if selected != nil {
		result.SourceSelection, err = repositoryinspect.WriteRepositoryMetadata(absolute, *selected)
		if err != nil {
			_ = os.Remove(path)
			return applicationAdoptionResult{}, err
		}
	}
	return result, nil
}
