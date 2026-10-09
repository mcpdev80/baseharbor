package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/openbao"
)

// verifyOwnedApplicationSecretScopes checks the actual protected AppRole
// credentials of every registered application that requested secrets. A
// missing/foreign deployment runtime is a hard failure, not permission to
// silently skip an application's secret access after a Core upgrade.
func (o *coreNativeRuntimeOps) verifyOwnedApplicationSecretScopes(ctx context.Context) error {
	records, err := deployment.ListDeployments(o.target)
	if err != nil {
		return err
	}
	return verifyApplicationSecretScopeRecords(records, func(ctx context.Context, id openbao.ApplicationIdentity, path string) error {
		return openbao.CheckApplicationScope(ctx, o.runtime, o.core, id, path)
	}, ctx)
}

func verifyApplicationSecretScopeRecords(records []deployment.DeploymentRecord, check func(context.Context, openbao.ApplicationIdentity, string) error, ctx context.Context) error {
	if check == nil {
		return errors.New("managed OpenBao application scope verifier required")
	}
	for _, record := range records {
		var m application.Manifest
		if len(record.Applied.Intent) == 0 || json.Unmarshal(record.Applied.Intent, &m) != nil {
			return fmt.Errorf("deployment %s missing valid applied secret intent", record.Identity.DeploymentID)
		}
		if m.Name != record.Identity.Application || m.Environment != record.Identity.Environment {
			return errors.New("deployment application identity differs from applied secret scope")
		}
		if !m.Services.Secrets {
			continue
		}
		root, err := deployment.DeploymentRoot(record.Identity)
		if err != nil {
			return err
		}
		runtimeDir := filepath.Join(root, "state", m.Name, "runtime")
		if record.Applied.GeneratedState["runtime"] != runtimeDir {
			return fmt.Errorf("application %s OpenBao credentials not bound to the owned deployment runtime", m.Name)
		}
		path := openbao.ApplicationCredentialsPath(runtimeDir)
		st, err := os.Lstat(path)
		if err != nil {
			return fmt.Errorf("application %s protected OpenBao credential unavailable: %w", m.Name, err)
		}
		if !st.Mode().IsRegular() || st.Mode().Perm()&0077 != 0 {
			return fmt.Errorf("application %s OpenBao credential is not owner-only regular file", m.Name)
		}
		if err := check(ctx, openbao.ApplicationIdentity{Name: m.Name, Environment: m.Environment}, path); err != nil {
			return fmt.Errorf("application %s/%s AppRole secret-scope verification failed: %w", m.Name, m.Environment, err)
		}
	}
	return nil
}
