package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/targetenrollment"
	"github.com/mcpdev80/baseharbor/internal/targetsession"
)

// Reload protected state rather than retaining a potentially stale resolution.
// Changing the selected Node cannot silently adopt or discard its live project.
func retainedRemoteProject(resolved resolvedApplication) (*targetsession.ProjectRecord, error) {
	record, err := deployment.LoadDeploymentRecord(resolved.DeploymentIdentity)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	project := record.Applied.RemoteProject
	if project == nil {
		return nil, nil
	}
	target := resolved.Target
	expected := targetenrollment.Scope{TenantID: target.TenantID, TargetID: target.Name, NodeID: target.AccessReference, Runtime: target.RuntimeProvider}
	if target.AccessProvider != "baseharbor-node-connector" || expected.Validate() != nil || project.Scope != expected {
		return nil, fmt.Errorf("persisted remote project differs from selected Target Access binding")
	}
	return project, nil
}
