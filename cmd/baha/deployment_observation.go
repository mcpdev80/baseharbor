package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/mcpdev80/baseharbor/internal/application"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"os"
	"path/filepath"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/deployment"
)

// Retain source and deployment inputs. Only registrations without any application
// runtime directory can be reconciled here; partial or live application state is
// not evidence of absence and must remain available for ownership-safe recovery.
func inactiveTargetDeployments(ctx context.Context, target string) ([]deployment.DeploymentRecord, error) {
	records, err := deployment.ListDeployments(target)
	if err != nil {
		return nil, err
	}
	var inactive []deployment.DeploymentRecord
	for _, record := range records {
		root, err := deployment.DeploymentRoot(record.Identity)
		if err != nil {
			return nil, err
		}
		runtimeDir := filepath.Join(root, "state", record.Identity.Application, "runtime")
		if _, err := os.Lstat(runtimeDir); err == nil {
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		if err := authorizeMCPOperation(ctx, "control-plane.destroy", target, record.Identity.Environment, record.Identity.ApplicationID, root); err != nil {
			return nil, err
		}
		inactive = append(inactive, record)
	}
	return inactive, nil
}

func markInactiveTargetDeployments(records []deployment.DeploymentRecord, containers []bhruntime.RuntimeContainer) error {
	for _, record := range records {
		current, err := deployment.LoadDeploymentRecord(record.Identity)
		if err != nil {
			return err
		}
		record = current
		// Recheck after teardown so concurrent/partial materialization is never lost.
		root, err := deployment.DeploymentRoot(record.Identity)
		if err != nil {
			return err
		}
		path := filepath.Join(root, "state", record.Identity.Application, "runtime")
		if _, err := os.Lstat(path); err == nil {
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		project := bhruntime.ApplicationProjectName(record.Identity.Target, record.Identity.Application, record.Identity.Environment)
		unscoped := application.WorkloadProjectName(application.Manifest{Name: record.Identity.Application, Environment: record.Identity.Environment})
		owned := false
		for _, container := range containers {
			if container.Project == project || strings.HasPrefix(container.Project, project+"-") || container.Project == unscoped {
				owned = true
				break
			}
		}
		if owned {
			continue
		}
		record.Observed = deployment.ObservedDeployment{State: "not_applied"}
		if err := deployment.SaveDeploymentRecord(record); err != nil {
			return fmt.Errorf("reconcile retained deployment observation: %w", err)
		}
	}
	return nil
}
