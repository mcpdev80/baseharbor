package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	runtimecontract "github.com/mcpdev80/baseharbor/internal/runtime/contract"
	"github.com/mcpdev80/baseharbor/internal/runtimeexplorer"
)

type deploymentRuntimeOwnershipResolver struct{}

func (deploymentRuntimeOwnershipResolver) ResolveContainer(
	_ context.Context,
	target string,
	provider runtimecontract.ProviderKind,
	container runtimecontract.RuntimeContainer,
) (runtimeexplorer.OwnershipEvidence, error) {
	records, err := deployment.ListDeployments(target)
	if err != nil {
		return runtimeexplorer.OwnershipEvidence{}, err
	}

	var matches []runtimeexplorer.OwnershipEvidence
	for _, record := range records {
		if strings.TrimSpace(record.Applied.RuntimeProvider) != string(provider) {
			continue
		}
		var manifest application.Manifest
		if len(record.Applied.Intent) == 0 {
			continue
		}
		if err := json.Unmarshal(record.Applied.Intent, &manifest); err != nil {
			return runtimeexplorer.OwnershipEvidence{}, fmt.Errorf(
				"decode deployment intent for runtime ownership %s: %w",
				record.Identity.DeploymentID,
				err,
			)
		}
		files := application.RuntimeFilesFor(application.Store{Namespace: target}, manifest)
		expectedProject := application.WorkloadProjectNameForRuntime(manifest, files)
		if strings.TrimSpace(container.Project) != expectedProject {
			continue
		}

		relationship := runtimeexplorer.Relationship{
			ApplicationID: record.Identity.ApplicationID,
			DeploymentID:  record.Identity.DeploymentID,
			Application:   record.Identity.Application,
			Environment:   record.Identity.Environment,
		}
		if workloadComponent(manifest, container.Service) {
			relationship.Component = container.Service
		} else {
			relationship.Provider = container.Service
		}
		matches = append(matches, runtimeexplorer.OwnershipEvidence{
			Ownership:    runtimeexplorer.OwnershipManaged,
			Relationship: relationship,
			Reconciliation: runtimeexplorer.ReconciliationHint{
				PreferredOperation:            "apply",
				DirectMutationMayBeReconciled: true,
				Detail:                        "Direct runtime mutation may be replaced by the next BaseHarbor reconciliation.",
			},
		})
	}

	switch len(matches) {
	case 0:
		return runtimeexplorer.OwnershipEvidence{Ownership: runtimeexplorer.OwnershipUnmanaged}, nil
	case 1:
		return matches[0], nil
	default:
		return runtimeexplorer.OwnershipEvidence{}, &runtimeOwnershipAmbiguousError{
			target:   target,
			provider: provider,
			resource: container.ID,
		}
	}
}

func workloadComponent(manifest application.Manifest, service string) bool {
	service = strings.TrimSpace(service)
	if service == "" {
		return false
	}
	for _, component := range application.WorkloadComponentNames(manifest) {
		if component == service {
			return true
		}
	}
	return false
}

type runtimeOwnershipAmbiguousError struct {
	target   string
	provider runtimecontract.ProviderKind
	resource string
}

func (e *runtimeOwnershipAmbiguousError) Error() string {
	return fmt.Sprintf(
		"multiple BaseHarbor deployments claim runtime resource %s on target %s provider %s",
		e.resource,
		e.target,
		e.provider,
	)
}

var _ runtimeexplorer.OwnershipResolver = deploymentRuntimeOwnershipResolver{}
