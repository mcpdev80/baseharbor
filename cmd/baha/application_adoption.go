package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
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
	owner := application.Store{Root: filepath.Join(absolute, ".baseharbor", "apps")}
	target, err := effectiveTarget(ctx)
	if err != nil {
		return applicationAdoptionResult{}, err
	}
	snapshot, err := repositoryIdentityClaims(target, absolute, owner, m)
	if err != nil {
		return applicationAdoptionResult{}, err
	}
	// A remote Core is the sole authority that may allocate an application ID.
	// Guided init still writes the authored source locally, but must not create a
	// competing workstation identity before `baha app create` receives the Core
	// receipt. Existing repository identities remain subject to the normal
	// continuity checks below.
	if target.AccessProvider != "" && target.AccessProvider != "local" &&
		m.ApplicationID == "" && !snapshot.PreviouslyRegistered && len(snapshot.ApplicationIDs) == 0 {
		return persistRemoteRepositorySource(absolute, path, m, selected)
	}
	// Never attest a fresh application when an existing identity is being adopted.
	freshIdentity := !snapshot.PreviouslyRegistered && len(snapshot.ApplicationIDs) == 0 && m.ApplicationID == ""
	m, err = application.ResolveIdentity(m, snapshot)
	if err != nil {
		return applicationAdoptionResult{}, err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return applicationAdoptionResult{}, fmt.Errorf("%s already exists; edit the existing application contract instead", path)
		}
		return applicationAdoptionResult{}, err
	}
	// Claim the authored path before initialization, so rejecting an existing
	// manifest cannot allocate or persist identity. Retain a canonical identity
	// after a later write failure so an authorized retry reuses the same owner.
	m, err = owner.InitializeIdentity(m, snapshot)
	if err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return applicationAdoptionResult{}, err
	}
	if _, err := file.WriteString(m.AuthoredYAML()); err != nil {
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
	// Initialization owns the generated identity. Keep it in the existing
	// canonical Store while the authored contract contains only application needs.
	if _, err := owner.Sync(m); err != nil {
		_ = os.Remove(path)
		return applicationAdoptionResult{}, err
	}
	// A positive, one-shot proof exists only for a newly initialized app.
	// First deployment persistence consumes it BEFORE touching runtime state.
	// Older applications without this proof cannot claim "never delivered".
	if freshIdentity {
		if err := deployment.CreateNeverDeliveredProof(target.Name, m.ApplicationID, m.Name, m.Environment); err != nil {
			return applicationAdoptionResult{}, fmt.Errorf("record never-delivered provenance: %w", err)
		}
	}
	return result, nil
}

func persistRemoteRepositorySource(root, path string, m application.Manifest, selected *repositoryinspect.WorkloadSourceCandidate) (applicationAdoptionResult, error) {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return applicationAdoptionResult{}, fmt.Errorf("%s already exists; edit the existing application contract instead", path)
		}
		return applicationAdoptionResult{}, err
	}
	if _, err := file.WriteString(m.AuthoredYAML()); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return applicationAdoptionResult{}, err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return applicationAdoptionResult{}, err
	}
	result := applicationAdoptionResult{Application: m.Name, Environment: m.Environment, Manifest: path}
	if selected != nil {
		result.SourceSelection, err = repositoryinspect.WriteRepositoryMetadata(root, *selected)
		if err != nil {
			_ = os.Remove(path)
			return applicationAdoptionResult{}, err
		}
	}
	return result, nil
}
